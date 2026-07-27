import { ArrowLeft, CloudOff, LogIn, RefreshCw, ShieldX } from 'lucide-react';
import type { ProjectPageIssue } from '../v3/page-state';
import { Button } from './ui';

export function ProjectPageState({issue,compact=false,backLabel='返回项目列表',retryLabel='重试',onBack,onRetry}:{issue:ProjectPageIssue;compact?:boolean;backLabel?:string;retryLabel?:string;onBack:()=>void;onRetry?:()=>void}) {
  const Icon=issue.kind==='auth'?LogIn:issue.kind==='access'?ShieldX:CloudOff;
  const RetryIcon=issue.kind==='auth'?LogIn:RefreshCw;
  return <section className={`project-page-state ${compact?'is-compact':''}`} role="alert" aria-live="polite">
    <div className="project-page-state-icon"><Icon size={compact?22:28}/></div>
    <div className="project-page-state-copy"><span>{issue.code}</span><h2>{issue.title}</h2><p>{issue.detail}</p></div>
    <div className="project-page-state-actions">
      {onRetry&&<Button onClick={onRetry}><RetryIcon size={16}/>{retryLabel}</Button>}
      <Button variant="secondary" onClick={onBack}><ArrowLeft size={16}/>{backLabel}</Button>
    </div>
  </section>;
}
