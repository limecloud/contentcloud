import { useEffect, useMemo, useState } from 'react';
import { Activity, AlertTriangle, Ban, Bot, CheckCircle2, ChevronRight, Clock3, Play, RefreshCw, ServerCog, ShieldAlert, Workflow } from 'lucide-react';
import { api, post } from '../../api';
import type { RuntimeJobDetail, RuntimeJobList, RuntimeJobSummary } from '../../types';

const stateLabel:Record<string,string>={created:'已创建',admitted:'已接纳',running:'运行中',waiting_human:'等待人工',paused:'已暂停',completed:'已完成',failed:'失败',cancelled:'已取消',rejected:'已拒绝'};
const stateClass=(state:string)=>state==='completed'?'is-success':state==='failed'||state==='cancelled'?'is-danger':state==='waiting_human'||state==='paused'?'is-warning':state==='running'?'is-working':'is-neutral';
const agentStateLabel:Record<string,string>={created:'已创建',runnable:'可运行',active:'执行中',waiting_children:'等待子节点',waiting_gate:'等待审批',waiting_effect:'等待外部操作',completed:'已完成',failed:'失败',canceling:'取消中',cancelled:'已取消'};
const agentStateClass=(state:string)=>state==='completed'?'is-success':state==='failed'||state==='cancelled'?'is-danger':state.startsWith('waiting')||state==='canceling'?'is-warning':state==='active'?'is-working':'is-neutral';
const dateTime=(value:string)=>new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit'}).format(new Date(value));

export function AdminRuntimePage() {
  const [list,setList]=useState<RuntimeJobList>();
  const [selectedID,setSelectedID]=useState('');
  const [detail,setDetail]=useState<RuntimeJobDetail>();
  const [state,setState]=useState('');
  const [loading,setLoading]=useState(true);
  const [busy,setBusy]=useState('');
  const [error,setError]=useState('');

  const loadList=async()=>{
    setLoading(true);setError('');
    try{
      const query=state?`?state=${encodeURIComponent(state)}`:'';
      const next=await api<RuntimeJobList>(`/api/bff/runtime/jobs${query}`);setList(next);
      const nextID=selectedID&&next.items.some(item=>item.id===selectedID)?selectedID:next.items[0]?.id||'';
      setSelectedID(nextID);
      if(nextID) await loadDetail(nextID);
      else setDetail(undefined);
    }catch(value){setError(value instanceof Error?value.message:'Runtime 运行数据读取失败')}
    finally{setLoading(false)}
  };
  const loadDetail=async(jobID:string)=>{try{setDetail(await api<RuntimeJobDetail>(`/api/bff/runtime/jobs/${encodeURIComponent(jobID)}`))}catch(value){setError(value instanceof Error?value.message:'Runtime 详情读取失败')}};
  useEffect(()=>{void loadList()},[state]);
  const selected=useMemo(()=>list?.items.find(item=>item.id===selectedID),[list,selectedID]);
  const action=async(kind:'refresh'|'cancel'|'resume')=>{
    if(!selectedID)return;setBusy(kind);setError('');
    try{const next=await post<RuntimeJobDetail>(`/api/bff/runtime/jobs/${encodeURIComponent(selectedID)}/${kind}`);setDetail(next);await loadList()}catch(value){setError(value instanceof Error?value.message:'Runtime 操作失败')}finally{setBusy('')}
  };

  return <div className="admin-runtime-page">
    <div className="admin-heading"><div><span className="eyebrow">运行保障 / 任务记录</span><h1>任务记录</h1><p>查看任务执行状态、人工等待点和外部副作用。</p></div><div className="admin-runtime-actions"><label className="admin-runtime-filter"><span>状态</span><select value={state} onChange={event=>setState(event.target.value)}><option value="">全部</option><option value="running">运行中</option><option value="waiting_human">等待人工</option><option value="failed">失败</option><option value="completed">已完成</option></select></label><button className="icon-button" aria-label="刷新任务记录" title="刷新任务记录" disabled={loading} onClick={()=>void loadList()}><RefreshCw size={16} className={loading?'is-spinning':''}/></button></div></div>
    {error&&<div className="admin-runtime-error" role="alert"><AlertTriangle size={16}/><span>{error}</span></div>}
    <div className="admin-runtime-layout">
      <section className="section admin-runtime-list"><header className="section-header"><div><span className="section-kicker">JobRun</span><h2>执行实例</h2></div><span className="admin-muted">{list?.items.length||0} 条</span></header>{loading?<div className="admin-runtime-empty">正在读取运行实例…</div>:!list?.items.length?<div className="admin-runtime-empty"><ServerCog size={22}/><span>当前没有匹配的执行实例</span></div>:<div className="admin-runtime-job-list">{list.items.map(item=><RuntimeJobRow key={item.id} item={item} selected={item.id===selectedID} onClick={()=>{setSelectedID(item.id);void loadDetail(item.id)}}/>)}</div>}</section>
      <section className="section admin-runtime-detail">{!detail||!selected?<div className="admin-runtime-empty"><Workflow size={24}/><span>选择一个执行实例查看详情</span></div>:<RuntimeDetail detail={detail} busy={busy} onAction={action}/>}</section>
    </div>
  </div>;
}

function RuntimeJobRow({item,selected,onClick}:{item:RuntimeJobSummary;selected:boolean;onClick:()=>void}) {
  const nodeSummary=Object.entries(item.node_states).map(([key,value])=>`${value} ${key}`).join(' · ');
  return <button className={`admin-runtime-job-row ${selected?'is-selected':''}`} onClick={onClick}><span className="admin-runtime-job-icon"><Activity size={16}/></span><span className="admin-runtime-job-copy"><strong>{item.task_title||'未命名任务'}</strong><small>{item.project_name||'未命名项目'} · {item.id.slice(0,8)}</small><small>{nodeSummary||'尚未产生节点状态'}</small></span><span className={`runtime-state ${stateClass(item.state)}`}>{stateLabel[item.state]||item.state}</span><ChevronRight size={15}/></button>;
}

function RuntimeDetail({detail,busy,onAction}:{detail:RuntimeJobDetail;busy:string;onAction:(kind:'refresh'|'cancel'|'resume')=>Promise<void>}) {
  const {summary}=detail;
  return <div className="admin-runtime-detail-inner"><header className="admin-runtime-detail-header"><div><span className="eyebrow">{summary.project_name}</span><h2>{summary.task_title||'未命名任务'}</h2><p>{summary.id} · 更新于 {dateTime(summary.updated_at)}</p></div><div className="admin-runtime-detail-actions">{summary.state==='paused'&&<button className="button button-primary" disabled={Boolean(busy)} onClick={()=>void onAction('resume')}><Play size={14}/>{busy==='resume'?'恢复中…':'恢复运行'}</button>}<button className="button button-secondary" disabled={Boolean(busy)||['completed','failed','cancelled'].includes(summary.state)} onClick={()=>void onAction('refresh')}><RefreshCw size={14}/>{busy==='refresh'?'刷新中…':'刷新'}</button><button className="button button-danger" disabled={Boolean(busy)||['completed','failed','cancelled'].includes(summary.state)} onClick={()=>void onAction('cancel')}><Ban size={14}/>{busy==='cancel'?'取消中…':'取消运行'}</button></div></header>
    <div className="admin-runtime-facts"><span><small>状态</small><strong className={`runtime-state ${stateClass(summary.state)}`}>{stateLabel[summary.state]||summary.state}</strong></span><span><small>节点</small><strong>{summary.node_count}</strong></span><span><small>外部副作用</small><strong>{summary.effect_count}</strong></span><span><small>检查点</small><strong>{summary.checkpoint_count}</strong></span></div>
    <RuntimeNodes nodes={detail.nodes}/>
    <RuntimeAgents agents={detail.agents}/>
    <div className="admin-runtime-subgrid"><RuntimeEvents events={detail.events}/><RuntimeEffects effects={detail.effects}/></div>
    {detail.checkpoints.length>0&&<section className="admin-runtime-subsection"><header><strong>检查点</strong><span>{detail.checkpoints.length} 个</span></header>{detail.checkpoints.map(item=><div className="admin-runtime-checkpoint" key={item.id}><CheckCircle2 size={15}/><div><strong>{item.node_key}</strong><small>{item.completed_nodes.length} 个节点已完成 · {item.digest.slice(0,18)}…</small></div><time>{dateTime(item.created_at)}</time></div>)}</section>}
  </div>;
}

function RuntimeNodes({nodes}:{nodes:RuntimeJobDetail['nodes']}) {return <section className="admin-runtime-subsection"><header><strong>节点概览</strong><span>{nodes.length} 个节点</span></header><div className="admin-runtime-node-table"><div className="admin-runtime-node-head"><span>节点</span><span>状态</span><span>尝试</span><span>更新时间</span></div>{nodes.map(node=><div className="admin-runtime-node-row" key={node.id}><div><strong>{node.name||node.node_key}</strong><small>{node.node_key} · {node.kind}</small></div><span className={`runtime-state ${stateClass(node.state)}`}>{stateLabel[node.state]||node.state}</span><span>{node.attempt_count}</span><time>{dateTime(node.updated_at)}</time></div>)}</div></section>}
function RuntimeAgents({agents}:{agents:RuntimeJobDetail['agents']}) {return <section className="admin-runtime-subsection"><header><strong><Bot size={14}/>智能体树</strong><span>{agents.length} 个实例</span></header>{agents.length===0?<div className="admin-runtime-empty compact"><Bot size={18}/><span>当前执行尚未创建智能体实例</span></div>:<div className="admin-runtime-node-table"><div className="admin-runtime-node-head"><span>角色</span><span>状态</span><span>预算</span><span>ContextView</span></div>{agents.map(agent=><div className="admin-runtime-node-row" key={agent.id}><div style={{paddingLeft:`${Math.min(agent.depth,5)*12}px`}}><strong>{agent.role||'未命名角色'}</strong><small>{agent.harness_kind} · 深度 {agent.depth}{agent.parent_agent_instance_id?' · 子实例':''}</small></div><span className={`runtime-state ${agentStateClass(agent.state)}`}>{agentStateLabel[agent.state]||agent.state}</span><span>{agent.used_cost_minor} / {agent.budget_minor}</span><div><strong>{agent.context_view.allowed_tools.join(' · ')||'无工具'}</strong><small>{agent.context_view.input_ref_count} 个输入引用 · {agent.session_bound?'已绑定会话':'未绑定会话'}</small></div></div>)}</div>}</section>}
function RuntimeEvents({events}:{events:RuntimeJobDetail['events']}) {return <section className="admin-runtime-subsection"><header><strong>事件时间线</strong><span>{events.length} 条</span></header><div className="admin-runtime-event-list">{events.slice(-12).reverse().map(event=><article key={event.id}><Clock3 size={14}/><div><strong>{event.type}</strong><small>{event.node_key||'JobRun'} · {event.actor_type}</small></div><time>{dateTime(event.occurred_at)}</time></article>)}</div></section>}
function RuntimeEffects({effects}:{effects:RuntimeJobDetail['effects']}) {return <section className="admin-runtime-subsection"><header><strong>外部副作用</strong><span>{effects.length} 条</span></header>{effects.length===0?<div className="admin-runtime-empty compact"><ShieldAlert size={18}/><span>尚无外部副作用记录</span></div>:<div className="admin-runtime-event-list">{effects.map(effect=><article key={effect.id}><ShieldAlert size={14}/><div><strong>{effect.kind}</strong><small>{effect.state} · {effect.error_code||'已脱敏'}</small></div><time>{effect.cost_minor?`${effect.cost_minor} ${effect.currency}`:'-'}</time></article>)}</div>}</section>}
