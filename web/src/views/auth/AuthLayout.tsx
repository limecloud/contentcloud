import type { PropsWithChildren, ReactNode } from 'react';
import { BrandMark } from '../../components/Brand';

export function AuthLayout({children, footer}: PropsWithChildren<{footer?: ReactNode}>) {
  return (
    <main className="auth-shell">
      <section className="auth-scene"><div className="auth-scene-copy"><span>CONTENT WORK OS</span><h1>让内容工作，<br/>自然流转</h1><p>本地 Agent 专注创作，团队在云端接住审核、批准与交付。</p></div></section>
      <section className="auth-panel"><div className="auth-content">
        <header className="auth-head"><BrandMark className="auth-logo"/><h1 className="auth-title">Content Work OS</h1><p className="auth-subtitle">内容工作操作系统</p></header>
        <section className="auth-card">{children}</section>
        {footer && <div className="auth-footer">{footer}</div>}
        <p className="auth-copyright">© {new Date().getFullYear()} Content Work OS</p>
      </div></section>
    </main>
  );
}
