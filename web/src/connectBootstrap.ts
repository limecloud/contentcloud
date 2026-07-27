export type ConnectSessionState = 'waiting_for_computer'|'verifying'|'connected'|'expired'|'canceled'|'failed';
export type BootstrapProgressStatus = 'started'|'passed'|'needs_action'|'failed'|'skipped';

export interface BootstrapAction {
  action_id: string;
  kind: 'retry_check'|'open_guide'|'open_browser_auth'|'open_codex'|'choose_directory'|'run_managed_repair'|'copy_fixed_command'|'create_diagnostic_bundle'|'contact_support';
  title: string;
  body: string;
  doc_url?: string;
  handler?: string;
  requires_confirmation: boolean;
  recheck: string[];
}

export interface BootstrapProgress {
  attempt_id: string;
  stage: string;
  status: BootstrapProgressStatus;
  step: number;
  step_count: number;
  check_id?: string;
  error_code?: string;
  action_id?: string;
  action?: BootstrapAction;
  support_code: string;
  user_code?: string;
  updated_at: string;
}

export interface ConnectSession {
  id: string;
  project_id: string;
  state: ConnectSessionState;
  expires_at: string;
  progress?: BootstrapProgress;
}

export interface BootstrapAttempt {
  id: string;
  connect_session_id: string;
  user_code: string;
  state: 'pending'|'approved'|'denied'|'consumed'|'completed'|'failed';
  support_code: string;
  expires_at: string;
}

export interface BootstrapAuthorizationView {
  attempt: BootstrapAttempt;
  session: ConnectSession;
}

interface BootstrapPromptInput {
  serverURL: string;
  sessionID: string;
  projectName: string;
}

export interface BootstrapCommands {
  preflight: string;
  plan: string;
  resume: string;
  diagnostics?: string;
}

export interface ConnectStateCopy {
  title: string;
  detail: string;
  tone: 'waiting'|'progress'|'success'|'error';
}

export const CONTENTCLOUD_CLI='npx --yes @limecloud/contentcloud@0.6.0';
export const BOOTSTRAP_PLAN_CONFIRMATION='Codex 会先展示只读计划和计划编号（plan_id）；确认后，apply 必须原样携带该 plan_id，状态变化时会要求重新确认。';

const stageNames:Record<string,string>={
  prerequisites:'检查本机环境',codex_ready:'检查 Codex',network_ready:'检查网络',workspace_selected:'选择工作区',
  plan_ready:'准备变更计划',awaiting_confirmation:'等待确认',plugin_installing:'安装 ContentCloud Plugin',authorizing:'确认这台电脑',
  workspace_initializing:'初始化工作区',doctor_running:'验证工作区',registering:'注册工作区',opening_desktop:'打开新对话',complete:'初始化完成'
};

export function buildBootstrapPrompt({serverURL,sessionID,projectName}:BootstrapPromptInput):string {
  const origin=singleLine(serverURL.replace(/\/+$/,''));
  const safeProject=singleLine(projectName)||'ContentCloud project';
  return `Fetch ${origin}/api/bootstrap and follow it to initialize this ContentCloud project in Codex.\n\nserver-url: ${origin}\nsession-id: ${singleLine(sessionID)}\ncontentcloud-cli: ${CONTENTCLOUD_CLI}\nproject: ${JSON.stringify(safeProject)}`;
}

export function buildBootstrapCommands({serverURL,sessionID,attemptID}:Omit<BootstrapPromptInput,'projectName'>&{attemptID?:string}):BootstrapCommands {
  const origin=shellArg(singleLine(serverURL.replace(/\/+$/,'')));
  const directory='.';
  const commands:BootstrapCommands={
    preflight:`${CONTENTCLOUD_CLI} bootstrap preflight ${directory} --server-url ${origin} --json`,
    plan:`${CONTENTCLOUD_CLI} bootstrap plan ${directory} --server-url ${origin} --session ${shellArg(singleLine(sessionID))} --json`,
    resume:`${CONTENTCLOUD_CLI} bootstrap resume ${directory} --accept --json`
  };
  if(attemptID)commands.diagnostics=`${CONTENTCLOUD_CLI} bootstrap diagnostics ${directory} --attempt ${shellArg(singleLine(attemptID))} --json`;
  return commands;
}

export function isActiveConnectState(state:ConnectSessionState):boolean {
  return state==='waiting_for_computer'||state==='verifying';
}

export function connectStateCopy(session:ConnectSession,slow=false):ConnectStateCopy {
  const progress=session.progress;
  if(progress){
    const stage=stageNames[progress.stage]||'初始化创作环境';
    if(progress.status==='failed')return {title:`${stage}未通过`,detail:progress.action?.body||progress.error_code||'请根据支持码联系支持人员。',tone:'error'};
    if(progress.status==='needs_action')return {title:progress.action?.title||stage,detail:progress.action?.body||'完成当前操作后，Codex 会自动继续。',tone:'waiting'};
    if(session.state==='connected')return {title:'Codex 创作环境已就绪',detail:'Plugin 与工作区已通过检查，并完成云端注册。',tone:'success'};
    return {title:stage,detail:`正在执行第 ${progress.step} / ${progress.step_count} 步。`,tone:'progress'};
  }
  switch(session.state){
    case 'waiting_for_computer':
      return slow
        ? {title:'仍在等待 Codex',detail:'会话仍然有效。确认 Prompt 已完整粘贴，并允许 Codex 执行只读检查。',tone:'waiting'}
        : {title:'等待 Codex',detail:'复制 Prompt 并粘贴到用于初始化这个项目的 Codex 会话。',tone:'waiting'};
    case 'verifying':
      return {title:'正在初始化创作环境',detail:'Codex 已获授权，正在验证 Plugin、工作区与 doctor 结果。',tone:'progress'};
    case 'connected':
      return {title:'Codex 创作环境已就绪',detail:'Plugin 与工作区已通过检查，并完成云端注册。',tone:'success'};
    case 'expired':
      return {title:'初始化会话已过期',detail:'创建新的初始化会话后重试。',tone:'error'};
    case 'canceled':
      return {title:'初始化已取消',detail:'这个会话已经失效，没有绑定本地设备。',tone:'error'};
    case 'failed':
      return {title:'初始化未完成',detail:'查看失败检查和支持码，解决后重新发起。',tone:'error'};
  }
}

function singleLine(value:string):string {
  return value.replace(/[\u0000-\u001f\u007f]+/g,' ').replace(/\s+/g,' ').trim().slice(0,200);
}

function shellArg(value:string):string {
  return `'${value.replace(/'/g,`'\\''`)}'`;
}
