export const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
/** 与后端 newRegistration 的 len(password) < 10 保持一致 */
export const MIN_PASSWORD_LENGTH = 10;

export interface AuthErrors {
  email?: string;
  password?: string;
  display_name?: string;
  tenant_name?: string;
  invite_token?: string;
}

export function validateEmail(value: string): string | undefined {
  const email = value.trim();
  if (!email) return '请填写邮箱';
  if (!EMAIL_PATTERN.test(email)) return '邮箱格式不正确';
  return undefined;
}

export function validateLoginPassword(value: string): string | undefined {
  if (!value) return '请填写密码';
  return undefined;
}

export function validateNewPassword(value: string): string | undefined {
  if (!value) return '请填写密码';
  if (value.length < MIN_PASSWORD_LENGTH) return `密码至少 ${MIN_PASSWORD_LENGTH} 位`;
  return undefined;
}

export function validateLogin(form: {email: string; password: string}): AuthErrors {
  return prune({email: validateEmail(form.email), password: validateLoginPassword(form.password)});
}

export function validateRegister(form: {email: string; password: string; tenant_name: string; invite_token: string}, mode: 'create' | 'invite'): AuthErrors {
  return prune({
    email: validateEmail(form.email),
    password: validateNewPassword(form.password),
    tenant_name: mode === 'create' && !form.tenant_name.trim() ? '请填写团队名称' : undefined,
    invite_token: mode === 'invite' && !form.invite_token.trim() ? '请填写邀请令牌' : undefined
  });
}

export function hasErrors(errors: AuthErrors): boolean {
  return Object.keys(errors).length > 0;
}

function prune(errors: AuthErrors): AuthErrors {
  return Object.fromEntries(Object.entries(errors).filter(([, value]) => value !== undefined));
}
