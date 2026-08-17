import { useState } from 'react';
import { AlertCircle, Building2, CheckCircle2, Key, Lock, Mail, User, UserPlus } from 'lucide-react';
import { post } from '../../api';
import { Banner } from '../../components/ui';
import { AuthLayout } from './AuthLayout';
import { IconInput, PasswordInput, Submit } from './fields';
import { MIN_PASSWORD_LENGTH, hasErrors, validateRegister, type AuthErrors } from './validate';

type Mode = 'create' | 'invite';

export function RegisterView({onSuccess, onNavigate, initialInviteToken, loginPath='/login'}: {onSuccess: () => Promise<void>; onNavigate: (path: string) => void; initialInviteToken?: string; loginPath?:string}) {
  const [mode, setMode] = useState<Mode>(initialInviteToken ? 'invite' : 'create');
  const [form, setForm] = useState({email: '', password: '', display_name: '', tenant_name: '', invite_token: initialInviteToken || ''});
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
  const switchMode = (next: Mode) => {
    setMode(next);
    setErrors({});
    setFailure('');
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const found = validateRegister(form, mode);
    setErrors(found);
    if (hasErrors(found)) return;
    setBusy(true);
    setFailure('');
    try {
      const payload = mode === 'invite'
        ? {email: form.email.trim(), password: form.password, display_name: form.display_name.trim(), invite_token: form.invite_token.trim()}
        : {email: form.email.trim(), password: form.password, display_name: form.display_name.trim(), tenant_name: form.tenant_name.trim()};
      await post('/api/v1/auth/register', payload);
      await onSuccess();
    } catch (error) {
      setFailure(error instanceof Error ? error.message : '注册失败');
    } finally {
      setBusy(false);
    }
  };

  const tokenFilled = form.invite_token.trim().length > 0;
  return (
    <AuthLayout footer={<>已有账号？ <button type="button" className="auth-link" onClick={() => onNavigate(loginPath)}>登录</button></>}>
      <h2>{mode === 'create' ? '创建团队' : '加入团队'}</h2>
      <p>{mode === 'create' ? '注册账号并创建你的内容团队' : '使用管理员发来的邀请码加入现有团队'}</p>
      <div className="auth-modes" role="tablist" aria-label="注册方式">
        <button type="button" role="tab" aria-selected={mode === 'create'} className={`auth-mode ${mode === 'create' ? 'active' : ''}`} disabled={busy} onClick={() => switchMode('create')}>创建团队</button>
        <button type="button" role="tab" aria-selected={mode === 'invite'} className={`auth-mode ${mode === 'invite' ? 'active' : ''}`} disabled={busy} onClick={() => switchMode('invite')}>邀请加入</button>
      </div>
      {failure && <Banner kind="error" onClose={() => setFailure('')}>{failure}</Banner>}
      <form className="auth-form" onSubmit={submit} noValidate>
        {mode === 'invite' && (
          <IconInput label="邀请码" icon={<Key size={17} />} placeholder="cci_…" autoComplete="off" autoFocus value={form.invite_token} disabled={busy} error={errors.invite_token} valid={tokenFilled} hint="由团队管理员在“团队”页创建后发给你" suffix={errors.invite_token ? <AlertCircle size={17} /> : tokenFilled ? <CheckCircle2 size={17} /> : undefined} onChange={event => update({invite_token: event.target.value})} />
        )}
        <IconInput label="姓名" icon={<User size={17} />} placeholder="选填，默认取邮箱前缀" autoComplete="name" autoFocus={mode === 'create'} value={form.display_name} disabled={busy} error={errors.display_name} onChange={event => update({display_name: event.target.value})} />
        {mode === 'create' && (
          <IconInput label="团队名称" icon={<Building2 size={17} />} placeholder="例如：南京澄观内容科技" autoComplete="organization" value={form.tenant_name} disabled={busy} error={errors.tenant_name} onChange={event => update({tenant_name: event.target.value})} />
        )}
        <IconInput label="邮箱" icon={<Mail size={17} />} type="email" autoComplete="email" placeholder="name@company.com" value={form.email} disabled={busy} error={errors.email} hint={mode === 'invite' ? '必须与收到邀请的邮箱一致' : undefined} onChange={event => update({email: event.target.value})} />
        <PasswordInput label="密码" icon={<Lock size={17} />} autoComplete="new-password" placeholder={`至少 ${MIN_PASSWORD_LENGTH} 位`} value={form.password} disabled={busy} error={errors.password} hint={`密码至少 ${MIN_PASSWORD_LENGTH} 位`} onChange={event => update({password: event.target.value})} />
        <Submit busy={busy} busyLabel="提交中…" label={mode === 'create' ? '创建团队' : '加入团队'} icon={<UserPlus size={17} />} />
      </form>
    </AuthLayout>
  );
}
