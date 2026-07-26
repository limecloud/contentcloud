import { AlertTriangle, Check, CheckCircle2, Clipboard, Clock3, LoaderCircle, ShieldCheck, Terminal } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { buildBootstrapPrompt, buildManualInstallCommand, connectStateCopy, type ConnectSession } from '../connectBootstrap';
import { Banner, Button, IconButton, Modal } from './ui';

interface InitializeWorkspaceModalProps {
  session: ConnectSession;
  projectName: string;
  serverURL: string;
  canceling: boolean;
  retrying: boolean;
  onClose: () => void;
  onCancel: () => Promise<void>;
  onRetry: () => Promise<void>;
}

export function InitializeWorkspaceModal({session,projectName,serverURL,canceling,retrying,onClose,onCancel,onRetry}:InitializeWorkspaceModalProps) {
  const [copied,setCopied]=useState<'prompt'|'command'>();
  const [copyError,setCopyError]=useState('');
  const [slow,setSlow]=useState(false);
  const prompt=useMemo(()=>buildBootstrapPrompt({serverURL,connectKey:session.connect_key||'',projectName}),[serverURL,session.connect_key,projectName]);
  const command=useMemo(()=>buildManualInstallCommand({serverURL,connectKey:session.connect_key||''}),[serverURL,session.connect_key]);
  const state=connectStateCopy(session.state,slow);

  useEffect(()=>{
    setSlow(false);
    if(session.state!=='waiting_for_computer')return;
    const timer=window.setTimeout(()=>setSlow(true),90000);
    return()=>window.clearTimeout(timer);
  },[session.id,session.state]);

  const copy=async(value:string,kind:'prompt'|'command')=>{
    setCopyError('');
    try{
      await navigator.clipboard.writeText(value);
      setCopied(kind);
      window.setTimeout(()=>setCopied(current=>current===kind?undefined:current),1600);
    }catch{
      setCopyError('无法访问剪贴板，请检查浏览器权限后重试。');
    }
  };
  const requestClose=()=>{
    if(canceling)return;
    if(session.state==='waiting_for_computer')void onCancel();
    else onClose();
  };
  const icon=state.tone==='success'?<CheckCircle2 size={20}/>:state.tone==='error'?<AlertTriangle size={20}/>:state.tone==='progress'?<LoaderCircle className="spin" size={20}/>:<Clock3 size={20}/>;

  return <Modal title="初始化本地工作区" onClose={requestClose}>
    <div className="agent-bootstrap-heading"><div className="agent-bootstrap-mark"><Terminal size={20}/></div><div><strong>{projectName}</strong><span>项目级 Codex / Claude 环境</span></div><ShieldCheck size={19}/></div>
    <div className={`agent-bootstrap-state agent-bootstrap-state-${state.tone}`}>{icon}<div><strong>{state.title}</strong><span>{state.detail}</span></div></div>

    {session.state==='waiting_for_computer'&&<>
      <ol className="agent-bootstrap-steps"><li><span>1</span><div><strong>打开项目 Agent</strong><p>进入希望保存 ContentCloud 工作区的 Codex 或 Claude 会话。</p></div></li><li><span>2</span><div><strong>粘贴 Agent Prompt</strong><p>Agent 会读取安装协议，选择安全目录并完成项目级配置。</p></div></li></ol>
      <section className="agent-prompt"><header><div><span>AGENT PROMPT</span><small>连接码于 {formatExpiry(session.expires_at)} 失效</small></div><IconButton label="复制 Agent Prompt" onClick={()=>copy(prompt,'prompt')}>{copied==='prompt'?<Check size={17}/>:<Clipboard size={17}/>}</IconButton></header><pre><code>{prompt}</code></pre></section>
      {slow&&<Banner kind="warning">Agent 暂未连接。无需刷新页面；确认 Prompt 已完整发送并允许执行本地命令。</Banner>}
      {copyError&&<Banner kind="error" onClose={()=>setCopyError('')}>{copyError}</Banner>}
      <details className="manual-install"><summary><Terminal size={15}/>改用手动安装</summary><p>仅在空目录中运行。CLI 会拒绝覆盖未知的非空目录。</p><div className="command-box"><code>{command}</code><IconButton label="复制手动安装命令" onClick={()=>copy(command,'command')}>{copied==='command'?<Check size={17}/>:<Clipboard size={17}/>}</IconButton></div></details>
    </>}

    {session.state==='verifying'&&<div className="agent-verifying"><LoaderCircle className="spin" size={18}/><div><strong>等待 `workspace.register`</strong><span>只有项目级 Skill、MCP 和 doctor 全部完成后，页面才会显示成功。</span></div></div>}
    {session.state==='connected'&&<div className="agent-complete"><CheckCircle2 size={22}/><div><strong>本地负责创作，云端负责治理</strong><span>初始化没有上传已有文件，也没有自动开启 Daemon。</span></div></div>}

    <footer className="modal-actions">
      {session.state==='waiting_for_computer'&&<><Button variant="ghost" disabled={canceling} onClick={()=>void onCancel()}>{canceling?'取消中…':'取消'}</Button><Button onClick={()=>copy(prompt,'prompt')}>{copied==='prompt'?<Check size={16}/>:<Clipboard size={16}/>} {copied==='prompt'?'已复制':'复制 Prompt'}</Button></>}
      {session.state==='verifying'&&<Button variant="secondary" onClick={onClose}>后台等待</Button>}
      {session.state==='connected'&&<Button onClick={onClose}><Check size={16}/>完成</Button>}
      {(session.state==='expired'||session.state==='canceled'||session.state==='failed')&&<><Button variant="ghost" onClick={onClose}>关闭</Button><Button disabled={retrying} onClick={()=>void onRetry()}>{retrying?'生成中…':'生成新连接'}</Button></>}
    </footer>
  </Modal>;
}

function formatExpiry(value:string):string {
  return new Intl.DateTimeFormat('zh-CN',{hour:'2-digit',minute:'2-digit',second:'2-digit'}).format(new Date(value));
}
