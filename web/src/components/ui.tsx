import type { ButtonHTMLAttributes, PropsWithChildren, ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, X } from 'lucide-react';
import { statusLabel } from '../uiLabels';

export function Button({variant = 'primary', className = '', ...props}: ButtonHTMLAttributes<HTMLButtonElement> & {variant?: 'primary'|'secondary'|'ghost'|'danger'}) {
  return <button className={`button button-${variant} ${className}`} {...props} />;
}

export function IconButton({label, children, ...props}: ButtonHTMLAttributes<HTMLButtonElement> & {label:string;children:ReactNode}) {
  return <button className="icon-button" aria-label={label} title={label} {...props}>{children}</button>;
}

export function Status({value}: {value:string}) {
  return <span className={`status status-${value}`}>{statusLabel(value)}</span>;
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
