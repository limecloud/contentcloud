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

const developmentBootstrapPath=join(root,'web/src/devBootstrap.ts');
if(!source(developmentBootstrapPath).includes('import.meta.env.DEV'))fail(developmentBootstrapPath,'development bootstrap must be removed from production builds');
for(const path of filesUnder('web/src',new Set(['.ts','.tsx']))){
  if(path===developmentBootstrapPath||projectPath(path).endsWith('.test.ts')||projectPath(path).endsWith('.test.tsx'))continue;
  if(source(path).includes('/api/v1/dev/bootstrap'))fail(path,'production web entry must not call the development bootstrap endpoint');
}

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

// Workspace and SOP identity migrations are data boundaries, not current
// production defaults. Historical rows may still be read by stores/tests,
// but new application code must not recreate retired identifiers.
const retiredWorkspaceReferences=['task_marketing_video','workspace_marketing_video','legacy/default-short-video'];
for(const path of filesUnder('internal',new Set(['.go']))){
  if(projectPath(path).endsWith('_test.go'))continue;
  const value=source(path);
  for(const reference of retiredWorkspaceReferences){
    if(value.includes(reference))fail(path,`current production code must not recreate retired workspace/SOP identity ${reference}`);
  }
}
const bootstrapPath=join(root,'internal/app/bootstrap_onboarding.go');
const bootstrapSource=source(bootstrapPath);
for(const required of ['localworkspace.TemplateID','localworkspace.TemplateVersion']){
  if(!bootstrapSource.includes(required))fail(bootstrapPath,`bootstrap must use current workspace template constant ${required}`);
}
for(const path of filesUnder('internal',new Set(['.go']))){
  const relativePath=projectPath(path);
  if(relativePath.endsWith('_test.go')||relativePath==='internal/cli/plugin_host.go')continue;
  if(source(path).includes('/integration/pluginbuiltin'))fail(path,'only the CLI installation boundary may depend on the bundled Plugin loader; identity consumers must use pluginidentity');
}

// Local daemon bindings are current in daemon_bindings. Legacy single-workspace
// keys may only be decoded and rewritten at the localconfig load boundary.
const localConfigPath=join(root,'internal/localconfig/config.go');
const localConfigSource=source(localConfigPath);
for(const required of ['type configFile struct','hasLegacyBinding','containsLegacyKeys','savePath(path, c)','func (c Config) Bindings()','func (c Config) PrimaryBinding()']){
  if(!localConfigSource.includes(required))fail(localConfigPath,`local config migration boundary is missing ${required}`);
}
if(localConfigSource.includes('RuntimeBindings()'))fail(localConfigPath,'local config must not retain a runtime single-workspace fallback');
for(const path of filesUnder('internal/cli',new Set(['.go']))){
  if(projectPath(path).endsWith('_test.go'))continue;
  const value=source(path);
  for(const reference of ['RuntimeBindings()','cfg.DeviceID','cfg.WorkspaceID','cfg.ProjectID','cfg.WorkspaceRoot']){
    if(value.includes(reference))fail(path,`CLI must use current DaemonBindings instead of legacy config reference ${reference}`);
  }
}

// Public Runtime reads use RuntimeRun/RuntimeRunEvent and runtime_run. The V7
// DTO and lineage names are deleted, not aliases that callers may restore.
const retiredRuntimeReadNames=['TaskRun','RunProgressEvent','task_run'];
for(const path of filesUnder('internal',new Set(['.go']))){
  if(projectPath(path).endsWith('_test.go'))continue;
  const value=source(path);
  for(const name of retiredRuntimeReadNames){
    if(value.includes(name))fail(path,`production Go must not restore retired Runtime read name ${name}`);
  }
}
for(const path of filesUnder('web/src',new Set(['.ts','.tsx']))){
  const value=source(path);
  for(const name of [...retiredRuntimeReadNames,'taskRun']){
    if(value.includes(name))fail(path,`web code must not restore retired Runtime read name ${name}`);
  }
}

// Customer Studio is the first current owner cut over to Runtime. Its
// execution path may only start a JobRun and read the current RuntimeRun
// projection; a V7 execution write would recreate a second authority.
const currentRuntimeConsumerFiles = ['internal/app/customer_studio.go','internal/app/runtime_run_projection.go'];
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
  if(source(path).includes('s.store.Runs('))fail(path,'Runtime-backed projection must use runtimeRunsForProject, not the V7 Runs table');
}

for(const path of filesUnder('internal/app',new Set(['.go']))){
  if(source(path).includes('s.store.Runs('))fail(path,'application code must not read the deleted V7 Runs table');
}

// Worker finalization may validate and publish a Runtime result reference, but
// business facts must only be materialized by the durable outbox subscriber.
const runtimeWorkerPath=join(root,'internal/app/runtime_worker.go');
for(const forbidden of ['applyRuntimeBusinessResult(','importKnowledgePackage(','CreateKnowledgeObject(']){
  if(source(runtimeWorkerPath).includes(forbidden))fail(runtimeWorkerPath,`Runtime finalize must not synchronously materialize business facts via ${forbidden}`);
}

// Runtime execution owns real host session/thread ids and a resume protocol.
// Retired one-shot adapters must not return as a second execution authority.
const codexHarnessPath=join(root,'internal/agentadapter/codex_harness.go');
if(source(codexHarnessPath).includes('--ephemeral'))fail(codexHarnessPath,'Codex Runtime Harness must persist threads for cross-process resume');
for(const path of filesUnder('internal/agentadapter',new Set(['.go']))){
  if(projectPath(path).endsWith('_test.go'))continue;
  const value=source(path);
  for(const reference of ['type Codex struct','codexRunArguments','--ephemeral']){
    if(value.includes(reference))fail(path,`production Agent adapter must not restore retired one-shot Codex path ${reference}`);
  }
}
const cliRuntimeWorkerPath=join(root,'internal/cli/runtime_worker.go');
for(const forbidden of ['agentadapter.Select(', '.Run(ctx, workspace)', 'runtime-worker:']){
  if(source(cliRuntimeWorkerPath).includes(forbidden))fail(cliRuntimeWorkerPath,`remote Runtime worker must not bypass AgentHarnessAdapter via ${forbidden}`);
}
if(source(join(root,'internal/runtime/dispatch.go')).includes('commands.AppendRuntimeEvent(ctx'))fail(join(root,'internal/runtime/dispatch.go'),'Harness events must use the fenced event command');

// RuntimeAttempt.session_ref is the only durable host-session binding. The
// retired mirror recreated session/event authority without a production user.
const retiredHarnessMirrorReferences=['DurableHarness','SessionStore','runtime_agent_sessions','runtime_agent_session_events'];
for(const path of filesUnder('internal',new Set(['.go']))){
  if(projectPath(path).endsWith('_test.go'))continue;
  const value=source(path);
  for(const reference of retiredHarnessMirrorReferences){
    if(value.includes(reference))fail(path,`production Go must not restore retired Harness mirror ${reference}`);
  }
}

// Agent handoff is the only current recovery DTO and route. The Codex-only
// facade had no production consumer and must not return as a parallel API.
const retiredCodexHandoffReferences=['contentcloud.codex-handoff','/codex-handoff','projectCodexHandoff','reviewFeedbackCodexHandoff'];
for(const path of [...filesUnder('internal',new Set(['.go'])),...filesUnder('web/src',new Set(['.ts','.tsx']))]){
  if(projectPath(path).endsWith('_test.go')||projectPath(path).endsWith('.test.ts')||projectPath(path).endsWith('.test.tsx'))continue;
  for(const reference of retiredCodexHandoffReferences){
    if(source(path).includes(reference))fail(path,`current code must not restore retired Codex-only handoff surface ${reference}`);
  }
}

// Runtime Schema lifecycle and Explorer pagination are current Repository
// capabilities. Optional repositories and slice-after-full-detail adapters
// would reintroduce compatibility paths and unbounded reads.
const runtimeRepositoryPath=join(root,'internal/runtime/repository.go');
const runtimeRepositorySource=source(runtimeRepositoryPath);
if(!runtimeRepositorySource.includes('RuntimeCommandStore'))fail(runtimeRepositoryPath,'Runtime Repository must require the transactional command store');
for(const required of ['CreateRuntimeSchema(', 'RuntimeSchema(', 'JobRunsPage(', 'NodeRunsPage(', 'JobEventsPage(', 'EffectsPage(', 'CheckpointsPage(']){
  if(!runtimeRepositorySource.includes(required))fail(runtimeRepositoryPath,`Runtime Repository must own current capability ${required}`);
}
const runtimeSchemaPath=join(root,'internal/runtime/schema.go');
if(source(runtimeSchemaPath).includes('SchemaRepository'))fail(runtimeSchemaPath,'Runtime Schema Registry must not be an optional compatibility repository');
const runtimeExplorerPath=join(root,'internal/app/runtime_explorer.go');
if(source(runtimeExplorerPath).includes('detail, err := s.RuntimeJobDetail(ctx, actor, jobID)'))fail(runtimeExplorerPath,'Runtime subresource pages must query paged Repository methods instead of slicing a full JobDetail');
const runtimeMCPGatewayPath=join(root,'internal/runtime/mcp_gateway.go');
if(source(runtimeMCPGatewayPath).includes('repo.ToolCalls(ctx'))fail(runtimeMCPGatewayPath,'MCP idempotency lookup must use the unique Repository key instead of scanning an Attempt');
if(!source(runtimeMCPGatewayPath).includes('SafeResult'))fail(runtimeMCPGatewayPath,'MCP successful replays must return the persisted safe result, not only a result digest');
const runtimeToolCallResultsMigrationPath=join(root,'migrations/00043_runtime_tool_call_results.sql');
if(!source(runtimeToolCallResultsMigrationPath).includes('ADD COLUMN safe_result jsonb'))fail(runtimeToolCallResultsMigrationPath,'ToolCall safe result persistence must be guarded by the current migration');
for(const path of filesUnder('internal/runtime',new Set(['.go']))){
  if(projectPath(path).endsWith('_test.go'))continue;
  if(source(path).includes('.(RuntimeCommandStore)'))fail(path,'current Runtime code must not keep an optional non-transactional Repository fallback');
}

// runtime_outbox is immutable after publication. Lease, retry and ack state
// belongs to one receipt per subscriber.
for(const path of filesUnder('internal/store/postgres',new Set(['.go']))){
  if(source(path).includes('UPDATE runtime_outbox SET'))fail(path,'outbox delivery state must be updated through runtime_outbox_receipts');
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
