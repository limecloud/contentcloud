import type { PropsWithChildren, ReactNode } from 'react';

export function AuthLayout({children, footer}: PropsWithChildren<{footer?: ReactNode}>) {
  return (
    <main className="auth-shell">
      <div className="auth-bg" />
      <div className="auth-grid" aria-hidden="true" />
      <div className="auth-content">
        <header className="auth-head">
          <div className="auth-logo">CC</div>
          <h1 className="auth-title">ContentCloud</h1>
          <p className="auth-subtitle">内容生产协作平台</p>
        </header>
        <section className="auth-card">{children}</section>
        {footer && <div className="auth-footer">{footer}</div>}
        <p className="auth-copyright">© {new Date().getFullYear()} ContentCloud. All rights reserved.</p>
      </div>
    </main>
  );
}
