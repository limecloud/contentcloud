import { readFileSync, readdirSync } from 'node:fs';
import { extname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root=resolve(fileURLToPath(new URL('..',import.meta.url)));
const failures=[];

function filesUnder(directory,extensions){
  const absolute=join(root,directory);
  const result=[];
  const visit=current=>{
    for(const entry of readdirSync(current,{withFileTypes:true})){
      const path=join(current,entry.name);
      if(entry.isDirectory())visit(path);
      else if(extensions.has(extname(entry.name)))result.push(path);
    }
  };
  visit(absolute);
  return result;
}

function source(path){return readFileSync(path,'utf8')}
function projectPath(path){return relative(root,path).replaceAll('\\','/')}
function fail(path,message){failures.push(`${projectPath(path)}: ${message}`)}

function checkWebBoundary(owner,forbidden){
  for(const path of filesUnder(`web/src/${owner}`,new Set(['.ts','.tsx']))){
    const imports=[...source(path).matchAll(/(?:from\s*|import\s*\()\s*['"]([^'"]+)['"]/g)].map(match=>match[1]);
    for(const specifier of imports){
      if(specifier.includes(`/${forbidden}/`)||specifier.endsWith(`/${forbidden}`)||specifier.startsWith(`../${forbidden}`)){
        fail(path,`${owner} must not import ${forbidden} (${specifier})`);
      }
    }
  }
}

checkWebBoundary('studio','admin');
checkWebBoundary('admin','studio');

const runtimeBusinessModules=['identity','workspace','source','catalog','work','review','delivery','performance','experience'];
const runtimeLegacyRunReferences=['TaskRun','RunAttempt','task_runs','run_attempts'];
for(const path of filesUnder('internal/runtime',new Set(['.go']))){
  const value=source(path);
  for(const module of runtimeBusinessModules){
    if(value.includes(`/internal/${module}`))fail(path,`runtime must depend on a port/reference, not internal/${module}`);
  }
  for(const reference of runtimeLegacyRunReferences){
    if(value.includes(reference))fail(path,`current runtime must not depend on V7 compatibility reference ${reference}`);
  }
}

// The V7 daemon protocol and token-based lease API are deleted and forbidden
// from production code. Migration history belongs in migrations and docs,
// neither of which is scanned as a current Runtime dependency.
const retiredRuntimeReferences=['daemon.poll','run.report','run.heartbeat','run.finish','RunToken','LeaseNextRun'];
for(const path of filesUnder('internal',new Set(['.go']))){
  const relativePath=projectPath(path);
  if(relativePath.endsWith('_test.go'))continue;
  const value=source(path);
  for(const reference of retiredRuntimeReferences){
    if(value.includes(reference))fail(path,`current production code must not reference retired Runtime protocol ${reference}`);
  }
}

// Customer Studio is the first current owner cut over to Runtime. Its
// execution path may only start a JobRun and read the compatibility projection;
// a legacy TaskRun write here would recreate a second execution authority.
const currentRuntimeConsumerFiles = ['internal/app/customer_studio.go','internal/app/runtime_task_projection.go'];
const legacyWriteCalls = ['CreateRun(', 'CreateRunWithBundle(', 'SaveRun(', 'CreateRunAttempt(', 'SaveRunAttempt(', 'LeaseNextRun('];
for(const relativePath of currentRuntimeConsumerFiles){
  const path=join(root,relativePath);
  const value=source(path);
  for(const call of legacyWriteCalls){
    if(value.includes(call))fail(path,`current Runtime consumer must not write V7 execution fact ${call}`);
  }
}

// Runtime-backed read models must not silently fall back to the V7 Runs table.
for(const relativePath of ['internal/app/projection.go','internal/app/lineage.go']){
  const path=join(root,relativePath);
  if(source(path).includes('s.store.Runs('))fail(path,'Runtime-backed projection must use taskRunsForProject, not the V7 Runs table');
}

for(const path of filesUnder('internal/app',new Set(['.go']))){
  if(source(path).includes('s.store.Runs('))fail(path,'application code must not read the deleted V7 Runs table');
}

const legacyImports=['/internal/app','/internal/store','/internal/httpapi'];
for(const path of filesUnder('internal/experience',new Set(['.go']))){
  const value=source(path);
  for(const legacy of legacyImports){
    if(value.includes(legacy))fail(path,`target module must not import deprecated or compatibility package ${legacy}`);
  }
}

for(const path of filesUnder('internal/experience',new Set(['.go']))){
  const value=source(path);
  if(value.includes('/internal/domain'))fail(path,'target experience module must use its own records or stable platform values, not legacy domain');
}

const globalStorePath=join(root,'internal/store/store.go');
const globalStoreMethods=(source(globalStorePath).match(/^\s*[A-Z][A-Za-z0-9_]*\(context\.Context/gm)||[]).length;
const globalStoreBaseline=226;
if(globalStoreMethods>globalStoreBaseline){
  fail(globalStorePath,`deprecated Store grew from ${globalStoreBaseline} to ${globalStoreMethods} methods; define a narrow module port instead`);
}

if(failures.length){
  console.error('Architecture checks failed:\n'+failures.map(value=>`- ${value}`).join('\n'));
  process.exit(1);
}
console.log(`Architecture checks passed (legacy Store methods: ${globalStoreMethods}/${globalStoreBaseline}).`);
