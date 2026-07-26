import type { ButtonHTMLAttributes, PropsWithChildren, ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, X } from 'lucide-react';

export function Button({variant = 'primary', className = '', ...props}: ButtonHTMLAttributes<HTMLButtonElement> & {variant?: 'primary'|'secondary'|'ghost'|'danger'}) {
  return <button className={`button button-${variant} ${className}`} {...props} />;
}

export function IconButton({label, children, ...props}: ButtonHTMLAttributes<HTMLButtonElement> & {label:string;children:ReactNode}) {
  return <button className="icon-button" aria-label={label} title={label} {...props}>{children}</button>;
}

export function Status({value}: {value:string}) {
  const labels: Record<string,string> = {draft:'草稿',active:'进行中',archived:'已归档',pending:'待接受',revoked:'已撤销',expired:'已过期',canceled:'已取消',blocked:'已阻断',waiting_for_computer:'等待连接',connected:'已连接',candidate:'候选',needs_review:'待审核',approved:'已批准',rejected:'已拒绝',conflicted:'有冲突',review_required:'待复核',internal_review:'内审中',revision_requested:'待修订',queued:'等待设备',leased:'执行中',running:'执行中',succeeded:'已完成',failed:'失败',review_ready:'可审核',internally_approved:'内审通过',client_review:'客户审核',imported:'已导入',seed_candidate:'跑量候选',repairable:'可修复',discarded:'不采用',insufficient_sample:'样本不足'};
  return <span className={`status status-${value}`}>{labels[value] || value}</span>;
}

export function Empty({title, detail, action}: {title:string;detail?:string;action?:ReactNode}) {
  return <div className="empty"><div className="empty-mark" /><h3>{title}</h3>{detail && <p>{detail}</p>}{action}</div>;
}

export function Loading() { return <div className="loading"><span /><span /><span /></div> }

export function Banner({kind='info',children,onClose}:PropsWithChildren<{kind?:'info'|'success'|'warning'|'error';onClose?:()=>void}>) {
  return <div className={`banner banner-${kind}`}>{kind==='success'?<CheckCircle2 size={17}/>:kind==='warning'||kind==='error'?<AlertTriangle size={17}/>:null}<div>{children}</div>{onClose&&<IconButton label="关闭" onClick={onClose}><X size={16}/></IconButton>}</div>
}

export function Modal({title,children,onClose}:PropsWithChildren<{title:string;onClose:()=>void}>) {
  return <div className="modal-backdrop" role="presentation" onMouseDown={(e)=>{if(e.target===e.currentTarget)onClose()}}><section className="modal" role="dialog" aria-modal="true"><header><h2>{title}</h2><IconButton label="关闭" onClick={onClose}><X size={18}/></IconButton></header><div className="modal-body">{children}</div></section></div>
}

export function Field({label,children,hint}:PropsWithChildren<{label:string;hint?:string}>) { return <label className="field"><span>{label}</span>{children}{hint&&<small>{hint}</small>}</label> }
