import { AlertTriangle, Check, CheckCircle2, Clipboard, Clock3, LoaderCircle, Terminal } from 'lucide-react';
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
  const [promptInstruction,promptValues]=prompt.split('\n\n',2);

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
    {session.state==='waiting_for_computer'&&<>
      <p className="agent-project-context">{projectName} · 项目级 Codex / Claude 工作区</p>
      <ol className="agent-bootstrap-steps">
        <li><span>1</span><div><strong>在项目 Coding Agent 中进行</strong><p>打开用于这个项目的 Codex 或 Claude 会话，安装会在本机完成。</p></div></li>
        <li><span>2</span><div><strong>粘贴 Prompt 开始初始化</strong><section className="agent-prompt"><pre className="agent-prompt-instruction"><code>{promptInstruction}</code></pre><pre className="agent-prompt-values"><code>{promptValues}</code></pre></section><p>粘贴到同一个会话。Agent 会读取安装协议、检查目录并配置项目级 Skill 与 MCP。</p></div></li>
      </ol>
      {slow&&<Banner kind="warning">Agent 暂未连接。无需刷新页面；确认 Prompt 已完整发送并允许执行本地命令。</Banner>}
      {copyError&&<Banner kind="error" onClose={()=>setCopyError('')}>{copyError}</Banner>}
      <div className="agent-waiting-footer"><div><span className={`agent-waiting-dot ${slow?'is-slow':''}`}/><p><strong>{state.title}</strong><small>{slow?'检查 Coding Agent 是否运行在正确的项目目录，然后再次粘贴。':`连接码于 ${formatExpiry(session.expires_at)} 失效`}</small></p></div><Button className="agent-copy-button" onClick={()=>copy(prompt,'prompt')}>{copied==='prompt'?<Check size={16}/>:<Clipboard size={16}/>} {copied==='prompt'?'已复制':'复制 Prompt'}</Button></div>
      <details className="manual-install"><summary><Terminal size={15}/>改用手动安装</summary><p>仅在空目录中运行。CLI 会拒绝覆盖未知的非空目录。</p><div className="command-box"><code>{command}</code><IconButton label="复制手动安装命令" onClick={()=>copy(command,'command')}>{copied==='command'?<Check size={17}/>:<Clipboard size={17}/>}</IconButton></div></details>
    </>}

    {session.state!=='waiting_for_computer'&&<><p className="agent-project-context">{projectName} · 项目级 Codex / Claude 工作区</p><div className={`agent-bootstrap-state agent-bootstrap-state-${state.tone}`}>{icon}<div><strong>{state.title}</strong><span>{state.detail}</span></div></div></>}
    {session.state==='verifying'&&<div className="agent-verifying"><LoaderCircle className="spin" size={18}/><div><strong>等待 `workspace.register`</strong><span>只有项目级 Skill、MCP 和 doctor 全部完成后，页面才会显示成功。</span></div></div>}
    {session.state==='connected'&&<div className="agent-complete"><CheckCircle2 size={22}/><div><strong>本地负责创作，云端负责治理</strong><span>初始化没有上传已有文件，也没有自动开启 Daemon。</span></div></div>}

    {session.state!=='waiting_for_computer'&&<footer className="modal-actions">
      {session.state==='verifying'&&<Button variant="secondary" onClick={onClose}>后台等待</Button>}
      {session.state==='connected'&&<Button onClick={onClose}><Check size={16}/>完成</Button>}
      {(session.state==='expired'||session.state==='canceled'||session.state==='failed')&&<><Button variant="ghost" onClick={onClose}>关闭</Button><Button disabled={retrying} onClick={()=>void onRetry()}>{retrying?'生成中…':'生成新连接'}</Button></>}
    </footer>}
  </Modal>;
}

function formatExpiry(value:string):string {
  return new Intl.DateTimeFormat('zh-CN',{hour:'2-digit',minute:'2-digit',second:'2-digit'}).format(new Date(value));
}
