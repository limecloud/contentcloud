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

export const CONTENTCLOUD_CLI='npx --yes @limecloud/contentcloud@0.5.0';
export const BOOTSTRAP_PLAN_CONFIRMATION='Codex 会先展示只读计划和计划编号（plan_id）；确认后，apply 必须原样携带该 plan_id，状态变化时会要求重新确认。';

export function buildBootstrapPrompt({serverURL,connectKey,projectName}:BootstrapPromptInput):string {
  const origin=singleLine(serverURL.replace(/\/+$/,''));
  const safeProject=singleLine(projectName)||'ContentCloud project';
  return `Fetch ${origin}/api/bootstrap and follow it to connect this ContentCloud project to Codex.\n\nserver-url: ${origin}\nconnect-key: ${singleLine(connectKey)}\ncontentcloud-cli: ${CONTENTCLOUD_CLI}\nproject: ${JSON.stringify(safeProject)}`;
}

export function buildManualInstallCommand({serverURL,connectKey}:Omit<BootstrapPromptInput,'projectName'>):string {
  const origin=singleLine(serverURL.replace(/\/+$/,''));
  return `${CONTENTCLOUD_CLI} bootstrap plan . --server-url ${shellArg(origin)} --connect ${shellArg(singleLine(connectKey))} --json`;
}

export function isActiveConnectState(state:ConnectSessionState):boolean {
  return state==='waiting_for_computer'||state==='verifying';
}

export function connectStateCopy(state:ConnectSessionState,slow=false):ConnectStateCopy {
  switch(state){
    case 'waiting_for_computer':
      return slow
        ? {title:'仍在等待 Agent',detail:'连接码仍然有效。确认 Prompt 已完整粘贴，并允许 Agent 执行初始化命令。',tone:'waiting'}
        : {title:'等待 Codex',detail:'复制 Prompt 并粘贴到用于初始化这个项目的 Codex 会话。',tone:'waiting'};
    case 'verifying':
      return {title:'正在初始化创作环境',detail:'Codex 已连接，正在验证 Plugin、工作区与 doctor 结果。',tone:'progress'};
    case 'connected':
      return {title:'Codex 创作环境已就绪',detail:'Plugin 与工作区已通过检查，并完成云端注册。',tone:'success'};
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

function shellArg(value:string):string {
  return `'${value.replace(/'/g,`'\\''`)}'`;
}
