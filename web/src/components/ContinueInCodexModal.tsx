import { ArrowLeft, BookOpen, Check, ChevronRight, Clipboard, Clock3, ExternalLink, MonitorUp, ShieldCheck } from 'lucide-react';
import { useState } from 'react';
import { capabilityStatus, type AgentClient, type AgentHandoff } from '../agentHandoff';
import './ContinueInCodexModal.css';
import { Banner, Button, Modal } from './ui';

interface ContinueInAgentModalProps {
  clients?: AgentClient[];
  handoff?: AgentHandoff;
  kind: 'project' | 'review_feedback';
  loading: boolean;
  error?: string;
  onSelect: (client: AgentClient) => Promise<void>;
  onBack: () => void;
  onClose: () => void;
}

export function ContinueInAgentModal({clients,handoff,kind,loading,error,onSelect,onBack,onClose}:ContinueInAgentModalProps) {
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState('');

  const copyPrompt = async () => {
    setCopyError('');
    try {
      if (!handoff) return;
      await navigator.clipboard.writeText(handoff.prompt);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopyError('无法访问剪贴板，请检查浏览器权限后重试。');
    }
  };

  const openAgent = () => {
    if (handoff) window.location.assign(handoff.launch.url);
  };

  if(!handoff)return <Modal title={kind==='review_feedback'?'选择修订客户端':'选择 Agent 客户端'} onClose={onClose}>
    <div className="agent-client-list" aria-busy={loading}>
      {(clients||[]).map(client=>{
        const available=capabilityStatus(client,'interactive_handoff')==='available';
        return <div className="agent-client-row" key={client.id}><button type="button" className="agent-client-option" disabled={!available||loading} onClick={()=>void onSelect(client)}>
          <span className={`agent-client-mark ${available?'is-available':''}`}>{available?<MonitorUp size={18}/>:<Clock3 size={18}/>}</span>
          <span><strong>{client.display_name}</strong><small>{available?'可用':'即将支持'}</small></span>
          {available&&<ChevronRight size={18}/>}
        </button><a className="icon-button agent-client-doc" href={`/docs/clients/${client.id}`} target="_blank" rel="noreferrer" title={`查看 ${client.display_name} 使用文档`} aria-label={`查看 ${client.display_name} 使用文档`}><BookOpen size={16}/></a></div>;
      })}
      {loading&&<p className="agent-client-loading">正在读取客户端目录…</p>}
    </div>
    {error&&<Banner kind="error">{error}</Banner>}
    <footer className="modal-actions"><Button variant="secondary" onClick={onClose}>取消</Button></footer>
  </Modal>;

  return <Modal title={handoff.kind === 'review_feedback' ? `在 ${handoff.client.display_name} 中修订` : `在 ${handoff.client.display_name} 中继续`} onClose={onClose}>
    <div className="agent-handoff-assurance"><ShieldCheck size={20}/><div><strong>安全的新对话恢复</strong><span>{handoff.client.display_name} 只会预填 Prompt，不会自动发送，也不会自动选择本机 Workspace。</span></div></div>
    <dl className="agent-handoff-facts">
      <div><dt>项目</dt><dd><code>{handoff.project_id}</code></dd></div>
      <div><dt>目标</dt><dd><code>{handoff.target.kind} / {handoff.target.id}</code></dd></div>
      {handoff.target.digest&&<div><dt>Digest</dt><dd><code>{handoff.target.digest}</code></dd></div>}
      <div><dt>{handoff.integration.kind}</dt><dd><code>{handoff.integration.id} · {handoff.integration.version}</code></dd></div>
    </dl>
    <ol className="agent-handoff-steps">{handoff.steps.map((step,index)=><li key={step}><span>{index+1}</span><p>{step}</p></li>)}</ol>
    <section className="agent-handoff-prompt"><header><strong>恢复 Prompt</strong><Button variant="ghost" onClick={()=>void copyPrompt()}>{copied?<Check size={15}/>:<Clipboard size={15}/>} {copied?'已复制':'复制 Prompt'}</Button></header><pre><code>{handoff.prompt}</code></pre></section>
    {copyError&&<Banner kind="error" onClose={()=>setCopyError('')}>{copyError}</Banner>}
    <footer className="modal-actions agent-handoff-actions">
      <Button variant="ghost" onClick={onBack}><ArrowLeft size={15}/>更换客户端</Button>
      <a className="button button-ghost" href={handoff.fallback_url} target="_blank" rel="noreferrer">接入指南<ExternalLink size={15}/></a>
      <Button variant="secondary" onClick={onClose}>取消</Button>
      <Button onClick={openAgent}><MonitorUp size={16}/>打开 {handoff.client.display_name}</Button>
    </footer>
  </Modal>;
}
