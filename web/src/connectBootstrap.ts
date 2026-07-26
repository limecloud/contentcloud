export type ConnectSessionState = 'waiting_for_computer'|'verifying'|'connected'|'expired'|'canceled'|'failed';

export interface ConnectSession {
  id: string;
  state: ConnectSessionState;
  expires_at: string;
  connect_key?: string;
}

interface BootstrapPromptInput {
  serverURL: string;
  connectKey: string;
  projectName: string;
}

export interface ConnectStateCopy {
  title: string;
  detail: string;
  tone: 'waiting'|'progress'|'success'|'error';
}

export function buildBootstrapPrompt({serverURL,connectKey,projectName}:BootstrapPromptInput):string {
  const origin=serverURL.replace(/\/+$/,'');
  const safeProject=singleLine(projectName)||'ContentCloud project';
  return `Fetch ${origin}/api/bootstrap and initialize this ContentCloud project.\n\nserver-url: ${origin}\nconnect-key: ${singleLine(connectKey)}\nproject: ${JSON.stringify(safeProject)}`;
}

export function buildManualInstallCommand({serverURL,connectKey}:Omit<BootstrapPromptInput,'projectName'>):string {
  const origin=serverURL.replace(/\/+$/,'');
  return `npx --yes @limecloud/contentcloud@latest init . --server-url ${origin} --connect ${singleLine(connectKey)} --target all --accept-project-config`;
}

export function isActiveConnectState(state:ConnectSessionState):boolean {
  return state==='waiting_for_computer'||state==='verifying';
}

export function connectStateCopy(state:ConnectSessionState,slow=false):ConnectStateCopy {
  switch(state){
    case 'waiting_for_computer':
      return slow
        ? {title:'仍在等待 Agent',detail:'连接码仍然有效。确认 Prompt 已完整粘贴，并允许 Agent 执行初始化命令。',tone:'waiting'}
        : {title:'等待 Coding Agent',detail:'复制 Prompt 并粘贴到这个项目的 Codex 或 Claude 会话。',tone:'waiting'};
    case 'verifying':
      return {title:'正在初始化工作区',detail:'Agent 已连接，正在写入项目级 Skill、MCP 并执行 doctor。',tone:'progress'};
    case 'connected':
      return {title:'本地工作区已就绪',detail:'项目级 Agent 配置已通过检查并完成云端注册。',tone:'success'};
    case 'expired':
      return {title:'连接码已过期',detail:'生成一个新的单次连接码后再粘贴 Prompt。',tone:'error'};
    case 'canceled':
      return {title:'初始化已取消',detail:'这个连接码已经失效，没有绑定本地设备。',tone:'error'};
    case 'failed':
      return {title:'初始化未完成',detail:'查看 Agent 中的失败检查，生成新连接码后重试。',tone:'error'};
  }
}

function singleLine(value:string):string {
  return value.replace(/[\u0000-\u001f\u007f]+/g,' ').replace(/\s+/g,' ').trim().slice(0,200);
}
