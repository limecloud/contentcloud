import { existsSync, readFileSync, readdirSync } from 'node:fs';
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
  if(!existsSync(absolute))return result;
  visit(absolute);
  return result;
}

function source(path){return readFileSync(path,'utf8')}
function projectPath(path){return relative(root,path).replaceAll('\\','/')}
function fail(path,message){failures.push(`${projectPath(path)}: ${message}`)}

const retiredInternalDirectories=[
  'agentadapter','apiclient','app','automationworkspace','blob','bootstrapcheck',
  'capabilitycatalog','capabilityrouting','channeladapter','cli','connector',
  'contentprofile','domain','environment','exportfmt','fixturev3','httpapi',
  'ingest','localconfig','localworkspace','mediapipeline','modelprovider',
  'projectview','serverconfig','sourceinfra','store','workbench','worker',
];
for(const directory of retiredInternalDirectories){
  const path=join(root,'internal',directory);
  if(existsSync(path))fail(path,'retired top-level package must not return');
}
const retiredApplicationPath=join(root,'internal/work/application');
if(existsSync(retiredApplicationPath))fail(retiredApplicationPath,'the former aggregate application package must not return; use internal/application');

const productionExtensions=new Set(['.go','.js','.mjs','.ts','.tsx']);
const productionFiles=[
  ...filesUnder('cmd',productionExtensions),
  ...filesUnder('internal',productionExtensions),
  ...filesUnder('apps',productionExtensions),
  ...filesUnder('packages',productionExtensions),
  ...filesUnder('scripts',productionExtensions),
].filter(path=>!projectPath(path).includes('/node_modules/')&&!projectPath(path).includes('/out/')&&!projectPath(path).includes('/dist/'));
const retiredImportPattern=new RegExp(`/internal/(?:${retiredInternalDirectories.join('|')})(?:/|["'])`);
for(const path of productionFiles){
  if(path===fileURLToPath(import.meta.url))continue;
  if(retiredImportPattern.test(source(path)))fail(path,'production code must not import a retired internal package');
  if(source(path).includes('/internal/work/application'))fail(path,'production code must not import the retired application path');
  if(source(path).includes('application.Service'))fail(path,'production code must use named application services, not application.Service');
  if(source(path).includes('persistence.Repository'))fail(path,'persistence must expose narrow repository ports, not persistence.Repository');
}

function checkWebBoundary(owner,forbidden){
  for(const path of filesUnder(`apps/web/src/${owner}`,new Set(['.ts','.tsx']))){
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

const developmentBootstrapPath=join(root,'apps/web/src/devBootstrap.ts');
if(!source(developmentBootstrapPath).includes('import.meta.env.DEV'))fail(developmentBootstrapPath,'development bootstrap must be removed from production builds');
for(const path of filesUnder('apps/web/src',new Set(['.ts','.tsx']))){
  if(path===developmentBootstrapPath||projectPath(path).endsWith('.test.ts')||projectPath(path).endsWith('.test.tsx'))continue;
  if(source(path).includes('/api/v1/dev/bootstrap'))fail(path,'production web entry must not call the development bootstrap endpoint');
}

const runtimeLegacyRunReferences=['TaskRun','RunAttempt','task_runs','run_attempts'];
for(const path of filesUnder('internal/runtime',new Set(['.go']))){
  const value=source(path);
  const isWorkerComposition=projectPath(path).startsWith('internal/runtime/worker/');
  for(const reference of runtimeLegacyRunReferences){
    if(value.includes(reference))fail(path,`current runtime must not depend on V7 compatibility reference ${reference}`);
  }
  if(!isWorkerComposition&&value.includes('/internal/application'))fail(path,'runtime core must depend on contracts and ports, not the business application service');
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
const bootstrapPath=join(root,'internal/application/bootstrap_onboarding.go');
const bootstrapSource=source(bootstrapPath);
for(const required of ['localworkspace.TemplateID','localworkspace.TemplateVersion']){
  if(!bootstrapSource.includes(required))fail(bootstrapPath,`bootstrap must use current workspace template constant ${required}`);
}
for(const path of filesUnder('internal',new Set(['.go']))){
  const relativePath=projectPath(path);
  if(relativePath.endsWith('_test.go')||relativePath==='internal/transport/cli/plugin_host.go')continue;
  if(source(path).includes('/integration/pluginbuiltin'))fail(path,'only the CLI installation boundary may depend on the bundled Plugin loader; identity consumers must use pluginidentity');
}

// Local daemon bindings are current in daemon_bindings. The retired top-level
// single-workspace config is deleted and must not return as a migration path.
const localConfigPath=join(root,'internal/local/config/config.go');
const localConfigSource=source(localConfigPath);
for(const required of ['func (c Config) Bindings()','func (c Config) PrimaryBinding()']){
	if(!localConfigSource.includes(required))fail(localConfigPath,`current local config boundary is missing ${required}`);
}
for(const retired of ['type configFile struct','hasLegacyBinding','containsLegacyKeys','WorkspaceRoot','RuntimeBindings()']){
	if(localConfigSource.includes(retired))fail(localConfigPath,`local config must not restore retired single-workspace migration surface ${retired}`);
}
for(const path of filesUnder('internal/transport/cli',new Set(['.go']))){
  if(projectPath(path).endsWith('_test.go'))continue;
  const value=source(path);
  for(const reference of ['RuntimeBindings()','cfg.DeviceID','cfg.WorkspaceID','cfg.ProjectID','cfg.WorkspaceRoot']){
    if(value.includes(reference))fail(path,`CLI must use current DaemonBindings instead of legacy config reference ${reference}`);
  }
}

// Built-in SOP identity is explicit. Name/shape matching and metadata adoption
// were pre-user migration paths and must not become runtime behavior again.
const builtinSOPPath=join(root,'internal/application/builtin_sops.go');
const builtinSOPSource=source(builtinSOPPath);
for(const retired of ['migrateLegacyBuiltinSOPs','matchLegacyShortVideo','adoptBuiltinDefinition','sop.legacy_migrated']){
	if(builtinSOPSource.includes(retired))fail(builtinSOPPath,`built-in SOPs must not restore retired migration surface ${retired}`);
}

// The server KnowledgePack is a domain aggregate, while knowledge-pack/3.0 is
// the local exchange contract. A fabricated 1.0 exchange schema must not return.
for(const path of filesUnder('internal',new Set(['.go']))){
	if(source(path).includes('contentcloud.knowledge-pack/1.0'))fail(path,'production Go must not restore the retired KnowledgePack 1.0 schema marker');
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
for(const path of filesUnder('apps/web/src',new Set(['.ts','.tsx']))){
  const value=source(path);
  for(const name of [...retiredRuntimeReadNames,'taskRun']){
    if(value.includes(name))fail(path,`web code must not restore retired Runtime read name ${name}`);
  }
}

// Customer Studio is the first current owner cut over to Runtime. Its
// execution path may only start a JobRun and read the current RuntimeRun
// projection; a V7 execution write would recreate a second authority.
const currentRuntimeConsumerFiles = ['internal/application/customer_studio.go','internal/application/runtime_run_projection.go'];
const legacyWriteCalls = ['CreateRun(', 'CreateRunWithBundle(', 'SaveRun(', 'CreateRunAttempt(', 'SaveRunAttempt(', 'LeaseNextRun('];
for(const relativePath of currentRuntimeConsumerFiles){
  const path=join(root,relativePath);
  const value=source(path);
  for(const call of legacyWriteCalls){
    if(value.includes(call))fail(path,`current Runtime consumer must not write V7 execution fact ${call}`);
  }
}

// Runtime-backed read models must not silently fall back to the V7 Runs table.
for(const relativePath of ['internal/application/projection.go','internal/application/lineage.go']){
  const path=join(root,relativePath);
  if(source(path).includes('s.store.Runs('))fail(path,'Runtime-backed projection must use runtimeRunsForProject, not the V7 Runs table');
}

for(const path of filesUnder('internal/application',new Set(['.go']))){
  if(source(path).includes('s.store.Runs('))fail(path,'application code must not read the deleted V7 Runs table');
}

// Worker finalization may validate and publish a Runtime result reference, but
// business facts must only be materialized by the durable outbox subscriber.
const runtimeWorkerPath=join(root,'internal/application/runtime_worker.go');
for(const forbidden of ['applyRuntimeBusinessResult(','importKnowledgePackage(','CreateKnowledgeObject(']){
  if(source(runtimeWorkerPath).includes(forbidden))fail(runtimeWorkerPath,`Runtime finalize must not synchronously materialize business facts via ${forbidden}`);
}

// Runtime execution owns real host session/thread ids and a resume protocol.
// Retired one-shot adapters must not return as a second execution authority.
const codexHarnessPath=join(root,'internal/integration/agent/codex_harness.go');
if(source(codexHarnessPath).includes('--ephemeral'))fail(codexHarnessPath,'Codex Runtime Harness must persist threads for cross-process resume');
for(const path of filesUnder('internal/integration/agent',new Set(['.go']))){
  if(projectPath(path).endsWith('_test.go'))continue;
  const value=source(path);
  for(const reference of ['type Codex struct','codexRunArguments','--ephemeral']){
    if(value.includes(reference))fail(path,`production Agent adapter must not restore retired one-shot Codex path ${reference}`);
  }
}

// Open execution is one Runtime Harness contract. Remote Agent/SaaS/Pi
// wrappers must register through it instead of adding provider-specific task
// state or bypassing RuntimeAttempt.
const harnessRegistrySource=source(join(root,'internal/integration/agent/harness_registry.go'));
if(!harnessRegistrySource.includes('mustRegister("remote-http"'))fail(join(root,'internal/integration/agent/harness_registry.go'),'provider-neutral remote Agent Harness must remain registered');
const remoteHarnessSource=source(join(root,'internal/integration/agent/remote_http_harness.go'));
for(const required of ['func (h *RemoteHTTPHarness) Detect(','func (h *RemoteHTTPHarness) Start(','func (h *RemoteHTTPHarness) Resume(','func (h *RemoteHTTPHarness) Interrupt(','func (h *RemoteHTTPHarness) Inspect(']){
  if(!remoteHarnessSource.includes(required))fail(join(root,'internal/integration/agent/remote_http_harness.go'),`remote Agent Harness is missing ${required}`);
}

// Remote Agent callbacks are durable RuntimeAttempt input, not business
// Effects or a second session/callback aggregate.
const agentCallbackPath=join(root,'internal/runtime/agent_callback.go');
const agentCallbackSource=source(agentCallbackPath);
for(const required of ['ReceiveAgentInboxCommand(','CompleteAgentInboxCommand(','s.FinalizeDispatch(','s.RecordHarnessEvent(','s.YieldDispatch(']){
  if(!agentCallbackSource.includes(required))fail(agentCallbackPath,`durable Agent callback boundary is missing ${required}`);
}
for(const forbidden of ['ExternalEffect','ProviderReconciliation']){
  if(agentCallbackSource.includes(forbidden))fail(agentCallbackPath,`Agent callback must remain bound to RuntimeAttempt and cannot create ${forbidden}`);
}
const runtimeProviderStorePaths=[join(root,'internal/persistence/memory/runtime_provider.go'),join(root,'internal/persistence/postgres/runtime_provider.go')];
for(const path of runtimeProviderStorePaths){
  const value=source(path);
  for(const required of ['ReceiveAgentInboxCommand(','CompleteAgentInboxCommand(']){
    if(!value.includes(required))fail(path,`shared Runtime inbox must implement ${required}`);
  }
}
for(const path of filesUnder('migrations',new Set(['.sql']))){
  if(/(?:agent|harness)_(?:callback|inbox)/i.test(source(path)))fail(path,'Agent callbacks must reuse runtime_provider_inbox instead of adding a parallel table');
}
const agentIngressPath=join(root,'internal/transport/http/agent_ingress.go');
if(!source(agentIngressPath).includes('verifySignedIngress('))fail(agentIngressPath,'Agent ingress must use the shared HMAC and replay verifier');
const serverMainSource=source(join(root,'cmd/contentcloud-server/main.go'));
if(!serverMainSource.includes('CONTENTCLOUD_AGENT_CALLBACK_SECRETS'))fail(join(root,'cmd/contentcloud-server/main.go'),'Agent callback secrets must have an independent environment scope');

// vLLM and SGLang share one OpenAI-compatible model boundary; provider names
// are bindings, not separate application pipelines.
const modelProviderSource=source(join(root,'internal/integration/provider/model/openai_compatible.go'));
for(const required of ['func NewVLLM(','func NewSGLang(','/v1/chat/completions']){
  if(!modelProviderSource.includes(required))fail(join(root,'internal/integration/provider/model/openai_compatible.go'),`OpenAI-compatible model adapter is missing ${required}`);
}

// A model provider returns a candidate, never an approval fact. The only
// durable write is an atomic draft TaskRevision plus its immutable receipt.
const modelInfraPath=join(root,'internal/application/model_infra.go');
const modelInfraSource=source(modelInfraPath);
for(const required of ['Status: reviewdomain.TaskRevisionDraft','CreateModelGeneratedRevision(ctx, revision, receipt)']){
  if(!modelInfraSource.includes(required))fail(modelInfraPath,`model candidate boundary is missing ${required}`);
}
for(const forbidden of ['ApprovedSnapshot','CreateApproved','SaveApproved']){
  if(modelInfraSource.includes(forbidden))fail(modelInfraPath,`model candidate must not create approval authority via ${forbidden}`);
}

// Search/fetch results must enter the existing SourceRevision chain. A second
// source asset aggregate would split evidence lineage.
const sourceInfraAppSource=source(join(root,'internal/application/source_infra.go'));
if(!sourceInfraAppSource.includes('s.uploadSource('))fail(join(root,'internal/application/source_infra.go'),'source.fetch must persist through the existing Source/SourceRevision boundary');
if(sourceInfraAppSource.includes('CreateFetchedSource'))fail(join(root,'internal/application/source_infra.go'),'source.fetch must not introduce a parallel fetched-source aggregate');
const connectorInfraPath=join(root,'internal/application/connector_infra.go');
const connectorInfraSource=source(connectorInfraPath);
for(const required of ['s.UploadSourceRevision(','s.uploadSource(','next.SourceID, next.RevisionID = revision.SourceID, revision.ID']){
  if(!connectorInfraSource.includes(required))fail(connectorInfraPath,`Connector records must materialize through SourceRevision; missing ${required}`);
}
for(const forbidden of ['CreateConnectorSource','ConnectorArtifact','ConnectorContent']){
  if(connectorInfraSource.includes(forbidden))fail(connectorInfraPath,`Connector must not own a parallel content fact via ${forbidden}`);
}
const cliRuntimeWorkerPath=join(root,'internal/transport/cli/runtime_worker.go');
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
const retiredHarnessCreationMigration=join(root,'migrations/00030_runtime_session_store.sql');
const retiredHarnessRemovalMigration=join(root,'migrations/00036_remove_runtime_session_mirror.sql');
if(!existsSync(retiredHarnessCreationMigration)||!existsSync(retiredHarnessRemovalMigration)){
  fail(retiredHarnessRemovalMigration,'applied migration history is immutable and must retain both Session Mirror migrations');
}else{
  const removal=source(retiredHarnessRemovalMigration);
  for(const required of ['DROP TABLE IF EXISTS runtime_agent_session_events','DROP TABLE IF EXISTS runtime_agent_sessions']){
    if(!removal.includes(required))fail(retiredHarnessRemovalMigration,`retired Session Mirror removal migration must contain ${required}`);
  }
}

// Agent handoff is the only current recovery DTO and route. The Codex-only
// facade had no production consumer and must not return as a parallel API.
const retiredCodexHandoffReferences=['contentcloud.codex-handoff','/codex-handoff','projectCodexHandoff','reviewFeedbackCodexHandoff'];
for(const path of [...filesUnder('internal',new Set(['.go'])),...filesUnder('apps/web/src',new Set(['.ts','.tsx']))]){
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
const runtimeExplorerPath=join(root,'internal/application/runtime_explorer.go');
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
for(const path of filesUnder('internal/persistence/postgres',new Set(['.go']))){
  if(source(path).includes('UPDATE runtime_outbox SET'))fail(path,'outbox delivery state must be updated through runtime_outbox_receipts');
}

const legacyImports=['/internal/application','/internal/persistence','/internal/transport/http'];
for(const path of filesUnder('internal/experience',new Set(['.go']))){
  const value=source(path);
  for(const legacy of legacyImports){
    if(value.includes(legacy))fail(path,`target module must not import deprecated or compatibility package ${legacy}`);
  }
}

for(const path of filesUnder('internal/experience',new Set(['.go']))){
  const value=source(path);
  if(value.includes('/internal/work'))fail(path,'target experience module must use its own records or stable platform values, not legacy domain');
}

const repositoryPath=join(root,'internal/persistence/repository.go');
const repositorySource=source(repositoryPath);
const repositoryInterfaces=[
  'IdentityRepository','WorkspaceRepository','SourceRepository','KnowledgeRepository',
  'CatalogRepository','WorkRepository','DeliveryRepository','ContextRepository',
  'ReviewRepository','ArtifactRepository','PerformanceRepository','AuditRepository',
];
if(/type\s+Repository\s+interface\s*{/.test(repositorySource))fail(repositoryPath,'aggregate Repository interface must not return; compose explicit Dependencies from narrow ports');
for(const name of repositoryInterfaces){
	if(!repositorySource.includes(`type ${name} interface {`))fail(repositoryPath,`narrow repository interface is missing ${name}`);
}
if(/type\s+Store\s+interface/.test(repositorySource))fail(repositoryPath,'global Store interface must not return');
for(const path of filesUnder('internal/persistence',new Set(['.go']))){
  if(/^package\s+store\b/m.test(source(path)))fail(path,'persistence packages must not retain the historical store package name');
}

// Desktop is a persistent project surface, not a second business backend or
// an embedded Codex renderer. Renderer -> Preload -> Main -> local Go API is
// the only allowed authority path.
const desktopPath=join(root,'apps/desktop');
if(existsSync(desktopPath)){
  const rendererFiles=filesUnder('apps/desktop/src/renderer',new Set(['.ts','.tsx']));
  for(const path of rendererFiles){
    const value=source(path);
    if(/from\s+['"](?:electron|node:|fs(?:\/|['"]))/m.test(value))fail(path,'Desktop Renderer must not import Electron or Node APIs');
    if(/(?:\bfetch\s*\(|new\s+WebSocket\s*\(|new\s+EventSource\s*\()/m.test(value))fail(path,'Desktop Renderer must use the typed preload API instead of direct network access');
    if(/<(?:webview|iframe)\b/i.test(value))fail(path,'Desktop Renderer must not embed another rendering surface');
    if(/\.\.\/(?:main|preload)(?:\/|['"])/.test(value))fail(path,'Desktop Renderer must not import Main or Preload code');
  }

  const preloadPath=join(root,'apps/desktop/src/preload/preload.ts');
  const preloadSource=source(preloadPath);
  for(const forbidden of ['ipcRenderer.send(', 'ipcRenderer.sendSync(', 'shell.', 'webFrame.', 'remote.']){
    if(preloadSource.includes(forbidden))fail(preloadPath,`Preload must not expose broad Electron capability via ${forbidden}`);
  }
  if(/ipcRenderer\.(?:invoke|on)\((?!['"]desktop\.)/.test(preloadSource))fail(preloadPath,'Preload IPC channels must be literal desktop.* channels');
  if(!preloadSource.includes("contextBridge.exposeInMainWorld('contentcloudDesktop', api)"))fail(preloadPath,'Preload must expose one typed contentcloudDesktop API');
  if(/\.\.\/(?:main|renderer)(?:\/|['"])/.test(preloadSource))fail(preloadPath,'Preload must depend only on shared contracts');

  const mainPath=join(root,'apps/desktop/src/main/main.ts');
  const mainSource=source(mainPath);
  for(const required of [
    'sandbox: true','contextIsolation: true','nodeIntegration: false','webSecurity: true',
    "setWindowOpenHandler(() => ({ action: 'deny' }))",'will-attach-webview',
    'setPermissionRequestHandler',
  ]){
    if(!mainSource.includes(required))fail(mainPath,`Electron Main security baseline is missing ${required}`);
  }
  if(/from\s+['"]\.\.\/(?:preload|renderer)(?:\/|['"])/.test(mainSource))fail(mainPath,'Electron Main must not import Preload or Renderer code');

  const desktopSources=filesUnder('apps/desktop/src',new Set(['.ts','.tsx']));
  for(const path of desktopSources){
    const value=source(path);
    if(/(?:better-sqlite3|node:sqlite|sqlite3)/i.test(value))fail(path,'Electron must not own a SQLite business store; local persistence belongs to the Go daemon');
    if(/(?:apps\/web|\/internal\/)/.test(value))fail(path,'Desktop must not import Web or Go implementation modules across surfaces');
  }
  const sharedContractsPath=join(root,'apps/desktop/src/shared/contracts.ts');
  for(const forbidden of ['capability','device_token','absolute_path']){
    if(source(sharedContractsPath).toLowerCase().includes(forbidden))fail(sharedContractsPath,`Renderer contract must not expose ${forbidden}`);
  }
  const desktopPackagePath=join(root,'apps/desktop/package.json');
  const desktopPackage=JSON.parse(source(desktopPackagePath));
  for(const dependency of Object.keys({...desktopPackage.dependencies,...desktopPackage.devDependencies})){
    if(/sqlite/i.test(dependency))fail(desktopPackagePath,`Electron package must not add SQLite dependency ${dependency}`);
  }
  const indexPath=join(root,'apps/desktop/index.html');
  const indexSource=source(indexPath);
  if(!indexSource.includes('Content-Security-Policy'))fail(indexPath,'Desktop Renderer must define a CSP');
  if(!indexSource.includes("connect-src 'none'"))fail(indexPath,'Desktop Renderer CSP must block direct network access');
}

for(const path of filesUnder('apps/web/src',new Set(['.ts','.tsx']))){
  if(source(path).includes('apps/desktop'))fail(path,'Web must not import Desktop implementation code');
}
for(const path of filesUnder('internal',new Set(['.go']))){
  const value=source(path);
  if((value.includes('"database/sql"')||value.includes('modernc.org/sqlite'))&&!projectPath(path).startsWith('internal/local/')){
    fail(path,'SQLite is restricted to rebuildable local indexes, caches, outbox, parts, and cursors');
  }
}

// Secrets are references at durable boundaries. Adapters may resolve an
// environment token in memory, but persisted bindings must never expose it.
for(const relativePath of ['internal/integration/connector/sync.go','internal/application/connector_infra.go','internal/application/channel_infra.go']){
  const path=join(root,relativePath);
  const value=source(path);
  if(/json:\"(?:token|api_key|password|credential)\"/i.test(value))fail(path,'durable infra contract must store a SecretRef, not plaintext credentials');
}
const connectorContractPath=join(root,'internal/integration/connector/sync.go');
const connectorContractSource=source(connectorContractPath);
for(const required of ['AuthorizationRef string','validSecretRef(v.AuthorizationRef)']){
  if(!connectorContractSource.includes(required))fail(connectorContractPath,`Connector binding must enforce opaque SecretRef through ${required}`);
}

// Content Profiles compile into the existing SOP. They describe capabilities
// and executor kinds, never duplicate business aggregates or vendor stages.
const contentProfilePath=join(root,'internal/catalog/profile/profile.go');
const contentProfileSource=source(contentProfilePath);
for(const forbiddenType of ['WorkTask','Submission','Artifact','TaskRevision','ApprovedSnapshot']){
  if(new RegExp(`type\\s+${forbiddenType}\\s+struct`).test(contentProfileSource))fail(contentProfilePath,`Content Profile must reuse existing ${forbiddenType} instead of defining a parallel aggregate`);
}
for(const forbiddenBrand of ['codex','claude','vllm','sglang','openai','anthropic']){
  const quotedBrand=new RegExp(`[\"'](?:[^\"']*[._ -])?${forbiddenBrand}(?:[._ -][^\"']*)?[\"']`,'i');
  if(quotedBrand.test(contentProfileSource))fail(contentProfilePath,`Content Profile business stages must reference capabilities/executor kinds, not provider brand ${forbiddenBrand}`);
}
if(!contentProfileSource.includes('"agent_saas"'))fail(contentProfilePath,'Content Profiles must allow provider-neutral Agent SaaS execution');
if(!contentProfileSource.includes('ExecutorKinds'))fail(contentProfilePath,'Content Profiles must declare executor kinds independently from Agent brands');

// Published is an externally observed state. Direct assignments would let an
// internal command forge a delivery receipt and prematurely complete a task.
const channelInfraPath=join(root,'internal/application/channel_infra.go');
const channelInfraSource=source(channelInfraPath);
for(const pattern of [/State:\s*domain\.ChannelPublicationPublished/,/\.State\s*=\s*domain\.ChannelPublicationPublished/]){
  if(pattern.test(channelInfraSource))fail(channelInfraPath,'Channel published state must come from an Adapter receipt, manual receipt, or signed callback');
}
for(const required of ['applyChannelReceipt(&value, receipt, now)','RecordManualChannelReceipt(','ReceiveChannelCallback(','ApplyChannelCallback(ctx, value, receipt)']){
  if(!channelInfraSource.includes(required))fail(channelInfraPath,`Channel receipt boundary is missing ${required}`);
}

// Creating a delivery is a preparation step. Only an explicit local delivery
// or a published channel receipt may advance it to delivered.
const taskGovernancePath=join(root,'internal/application/task_governance.go');
const taskGovernanceSource=source(taskGovernancePath);
if(!taskGovernanceSource.includes('Status: deliverydomain.TaskDeliveryReady'))fail(taskGovernancePath,'TaskDelivery must default to ready');
if(/TaskDelivery\{[^}]*Status:\s*deliverydomain\.TaskDeliveryDelivered/s.test(taskGovernanceSource))fail(taskGovernancePath,'TaskDelivery initializer must not default to delivered');
if(!taskGovernanceSource.includes('deliver := input.Deliver != nil && *input.Deliver'))fail(taskGovernancePath,'local delivered transition must remain an explicit caller choice');

if(failures.length){
  console.error('Architecture checks failed:\n'+failures.map(value=>`- ${value}`).join('\n'));
  process.exit(1);
}
console.log(`Architecture checks passed (${repositoryInterfaces.length} narrow persistence repositories; Desktop surface boundaries enforced).`);
