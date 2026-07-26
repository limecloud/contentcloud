import { useMemo, useState } from 'react';
import { PauseCircle, PlayCircle, Search } from 'lucide-react';
import type { PlatformTenant } from '../../types';
import { Button, Modal } from '../../components/ui';
import { useAdmin } from '../context';
import { TenantTable } from '../components';

export function AdminTenantsPage() {
  const {session,data,setTenantStatus}=useAdmin();
  const [query,setQuery]=useState('');const [pending,setPending]=useState<PlatformTenant>();const [busy,setBusy]=useState('');
  const normalized=query.trim().toLowerCase();
  const tenants=useMemo(()=>data?.tenants.filter(item=>!normalized||`${item.name} ${item.slug}`.toLowerCase().includes(normalized))||[],[data,normalized]);
  const update=async()=>{if(!pending)return;setBusy(pending.id);try{await setTenantStatus(pending.id,pending.status==='active'?'suspended':'active');setPending(undefined)}catch{}finally{setBusy('')}};
  return <><div className="admin-heading admin-directory-heading"><div><span className="eyebrow">Tenant Directory</span><h1>租户管理</h1><p>{tenants.length} 条结果</p></div><label className="admin-search"><Search size={15}/><span className="visually-hidden">搜索租户</span><input value={query} onChange={event=>setQuery(event.target.value)} placeholder="搜索租户"/></label></div><section className="section admin-table-section"><TenantTable tenants={tenants} currentTenantID={session.tenant.id} busy={busy} onAction={setPending}/></section>{pending&&<Modal title={pending.status==='active'?'停用租户':'恢复租户'} onClose={()=>setPending(undefined)}><div className="admin-confirm"><div className={pending.status==='active'?'warning':'success'}>{pending.status==='active'?<PauseCircle size={21}/>:<PlayCircle size={21}/>}</div><div><strong>{pending.name}</strong><p>{pending.status==='active'?'该租户的现有登录会话会立即失效，成员在恢复前无法继续使用工作台。':'租户成员将可以重新登录并访问原有项目数据。'}</p></div></div><footer className="modal-actions"><Button variant="secondary" disabled={Boolean(busy)} onClick={()=>setPending(undefined)}>取消</Button><Button variant={pending.status==='active'?'danger':'primary'} disabled={Boolean(busy)} onClick={update}>{busy?'处理中…':pending.status==='active'?'确认停用':'确认恢复'}</Button></footer></Modal>}</>;
}
