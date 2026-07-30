import { AlertTriangle, BookOpen, Check, CheckCircle2, Clipboard, Clock3, ExternalLink, LoaderCircle, ShieldCheck, Terminal, XCircle } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { BOOTSTRAP_PLAN_CONFIRMATION, buildBootstrapCommands, buildBootstrapPrompt, connectStateCopy, type ConnectSession } from '../connectBootstrap';
import { Banner, Button, IconButton, Modal } from './ui';

interface InitializeWorkspaceModalProps {
  session: ConnectSession;
  projectName: string;
  serverURL: string;
  canceling: boolean;
  retrying: boolean;
  approving: boolean;
  denying: boolean;
  onClose: () => void;
  onCancel: () => Promise<void>;
  onRetry: () => Promise<void>;
  onApprove: () => Promise<void>;
  onDeny: () => Promise<void>;
}

type CopyKind='prompt'|'preflight'|'plan'|'resume'|'diagnostics';

export function InitializeWorkspaceModal({session,projectName,serverURL,canceling,retrying,approving,denying,onClose,onCancel,onRetry,onApprove,onDeny}:InitializeWorkspaceModalProps) {
  const [copied,setCopied]=useState<CopyKind>();
  const [copyError,setCopyError]=useState('');
  const [slow,setSlow]=useState(false);
  const progress=session.progress;
  const prompt=useMemo(()=>buildBootstrapPrompt({serverURL,sessionID:session.id,projectName}),[serverURL,session.id,projectName]);
  const commands=useMemo(()=>buildBootstrapCommands({serverURL,sessionID:session.id,attemptID:progress?.attempt_id}),[serverURL,session.id,progress?.attempt_id]);
  const state=connectStateCopy(session,slow);
  const [promptInstruction,promptValues]=prompt.split('\n\n',2);
  const needsApproval=progress?.stage==='authorizing'&&progress.status==='needs_action'&&progress.action_id==='open.browser.authorization';
  const actionCommand=progress?.action?.handler==='bootstrap_resume'||progress?.action?.handler==='copy_bootstrap_resume'?commands.resume:progress?.action?.handler==='create_diagnostic_bundle'?commands.diagnostics:undefined;

  useEffect(()=>{
    setSlow(false);
    if(session.state!=='waiting_for_computer'||progress)return;
    const timer=window.setTimeout(()=>setSlow(true),90000);
    return()=>window.clearTimeout(timer);
  },[session.id,session.state,progress]);

  const copy=async(value:string,kind:CopyKind)=>{
    setCopyError('');
    try{
      await navigator.clipboard.writeText(value);
      setCopied(kind);
      window.setTimeout(()=>setCopied(current=>current===kind?undefined:current),1600);
    }catch{
      setCopyError('无法访问剪贴板，请检查浏览器权限后重试。');
    }
  };
  const icon=state.tone==='success'?<CheckCircle2 size={20}/>:state.tone==='error'?<AlertTriangle size={20}/>:state.tone==='progress'?<LoaderCircle className="spin" size={20}/>:<Clock3 size={20}/>;

  return <Modal title="初始化本地工作区" onClose={onClose}>
    <p className="agent-project-context">{projectName} · Codex 创作环境</p>
    {!progress&&session.state==='waiting_for_computer'&&<>
      <ol className="agent-bootstrap-steps">
        <li><span>1</span><div><strong>在 Codex 中开始</strong><p>打开一个用于初始化的 Codex 会话，本机检查和安装会由固定版本 CLI 完成。</p></div></li>
        <li><span>2</span><div><strong>粘贴 Prompt</strong><section className="agent-prompt"><pre className="agent-prompt-instruction"><code>{promptInstruction}</code></pre><pre className="agent-prompt-values"><code>{promptValues}</code></pre></section><p>{BOOTSTRAP_PLAN_CONFIRMATION}</p></div></li>
      </ol>
      {slow&&<Banner kind="warning">Codex 暂未开始。确认 Prompt 已完整发送，并允许执行只读环境检查。</Banner>}
      <div className="agent-waiting-footer"><div><span className={`agent-waiting-dot ${slow?'is-slow':''}`}/><p><strong>{state.title}</strong><small>初始化会话于 {formatExpiry(session.expires_at)} 失效</small></p></div><Button className="agent-copy-button" onClick={()=>copy(prompt,'prompt')}>{copied==='prompt'?<Check size={16}/>:<Clipboard size={16}/>} {copied==='prompt'?'已复制':'复制 Prompt'}</Button></div>
    </>}

    {(progress||session.state!=='waiting_for_computer')&&<div className={`agent-bootstrap-state agent-bootstrap-state-${state.tone}`}>{icon}<div><strong>{state.title}</strong><span>{state.detail}</span></div></div>}

    {progress&&<div className="bootstrap-progress-meta"><div><span>进度</span><strong>{progress.step} / {progress.step_count}</strong></div><progress max={progress.step_count} value={progress.step}/><div><span>{progress.check_id||progress.stage}</span><code>{progress.support_code}</code></div></div>}

    {needsApproval&&<section className="bootstrap-approval">
      <header><ShieldCheck size={20}/><div><strong>核对并确认这台电脑</strong><span>Codex 中显示的代码必须与下方一致。</span></div></header>
      <code>{progress.user_code}</code>
      <div className="bootstrap-approval-actions"><Button variant="danger" disabled={approving||denying} onClick={()=>void onDeny()}><XCircle size={16}/>{denying?'拒绝中…':'拒绝'}</Button><Button disabled={approving||denying} onClick={()=>void onApprove()}><ShieldCheck size={16}/>{approving?'批准中…':'批准这台电脑'}</Button></div>
    </section>}

    {progress?.action&&!needsApproval&&<section className="bootstrap-next-action">
      <div><strong>{progress.action.title}</strong><p>{progress.action.body}</p></div>
      {progress.action.doc_url&&<a href={progress.action.doc_url} target="_blank" rel="noreferrer">打开指南<ExternalLink size={14}/></a>}
      {actionCommand&&<Command value={actionCommand} kind={progress.action.handler==='create_diagnostic_bundle'?'diagnostics':'resume'} copied={copied} onCopy={copy}/>}
    </section>}

    {session.state==='connected'&&<div className="agent-complete"><CheckCircle2 size={22}/><div><strong>本地负责创作，云端负责治理</strong><span>初始化没有上传已有文件，也没有自动开启 Daemon。</span></div></div>}
    {copyError&&<Banner kind="error" onClose={()=>setCopyError('')}>{copyError}</Banner>}

    {session.state!=='connected'&&<details className="manual-install"><summary><Terminal size={15}/>手动排查</summary><p>按顺序运行固定命令；只根据 JSON 中的检查、错误码和下一动作处理。</p><Command label="1. 环境检查" value={commands.preflight} kind="preflight" copied={copied} onCopy={copy}/><Command label="2. 生成计划" value={commands.plan} kind="plan" copied={copied} onCopy={copy}/>{progress&&<Command label="恢复初始化" value={commands.resume} kind="resume" copied={copied} onCopy={copy}/>} {commands.diagnostics&&<Command label="生成脱敏诊断" value={commands.diagnostics} kind="diagnostics" copied={copied} onCopy={copy}/>}</details>}

    <footer className="modal-actions">
      <a className="button button-ghost" href="/docs/clients/codex" target="_blank" rel="noreferrer"><BookOpen size={15}/>接入指南</a>
      {!progress&&session.state==='waiting_for_computer'&&<Button variant="ghost" disabled={canceling} onClick={()=>void onCancel()}>{canceling?'取消中…':'取消初始化'}</Button>}
      {(session.state==='waiting_for_computer'||session.state==='verifying')&&<Button variant="secondary" onClick={onClose}>后台等待</Button>}
      {session.state==='connected'&&<Button onClick={onClose}><Check size={16}/>完成</Button>}
      {(session.state==='expired'||session.state==='canceled'||session.state==='failed')&&<><Button variant="ghost" onClick={onClose}>关闭</Button><Button disabled={retrying} onClick={()=>void onRetry()}>{retrying?'创建中…':'重新初始化'}</Button></>}
    </footer>
  </Modal>;
}

function Command({label,value,kind,copied,onCopy}:{label?:string;value:string;kind:CopyKind;copied?:CopyKind;onCopy:(value:string,kind:CopyKind)=>Promise<void>}) {
  return <div className="bootstrap-command">{label&&<span>{label}</span>}<div className="command-box"><code>{value}</code><IconButton label={`复制${label||'命令'}`} onClick={()=>void onCopy(value,kind)}>{copied===kind?<Check size={17}/>:<Clipboard size={17}/>}</IconButton></div></div>;
}

function formatExpiry(value:string):string {
  return new Intl.DateTimeFormat('zh-CN',{hour:'2-digit',minute:'2-digit',second:'2-digit'}).format(new Date(value));
}
