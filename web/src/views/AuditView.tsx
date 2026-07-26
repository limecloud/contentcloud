import { Clock3, FileClock } from 'lucide-react';
import { useEffect, useState } from 'react';
import { api } from '../api';
import type { Audit, Project } from '../types';
import { Empty } from '../components/ui';
import { ProjectPage } from './OverviewView';

export function AuditView({project}:{project:Project}) {const [items,setItems]=useState<Audit[]>([]);useEffect(()=>{api<Audit[]>(`/api/bff/projects/${project.id}/audit`).then(setItems)},[project.id]);return <ProjectPage project={project} kicker="审计" title="不可变业务事件"><section className="section audit-section">{items.length===0?<Empty title="暂无项目审计事件"/>:<div className="audit-list">{items.map(item=><div className="audit-row" key={item.id}><div className="audit-icon"><FileClock size={17}/></div><div><strong>{actionName(item.action)}</strong><span>{item.subject_type} · {item.subject_id.slice(0,8)}</span></div><time><Clock3 size={14}/>{formatDate(item.created_at)}</time></div>)}</div>}</section></ProjectPage>}
const actionName=(v:string)=>({'project.created':'创建项目','connect_session.created':'创建客户端连接','device.connected':'设备已连接','knowledge.created':'新增知识候选','knowledge.reviewed':'完成知识审核','brief.created':'创建 Brief 版本','brief.reviewed':'完成 Brief 审核','run.created':'创建本地生成任务','run.reported':'客户端提交生成结果','script.reviewed':'完成剧本审核'}[v]||v);const formatDate=(v:string)=>new Intl.DateTimeFormat('zh-CN',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}).format(new Date(v));
