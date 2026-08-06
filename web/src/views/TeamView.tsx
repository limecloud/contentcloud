import { Check, Clipboard, MailPlus, Shield, Trash2, UserRoundCheck, Users } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { api, patch, post } from '../api';
import { Banner, Button, Empty, Field, IconButton, Loading, Modal, Status } from '../components/ui';
import type { Member, MembershipInvite, Session } from '../types';
import { roleLabel } from '../uiLabels';

const roles = ['tenant_admin', 'project_manager', 'strategist', 'editor', 'reviewer', 'viewer'] as const;

export function TeamView({session, onChanged}: {session: Session; onChanged: () => Promise<void>}) {
  const canViewMembers = ['tenant_admin', 'project_manager', 'reviewer'].includes(session.role);
  const isAdmin = session.role === 'tenant_admin';
  const [members, setMembers] = useState<Member[]>([]);
  const [invites, setInvites] = useState<MembershipInvite[]>([]);
  const [loading, setLoading] = useState(canViewMembers);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [inviteForm, setInviteForm] = useState({email: '', role: 'viewer'});
  const [acceptToken, setAcceptToken] = useState('');
  const [createdToken, setCreatedToken] = useState('');
  const [copied, setCopied] = useState(false);
  const [confirm, setConfirm] = useState<{kind: 'member'|'invite'; id: string; label: string}>();

  const load = useCallback(async () => {
    if (!canViewMembers) return;
    setLoading(true);
    setError('');
    try {
      const [nextMembers, nextInvites] = await Promise.all([
        api<Member[]>('/api/bff/team/members'),
        isAdmin ? api<MembershipInvite[]>('/api/bff/team/invites') : Promise.resolve([])
      ]);
      setMembers(nextMembers);
      setInvites(nextInvites);
    } catch (e) {
      setError(message(e, '团队信息加载失败'));
    } finally {
      setLoading(false);
    }
  }, [canViewMembers, isAdmin, session.tenant.id]);

  useEffect(() => { load(); }, [load]);

  const invite = async () => {
    setBusy('invite'); setError(''); setNotice(''); setCreatedToken('');
    try {
      const created = await post<MembershipInvite>('/api/bff/team/invites', inviteForm);
      setCreatedToken(created.invite_token || '');
      setInviteForm({email: '', role: 'viewer'});
      setNotice('邀请已创建。邀请令牌只显示一次，请通过安全渠道发送。');
      await load();
    } catch (e) { setError(message(e, '邀请创建失败')); }
    finally { setBusy(''); }
  };

  const accept = async () => {
    setBusy('accept'); setError(''); setNotice('');
    try {
      await post('/api/bff/team/invites/accept', {token: acceptToken.trim()});
      setAcceptToken('');
      setNotice('邀请已接受，新团队已加入租户切换列表。');
      await onChanged();
      await load();
    } catch (e) { setError(message(e, '邀请接受失败')); }
    finally { setBusy(''); }
  };

  const updateRole = async (userID: string, role: string) => {
    setBusy(`role:${userID}`); setError('');
    try {
      await patch(`/api/bff/team/members/${userID}`, {role});
      if (userID === session.user.id) await onChanged();
      else await load();
    } catch (e) { setError(message(e, '角色更新失败')); }
    finally { setBusy(''); }
  };

  const revokeConfirmed = async () => {
    if (!confirm) return;
    const current = confirm;
    setConfirm(undefined); setBusy(`${current.kind}:${current.id}`); setError('');
    try {
      const path = current.kind === 'member' ? `/api/bff/team/members/${current.id}/revoke` : `/api/bff/team/invites/${current.id}/revoke`;
      await post(path);
      if (current.kind === 'member' && current.id === session.user.id) await onChanged();
      else await load();
    } catch (e) { setError(message(e, current.kind === 'member' ? '成员撤销失败' : '邀请撤销失败')); }
    finally { setBusy(''); }
  };

  const copyToken = async () => {
    await navigator.clipboard.writeText(createdToken);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  return <div className="page team-page">
    <div className="page-heading"><div><span className="eyebrow">租户访问管理</span><h1>团队与权限</h1><p>{session.tenant.name} 使用固定角色，撤销成员权限后其会话会立即失效。</p></div></div>
    {error && <Banner kind="error" onClose={() => setError('')}>{error}</Banner>}
    {notice && <Banner kind="success" onClose={() => setNotice('')}>{notice}</Banner>}

    <section className="team-accept section">
      <div className="team-callout-icon"><UserRoundCheck size={19}/></div>
      <div><strong>接受团队邀请</strong><span>令牌必须与当前登录邮箱一致，接受后可从左侧切换团队。</span></div>
      <input aria-label="邀请令牌" value={acceptToken} onChange={event => setAcceptToken(event.target.value)} placeholder="cci_..."/>
      <Button variant="secondary" disabled={!acceptToken.trim() || busy === 'accept'} onClick={accept}>{busy === 'accept' ? '接受中…' : '接受邀请'}</Button>
    </section>

    {!canViewMembers ? <section className="section"><Empty title="当前角色不显示成员目录" detail="邀请接受入口仍可使用；成员与权限目录仅对管理员、项目经理和审核角色开放。"/></section> : loading ? <section className="team-loading section"><Loading/></section> : <>
      <section className="section team-members">
        <header className="section-header"><div><span className="section-kicker">团队成员</span><h2>成员</h2></div><span className="section-count">{members.filter(member => member.membership.status === 'active').length} 位有效成员</span></header>
        {members.length === 0 ? <Empty title="暂无团队成员"/> : <div className="team-table">
          <div className="team-table-head"><span>成员</span><span>角色</span><span>状态</span><span>加入时间</span><span>操作</span></div>
          {members.map(member => <article className="team-member-row" key={member.membership.user_id}>
            <div className="team-person"><div>{initials(member.display_name || member.email)}</div><span><strong>{member.display_name || '未命名成员'}{member.membership.user_id === session.user.id && <small>你</small>}</strong><small>{member.email}</small></span></div>
            <div>{isAdmin && member.membership.status === 'active' ? <select aria-label={`设置 ${member.display_name} 的角色`} value={member.membership.role} disabled={busy === `role:${member.membership.user_id}`} onChange={event => updateRole(member.membership.user_id, event.target.value)}>{roles.map(role => <option key={role} value={role}>{roleLabel(role)}</option>)}</select> : <span className="role-label"><Shield size={13}/>{roleLabel(member.membership.role)}</span>}</div>
            <Status value={member.membership.status}/>
            <time>{formatDate(member.membership.created_at)}</time>
            <div>{isAdmin && member.membership.status === 'active' && <IconButton label={`撤销 ${member.display_name} 的成员资格`} disabled={busy === `member:${member.membership.user_id}`} onClick={() => setConfirm({kind: 'member', id: member.membership.user_id, label: member.display_name || member.email})}><Trash2 size={16}/></IconButton>}</div>
          </article>)}
        </div>}
      </section>

      {isAdmin && <div className="team-admin-grid">
        <section className="section invite-create">
          <header className="section-header"><div><span className="section-kicker">成员邀请</span><h2>邀请成员</h2></div><MailPlus size={18}/></header>
          <div className="team-form">
            <Field label="邮箱"><input type="email" value={inviteForm.email} onChange={event => setInviteForm({...inviteForm, email: event.target.value})} placeholder="name@company.com"/></Field>
            <Field label="固定角色"><select value={inviteForm.role} onChange={event => setInviteForm({...inviteForm, role: event.target.value})}>{roles.map(role => <option key={role} value={role}>{roleLabel(role)}</option>)}</select></Field>
            <Button disabled={!inviteForm.email.includes('@') || busy === 'invite'} onClick={invite}>{busy === 'invite' ? '创建中…' : '创建邀请'}</Button>
          </div>
          {createdToken && <div className="invite-token"><span>一次性邀请令牌</span><div><code>{createdToken}</code><IconButton label="复制邀请令牌" onClick={copyToken}>{copied ? <Check size={17}/> : <Clipboard size={17}/>}</IconButton></div></div>}
        </section>
        <section className="section invite-list">
          <header className="section-header"><div><span className="section-kicker">待接受</span><h2>待处理邀请</h2></div><span className="section-count">{invites.filter(item => item.status === 'pending').length} 个待接受</span></header>
          {invites.length === 0 ? <Empty title="暂无邀请"/> : <div>{invites.map(item => <article key={item.id}><div><strong>{item.email}</strong><span>{roleLabel(item.role)} · {formatExpiry(item.expires_at)}</span></div><Status value={item.status}/>{item.status === 'pending' && <IconButton label={`撤销 ${item.email} 的邀请`} disabled={busy === `invite:${item.id}`} onClick={() => setConfirm({kind: 'invite', id: item.id, label: item.email})}><Trash2 size={15}/></IconButton>}</article>)}</div>}
        </section>
      </div>}
    </>}

    {confirm && <Modal title={confirm.kind === 'member' ? '撤销成员资格' : '撤销邀请'} onClose={() => setConfirm(undefined)}><div className="confirm-copy"><Users size={20}/><p>{confirm.kind === 'member' ? `撤销 ${confirm.label} 后，该成员在当前团队中的活跃会话会立即失效。` : `撤销发送给 ${confirm.label} 的邀请后，原令牌将无法再使用。`}</p></div><div className="modal-actions"><Button variant="ghost" onClick={() => setConfirm(undefined)}>取消</Button><Button variant="danger" onClick={revokeConfirmed}>确认撤销</Button></div></Modal>}
  </div>;
}

function message(error: unknown, fallback: string) { return error instanceof Error ? error.message : fallback; }
function initials(value: string) { return value.trim().slice(0, 2).toUpperCase(); }
function formatDate(value: string) { return new Intl.DateTimeFormat('zh-CN', {year: 'numeric', month: '2-digit', day: '2-digit'}).format(new Date(value)); }
function formatExpiry(value: string) { return `有效至 ${new Intl.DateTimeFormat('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'}).format(new Date(value))}`; }
