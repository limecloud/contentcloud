import { describe, expect, it } from 'vitest';
import { MIN_PASSWORD_LENGTH, hasErrors, validateLogin, validateRegister } from './validate';

describe('validateLogin', () => {
  it('接受合法凭据', () => {
    expect(hasErrors(validateLogin({email: 'a@b.com', password: 'x'}))).toBe(false);
  });

  it('拒绝空邮箱与空密码', () => {
    expect(validateLogin({email: '', password: ''})).toEqual({email: '请填写邮箱', password: '请填写密码'});
  });

  it('拒绝缺少域名后缀的邮箱', () => {
    expect(validateLogin({email: 'a@b', password: 'x'}).email).toBe('邮箱格式不正确');
  });

  it('登录不校验密码长度', () => {
    expect(validateLogin({email: 'a@b.com', password: 'short'}).password).toBeUndefined();
  });
});

describe('validateRegister', () => {
  const base = {email: 'a@b.com', password: 'long-enough-password', tenant_name: '团队', invite_token: ''};

  it('创建模式要求团队名称', () => {
    expect(validateRegister({...base, tenant_name: '  '}, 'create').tenant_name).toBe('请填写团队名称');
  });

  it('创建模式不要求邀请令牌', () => {
    expect(hasErrors(validateRegister(base, 'create'))).toBe(false);
  });

  it('邀请模式要求令牌但不要求团队名称', () => {
    const errors = validateRegister({...base, tenant_name: '', invite_token: ''}, 'invite');
    expect(errors.invite_token).toBe('请填写邀请令牌');
    expect(errors.tenant_name).toBeUndefined();
  });

  it('邀请模式填了令牌即通过', () => {
    expect(hasErrors(validateRegister({...base, tenant_name: '', invite_token: 'cci_abc'}, 'invite'))).toBe(false);
  });

  it('密码长度门槛与后端一致', () => {
    expect(validateRegister({...base, password: 'a'.repeat(MIN_PASSWORD_LENGTH - 1)}, 'create').password).toBe(`密码至少 ${MIN_PASSWORD_LENGTH} 位`);
    expect(validateRegister({...base, password: 'a'.repeat(MIN_PASSWORD_LENGTH)}, 'create').password).toBeUndefined();
  });
});
