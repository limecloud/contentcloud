import { useState } from 'react';
import { LogIn, Lock, Mail } from 'lucide-react';
import { post } from '../../api';
import { Banner } from '../../components/ui';
import { AuthLayout } from './AuthLayout';
import { IconInput, PasswordInput, Submit } from './fields';
import { hasErrors, validateLogin, type AuthErrors } from './validate';

export function LoginView({onSuccess, onNavigate, notice, registerPath='/register'}: {onSuccess: () => Promise<void>; onNavigate: (path: string) => void; notice?: string; registerPath?:string}) {
  const [form, setForm] = useState({email: '', password: ''});
  const [errors, setErrors] = useState<AuthErrors>({});
  const [failure, setFailure] = useState('');
  const [busy, setBusy] = useState(false);
  const update = (patch: Partial<typeof form>) => {
    setForm(previous => ({...previous, ...patch}));
    setErrors(previous => {
      const next = {...previous};
      for (const key of Object.keys(patch)) delete next[key as keyof AuthErrors];
      return next;
    });
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const found = validateLogin(form);
    setErrors(found);
    if (hasErrors(found)) return;
    setBusy(true);
    setFailure('');
    try {
      await post('/api/v1/auth/login', {email: form.email.trim(), password: form.password});
      await onSuccess();
    } catch (error) {
      setFailure(error instanceof Error ? error.message : '登录失败');
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthLayout footer={<>还没有团队？ <button type="button" className="auth-link" onClick={() => onNavigate(registerPath)}>创建团队</button></>}>
      <h2>欢迎回来</h2>
      <p>登录以继续你的工作台</p>
      {notice && <Banner kind="warning">{notice}</Banner>}
      {failure && <Banner kind="error" onClose={() => setFailure('')}>{failure}</Banner>}
      <form className="auth-form" onSubmit={submit} noValidate>
        <IconInput label="邮箱" icon={<Mail size={17} />} type="email" autoComplete="email" autoFocus placeholder="name@company.com" value={form.email} disabled={busy} error={errors.email} onChange={event => update({email: event.target.value})} />
        <PasswordInput label="密码" icon={<Lock size={17} />} autoComplete="current-password" placeholder="请输入密码" value={form.password} disabled={busy} error={errors.password} onChange={event => update({password: event.target.value})} />
        <Submit busy={busy} busyLabel="登录中…" label="登录" icon={<LogIn size={17} />} />
      </form>
    </AuthLayout>
  );
}
