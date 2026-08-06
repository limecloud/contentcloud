import { LoaderCircle, PauseCircle, PlayCircle, ShieldCheck, Video } from 'lucide-react';
import type { PlatformTenant, PlatformUser } from '../types';
import { Empty, IconButton, Status } from '../components/ui';
import { roleLabel } from '../uiLabels';

export function TenantTable({tenants,currentTenantID,busy,onAction,onContentAction,compact=false}:{tenants:PlatformTenant[];currentTenantID:string;busy:string;onAction?:(tenant:PlatformTenant)=>void;onContentAction?:(tenant:PlatformTenant)=>void;compact?:boolean}) {
  return <div className="admin-table-scroll"><div className={`admin-tenant-table ${compact?'is-compact':''}`}>
    <header><span>租户</span><span>状态</span><span>内容能力</span><span>成员</span><span>项目</span><span>在线设备</span><span>活跃任务</span><span>最近活动</span>{!compact&&<span>操作</span>}</header>
    {tenants.length===0?<Empty title="没有匹配的租户"/>:tenants.map(tenant=><TenantRow key={tenant.id} tenant={tenant} currentTenantID={currentTenantID} busy={busy} onAction={onAction} onContentAction={onContentAction} compact={compact}/>) }
  </div></div>;
}

function TenantRow({tenant,currentTenantID,busy,onAction,onContentAction,compact}:{tenant:PlatformTenant;currentTenantID:string;busy:string;onAction?:(tenant:PlatformTenant)=>void;onContentAction?:(tenant:PlatformTenant)=>void;compact:boolean}) {
  const wechat=tenant.content_types.includes('wechat_article');
  const capabilityBusy=busy===`${tenant.id}:wechat_article`;
  return <article>
    <div className="admin-tenant-name"><span>{tenant.name.slice(0,1)}</span><div><strong>{tenant.name}</strong><small>{tenant.slug}</small></div></div>
    <Status value={tenant.status}/>
    <div className="admin-content-capabilities"><span title="视频剧本为默认能力"><Video size={13}/>视频</span><label className="admin-content-toggle"><input aria-label={`${tenant.name}微信公众号文章`} type="checkbox" checked={wechat} disabled={!onContentAction||capabilityBusy} onChange={()=>onContentAction?.(tenant)}/>{capabilityBusy?<><LoaderCircle className="is-spinning" size={13}/>更新中</>:<span>公众号</span>}</label></div>
    <strong>{tenant.member_count}</strong><strong>{tenant.project_count}</strong><strong>{tenant.device_count}</strong><strong>{tenant.active_run_count}</strong><time>{tenant.last_activity_at?formatRelative(tenant.last_activity_at):'暂无活动'}</time>
    {!compact&&<div className="admin-row-action">{tenant.id===currentTenantID?<span>当前租户</span>:tenant.status==='active'?<IconButton label={`停用 ${tenant.name}`} disabled={busy===tenant.id} onClick={()=>onAction?.(tenant)}><PauseCircle size={17}/></IconButton>:<IconButton label={`恢复 ${tenant.name}`} disabled={busy===tenant.id} onClick={()=>onAction?.(tenant)}><PlayCircle size={17}/></IconButton>}</div>}
  </article>;
}

export function UserAvatar({user}:{user:PlatformUser}) {return <span className="admin-user-avatar">{(user.display_name||user.email).slice(0,1).toUpperCase()}</span>}
export function AdminBadge({long=false}:{long?:boolean}) {return <span className="admin-badge"><ShieldCheck size={12}/>{long?'平台管理员':'管理员'}</span>}
export { roleLabel };
export const formatDate=(value:string)=>new Intl.DateTimeFormat('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit'}).format(new Date(value));
function formatRelative(value:string){const seconds=Math.round((Date.now()-new Date(value).getTime())/1000);if(seconds<60)return '刚刚';if(seconds<3600)return `${Math.floor(seconds/60)} 分钟前`;if(seconds<86400)return `${Math.floor(seconds/3600)} 小时前`;return formatDate(value)}
