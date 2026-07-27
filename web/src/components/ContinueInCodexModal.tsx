import { Check, Clipboard, ExternalLink, MonitorUp, ShieldCheck } from 'lucide-react';
import { useState } from 'react';
import type { CodexHandoff } from '../codexHandoff';
import './ContinueInCodexModal.css';
import { Banner, Button, Modal } from './ui';

export function ContinueInCodexModal({ handoff, onClose }: { handoff: CodexHandoff; onClose: () => void }) {
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState('');

  const copyPrompt = async () => {
    setCopyError('');
    try {
      await navigator.clipboard.writeText(handoff.prompt);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopyError('无法访问剪贴板，请检查浏览器权限后重试。');
    }
  };

  const openCodex = () => {
    window.location.assign(handoff.launch_url);
  };

  return <Modal title={handoff.kind === 'review_feedback' ? '在 Codex 中修订' : '在 Codex 中继续'} onClose={onClose}>
    <div className="codex-handoff-assurance"><ShieldCheck size={20}/><div><strong>安全的新对话恢复</strong><span>Codex 只会预填 Prompt，不会自动发送，也不会自动选择本机 Workspace。</span></div></div>
    <dl className="codex-handoff-facts">
      <div><dt>项目</dt><dd><code>{handoff.project_id}</code></dd></div>
      <div><dt>目标</dt><dd><code>{handoff.target.kind} / {handoff.target.id}</code></dd></div>
      {handoff.target.digest&&<div><dt>Digest</dt><dd><code>{handoff.target.digest}</code></dd></div>}
      <div><dt>Plugin</dt><dd><code>{handoff.plugin_id} · {handoff.plugin_version}</code></dd></div>
    </dl>
    <ol className="codex-handoff-steps">{handoff.steps.map((step,index)=><li key={step}><span>{index+1}</span><p>{step}</p></li>)}</ol>
    <section className="codex-handoff-prompt"><header><strong>恢复 Prompt</strong><Button variant="ghost" onClick={()=>void copyPrompt()}>{copied?<Check size={15}/>:<Clipboard size={15}/>} {copied?'已复制':'复制 Prompt'}</Button></header><pre><code>{handoff.prompt}</code></pre></section>
    {copyError&&<Banner kind="error" onClose={()=>setCopyError('')}>{copyError}</Banner>}
    <footer className="modal-actions codex-handoff-actions">
      <a className="button button-ghost" href={handoff.fallback_url} target="_blank" rel="noreferrer">接入指南<ExternalLink size={15}/></a>
      <Button variant="secondary" onClick={onClose}>取消</Button>
      <Button onClick={openCodex}><MonitorUp size={16}/>打开 Codex</Button>
    </footer>
  </Modal>;
}
