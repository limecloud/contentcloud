import { useMemo, useState } from 'react';
import { Search } from 'lucide-react';
import { Empty } from '../../components/ui';
import { useAdmin } from '../context';
import { AdminBadge, formatDate, roleLabel, UserAvatar } from '../components';

export function AdminUsersPage() {
  const {data}=useAdmin();const [query,setQuery]=useState('');const normalized=query.trim().toLowerCase();
  const users=useMemo(()=>data?.users.filter(item=>!normalized||`${item.display_name} ${item.email} ${item.memberships.map(value=>value.tenant_name).join(' ')}`.toLowerCase().includes(normalized))||[],[data,normalized]);
  return <><div className="admin-heading admin-directory-heading"><div><span className="eyebrow">User Directory</span><h1>用户目录</h1><p>{users.length} 条结果</p></div><label className="admin-search"><Search size={15}/><span className="visually-hidden">搜索用户</span><input value={query} onChange={event=>setQuery(event.target.value)} placeholder="搜索用户或租户"/></label></div><section className="section admin-table-section"><div className="admin-table-scroll"><div className="admin-user-table"><header><span>用户</span><span>平台权限</span><span>所属租户</span><span>账号状态</span><span>注册时间</span></header>{users.length===0?<Empty title="没有匹配的用户"/>:users.map(user=><article key={user.id}><div className="admin-user-name"><UserAvatar user={user}/><div><strong>{user.display_name}</strong><small>{user.email}</small></div></div>{user.is_platform_admin?<AdminBadge long/>:<span className="admin-muted">普通用户</span>}<div className="admin-memberships">{user.memberships.length?user.memberships.map(item=><span key={`${item.tenant_id}:${item.role}`}>{item.tenant_name}<small>{roleLabel(item.role)}</small></span>):<span>无有效租户</span>}</div><span className={user.verified_at?'admin-verified':'admin-muted'}>{user.verified_at?'已验证':'未验证'}</span><time>{formatDate(user.created_at)}</time></article>)}</div></div></section></>;
}
