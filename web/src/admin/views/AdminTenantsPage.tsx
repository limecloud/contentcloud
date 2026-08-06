import { useMemo, useState } from 'react';
import { FileText, PauseCircle, PlayCircle, Search, Video } from 'lucide-react';
import type { PlatformTenant } from '../../types';
import { Button, Modal } from '../../components/ui';
import { useAdmin } from '../context';
import { TenantTable } from '../components';

export function AdminTenantsPage() {
  const {session,data,setTenantStatus,setTenantContentCapability}=useAdmin();
  const [query,setQuery]=useState('');const [pending,setPending]=useState<PlatformTenant>();const [busy,setBusy]=useState('');
  const normalized=query.trim().toLowerCase();
  const tenants=useMemo(()=>data?.tenants.filter(item=>!normalized||`${item.name} ${item.slug}`.toLowerCase().includes(normalized))||[],[data,normalized]);
  const update=async()=>{if(!pending)return;setBusy(pending.id);try{await setTenantStatus(pending.id,pending.status==='active'?'suspended':'active');setPending(undefined)}catch{}finally{setBusy('')}};
  const toggleContent=async(tenant:PlatformTenant)=>{const enabled=!tenant.content_types.includes('wechat_article');setBusy(`${tenant.id}:wechat_article`);try{await setTenantContentCapability(tenant.id,'wechat_article',enabled)}catch{}finally{setBusy('')}};
  return <><div className="admin-heading admin-directory-heading"><div><span className="eyebrow">租户目录</span><h1>租户管理</h1><p>{tenants.length} 条结果</p></div><label className="admin-search"><Search size={15}/><span className="visually-hidden">搜索租户</span><input value={query} onChange={event=>setQuery(event.target.value)} placeholder="搜索租户"/></label></div><section className="section admin-capability-section"><header className="section-header"><div><span className="section-kicker">内容能力</span><h2>内容类型</h2></div></header><div className="admin-capability-legend"><span><Video size={15}/>视频剧本<small>所有租户默认启用</small></span><span><FileText size={15}/>微信公众号文章<small>按租户显式开通</small></span></div></section><section className="section admin-table-section"><TenantTable tenants={tenants} currentTenantID={session.tenant.id} busy={busy} onAction={setPending} onContentAction={tenant=>void toggleContent(tenant)}/></section>{pending&&<Modal title={pending.status==='active'?'停用租户':'恢复租户'} onClose={()=>setPending(undefined)}><div className="admin-confirm"><div className={pending.status==='active'?'warning':'success'}>{pending.status==='active'?<PauseCircle size={21}/>:<PlayCircle size={21}/>}</div><div><strong>{pending.name}</strong><p>{pending.status==='active'?'该租户的现有登录会话会立即失效，成员在恢复前无法继续使用工作台。':'租户成员将可以重新登录并访问原有项目数据。'}</p></div></div><footer className="modal-actions"><Button variant="secondary" disabled={Boolean(busy)} onClick={()=>setPending(undefined)}>取消</Button><Button variant={pending.status==='active'?'danger':'primary'} disabled={Boolean(busy)} onClick={update}>{busy?'处理中…':pending.status==='active'?'确认停用':'确认恢复'}</Button></footer></Modal>}</>;
}
