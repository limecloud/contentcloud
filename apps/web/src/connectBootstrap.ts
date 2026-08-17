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

export const CONTENTCLOUD_CLI='npx --yes @limecloud/contentcloud@0.28.0';
export const BOOTSTRAP_PLAN_CONFIRMATION='Codex 会先展示准备执行的步骤和编号（plan_id）；你确认后，Codex 才会执行 apply。电脑状态变化时，系统会要求重新确认。';

const stageNames:Record<string,string>={
  prerequisites:'检查电脑',codex_ready:'检查连接工具',network_ready:'检查网络',workspace_selected:'选择项目文件夹',
  plan_ready:'准备连接步骤',awaiting_confirmation:'等待确认',plugin_installing:'准备连接功能',authorizing:'确认这台电脑',
  workspace_initializing:'准备项目文件夹',doctor_running:'检查项目文件夹',registering:'记录电脑连接',opening_desktop:'打开连接页面',complete:'连接准备完成'
};

export function buildBootstrapPrompt({serverURL,sessionID,projectName}:BootstrapPromptInput):string {
  const origin=singleLine(serverURL.replace(/\/+$/,''));
  const safeProject=singleLine(projectName)||'Content Work OS 项目';
  return `请读取 ${origin}/api/bootstrap，并按照其中的步骤在 Codex 中把这台电脑连接到 Content Work OS 项目。\n\nserver-url: ${origin}\nsession-id: ${singleLine(sessionID)}\ncontentcloud-cli: ${CONTENTCLOUD_CLI}\nproject: ${JSON.stringify(safeProject)}`;
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
    const stage=stageNames[progress.stage]||'准备创作工具';
    if(progress.status==='failed')return {title:`${stage}未通过`,detail:progress.action?.body||progress.error_code||'请根据支持码联系支持人员。',tone:'error'};
    if(progress.status==='needs_action')return {title:progress.action?.title||stage,detail:progress.action?.body||'完成当前操作后，Codex 会自动继续。',tone:'waiting'};
    if(session.state==='connected')return {title:'工作电脑已连接',detail:'项目文件夹已通过检查，可以开始创作。',tone:'success'};
    return {title:stage,detail:`正在执行第 ${progress.step} / ${progress.step_count} 步。`,tone:'progress'};
  }
  switch(session.state){
    case 'waiting_for_computer':
      return slow
        ? {title:'仍在等待连接工具',detail:'连接仍然有效。请确认连接文字已完整粘贴，并允许工具进行只读检查。',tone:'waiting'}
        : {title:'等待连接工具',detail:'复制连接文字，粘贴到电脑上的连接工具中。',tone:'waiting'};
    case 'verifying':
      return {title:'正在连接工作电脑',detail:'连接工具已获授权，正在检查项目文件夹和连接状态。',tone:'progress'};
    case 'connected':
      return {title:'工作电脑已连接',detail:'项目文件夹已通过检查，可以开始创作。',tone:'success'};
    case 'expired':
      return {title:'连接已过期',detail:'请重新发起连接。',tone:'error'};
    case 'canceled':
      return {title:'连接已取消',detail:'这次连接没有绑定电脑。',tone:'error'};
    case 'failed':
      return {title:'连接没有完成',detail:'请按提示处理后重新发起连接。',tone:'error'};
  }
}

function singleLine(value:string):string {
  return value.replace(/[\u0000-\u001f\u007f]+/g,' ').replace(/\s+/g,' ').trim().slice(0,200);
}

function shellArg(value:string):string {
  return `'${value.replace(/'/g,`'\\''`)}'`;
}
