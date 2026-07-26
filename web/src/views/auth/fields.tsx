import { useId, useState, type InputHTMLAttributes, type ReactNode } from 'react';
import { Eye, EyeOff } from 'lucide-react';

type BaseProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'className'> & {
  label: string;
  icon: ReactNode;
  error?: string;
  hint?: string;
  /** 校验通过的视觉反馈，用于邀请令牌等需要即时确认的字段 */
  valid?: boolean;
  /** 输入框右侧的静态指示图标 */
  suffix?: ReactNode;
};

function Wrapper({label, id, error, hint, children}: {label: string; id: string; error?: string; hint?: string; children: ReactNode}) {
  return (
    <div className="auth-field">
      <label htmlFor={id}>{label}</label>
      <div className="auth-input-wrap">{children}</div>
      {error ? <span className="auth-field-error">{error}</span> : hint ? <span className="auth-field-hint">{hint}</span> : null}
    </div>
  );
}

function inputClass(error?: string, valid?: boolean, hasSuffix?: boolean): string {
  return ['auth-input', hasSuffix ? 'has-suffix' : '', error ? 'auth-input-error' : '', !error && valid ? 'auth-input-ok' : ''].filter(Boolean).join(' ');
}

export function IconInput({label, icon, error, hint, valid, suffix, id: providedID, ...props}: BaseProps) {
  const generatedID = useId();
  const id = providedID || generatedID;
  return (
    <Wrapper label={label} id={id} error={error} hint={hint}>
      <span className="auth-input-icon">{icon}</span>
      <input id={id} className={inputClass(error, valid, Boolean(suffix))} aria-invalid={error ? true : undefined} {...props} />
      {suffix && <span className="auth-suffix-static">{suffix}</span>}
    </Wrapper>
  );
}

export function PasswordInput({label, icon, error, hint, id: providedID, ...props}: Omit<BaseProps, 'valid' | 'suffix' | 'type'>) {
  const generatedID = useId();
  const id = providedID || generatedID;
  const [visible, setVisible] = useState(false);
  return (
    <Wrapper label={label} id={id} error={error} hint={hint}>
      <span className="auth-input-icon">{icon}</span>
      <input id={id} type={visible ? 'text' : 'password'} className={inputClass(error, false, true)} aria-invalid={error ? true : undefined} {...props} />
      <button type="button" className="auth-suffix" onClick={() => setVisible(!visible)} disabled={props.disabled} aria-label={visible ? '隐藏密码' : '显示密码'} title={visible ? '隐藏密码' : '显示密码'}>
        {visible ? <EyeOff size={17} /> : <Eye size={17} />}
      </button>
    </Wrapper>
  );
}

export function Submit({busy, busyLabel, label, icon, disabled}: {busy: boolean; busyLabel: string; label: string; icon: ReactNode; disabled?: boolean}) {
  return (
    <button type="submit" className="auth-submit" disabled={busy || disabled}>
      {busy ? <span className="auth-spinner" aria-hidden="true" /> : icon}
      {busy ? busyLabel : label}
    </button>
  );
}
