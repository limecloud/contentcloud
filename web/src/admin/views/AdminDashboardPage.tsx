import { Building2, FolderKanban, Laptop2, Users } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { useAdmin } from '../context';
import { adminPath } from '../routes';
import { AdminBadge, TenantTable, UserAvatar } from '../components';

export function AdminDashboardPage() {
  const {data}=useAdmin();const navigate=useNavigate();
  if(!data)return null;
  const stats=[{label:'活跃租户',value:data.counts.active_tenants,detail:`共 ${data.counts.tenants} 个`,icon:Building2,tone:'green'},{label:'注册用户',value:data.counts.users,detail:'全平台账号',icon:Users,tone:'cyan'},{label:'项目总数',value:data.counts.projects,detail:'包含已归档',icon:FolderKanban,tone:'ink'},{label:'在线设备',value:data.counts.online_devices,detail:`${data.counts.active_runs} 个活跃任务`,icon:Laptop2,tone:'amber'}];
  return <><div className="admin-heading"><div><span className="eyebrow">Platform Operations</span><h1>系统概览</h1><p>租户、用户与运行资源</p></div><div className="admin-health"><span></span><div><strong>服务正常</strong><small>管理 API 可用</small></div></div></div><section className="admin-stat-grid">{stats.map(({label,value,detail,icon:Icon,tone})=><article key={label}><div className={`stat-icon tone-${tone}`}><Icon size={19}/></div><div><strong>{value}</strong><span>{label}</span><small>{detail}</small></div></article>)}</section><div className="admin-overview-grid"><section className="section admin-table-section"><header className="section-header"><div><span className="section-kicker">租户</span><h2>最近加入</h2></div><button className="admin-text-button" onClick={()=>navigate(adminPath('tenants'))}>查看全部</button></header><TenantTable tenants={data.tenants.slice(0,6)} currentTenantID="" busy="" compact/></section><section className="section admin-activity"><header className="section-header"><div><span className="section-kicker">账号</span><h2>最近注册</h2></div><button className="admin-text-button" onClick={()=>navigate(adminPath('users'))}>查看全部</button></header>{data.users.slice(0,6).map(user=><article key={user.id}><UserAvatar user={user}/><div><strong>{user.display_name}</strong><span>{user.email}</span></div>{user.is_platform_admin?<AdminBadge/>:<small>{user.memberships.length} 个租户</small>}</article>)}</section></div></>;
}
