import { AlertTriangle, ArrowDown, ArrowLeft, ArrowRight, GitBranch, Network, RotateCcw, Search } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../api';
import { Banner, Empty, IconButton, Loading, Status } from '../components/ui';
import type { ImpactAnalysis, LineageEdge, LineageGraph, LineageNode, Project } from '../types';
import { ProjectPage } from './OverviewView';

type Direction = 'both'|'upstream'|'downstream';

const stages = [
  {id:'sources',label:'来源'},
  {id:'knowledge',label:'知识与权利'},
  {id:'strategy',label:'内容策略'},
  {id:'brief',label:'Brief'},
  {id:'generation',label:'本地任务'},
  {id:'script',label:'剧本'},
  {id:'delivery',label:'交付'},
  {id:'results',label:'效果'},
  {id:'learning',label:'学习决策'},
];

export function LineageView({project}:{project:Project}) {
  const [graph,setGraph]=useState<LineageGraph>();
  const [impact,setImpact]=useState<ImpactAnalysis>();
  const [focusKey,setFocusKey]=useState('');
  const [direction,setDirection]=useState<Direction>('both');
  const [search,setSearch]=useState('');
  const [loading,setLoading]=useState(true);
  const [error,setError]=useState('');

  useEffect(()=>{setFocusKey('');setDirection('both')},[project.id]);
  useEffect(()=>{
    let active=true;
    setLoading(true);setError('');
    const focus=focusKey?splitKey(focusKey):undefined;
    const params=new URLSearchParams();
    if(focus){params.set('focus_type',focus.type);params.set('focus_id',focus.id)}
    params.set('direction',direction);
    const suffix=`?${params.toString()}`;
    Promise.all([
      api<LineageGraph>(`/api/bff/projects/${project.id}/lineage${suffix}`),
      api<ImpactAnalysis>(`/api/bff/projects/${project.id}/impact${focus?`?focus_type=${encodeURIComponent(focus.type)}&focus_id=${encodeURIComponent(focus.id)}`:''}`),
    ]).then(([nextGraph,nextImpact])=>{if(active){setGraph(nextGraph);setImpact(nextImpact)}}).catch(reason=>{if(active)setError(reason instanceof Error?reason.message:'追踪数据加载失败')}).finally(()=>{if(active)setLoading(false)});
    return()=>{active=false};
  },[project.id,focusKey,direction]);

  const nodeByKey=useMemo(()=>new Map((graph?.nodes||[]).map(node=>[node.key,node])),[graph]);
  const selected=focusKey?nodeByKey.get(focusKey)||impact?.focus:undefined;
  const normalized=search.trim().toLowerCase();
  const visibleNodes=useMemo(()=>(graph?.nodes||[]).filter(node=>!normalized||`${node.label} ${node.type} ${node.id} ${node.status}`.toLowerCase().includes(normalized)),[graph,normalized]);
  const selectedEdges=useMemo(()=>(graph?.edges||[]).filter(edge=>!focusKey||edge.from===focusKey||edge.to===focusKey),[graph,focusKey]);
  const reset=()=>{setFocusKey('');setDirection('both');setSearch('')};

  return <ProjectPage project={project} kicker="追踪与影响" title="端到端内容链路" actions={focusKey?<IconButton label="清除聚焦" onClick={reset}><RotateCcw size={17}/></IconButton>:undefined}>
    {error&&<Banner kind="error">{error}</Banner>}
    <section className="lineage-toolbar" aria-label="追踪筛选">
      <label className="lineage-search"><Search size={15}/><input value={search} onChange={event=>setSearch(event.target.value)} placeholder="搜索对象或 ID"/></label>
      <div className="lineage-direction" role="group" aria-label="追踪方向">
        <button className={direction==='upstream'?'active':''} onClick={()=>setDirection('upstream')}><ArrowLeft size={14}/>上游</button>
        <button className={direction==='both'?'active':''} onClick={()=>setDirection('both')}><GitBranch size={14}/>双向</button>
        <button className={direction==='downstream'?'active':''} onClick={()=>setDirection('downstream')}>下游<ArrowRight size={14}/></button>
      </div>
    </section>
    {loading&&!graph?<section className="section lineage-loading"><Loading/></section>:!graph||graph.nodes.length===0?<section className="section"><Empty title="暂无可追踪对象"/></section>:<>
      <section className="lineage-summary" aria-label="链路摘要">
        <div><Network size={18}/><span>对象</span><strong>{graph.nodes.length}</strong></div>
        <div><GitBranch size={18}/><span>关系</span><strong>{graph.edges.length}</strong></div>
        <div><ArrowDown size={18}/><span>阶段</span><strong>{Object.keys(graph.stage_count).length}</strong></div>
        <div><AlertTriangle size={18}/><span>待处理</span><strong>{impact?.items.filter(item=>item.severity!=='review').length||0}</strong></div>
      </section>
      <section className={`lineage-board ${loading?'is-loading':''}`} aria-busy={loading}>
        {stages.map(stage=>{
          const nodes=visibleNodes.filter(node=>node.stage===stage.id);
          return <div className="lineage-stage" key={stage.id}><header><span>{stage.label}</span><strong>{nodes.length}</strong></header><div>{nodes.map(node=><LineageNodeButton key={node.key} node={node} active={focusKey===node.key} onClick={()=>setFocusKey(node.key)}/>)}</div></div>
        })}
      </section>
      <div className="lineage-detail-grid">
        <section className="section lineage-relations"><header className="section-header"><div><span className="section-kicker">Relations</span><h2>{selected?selected.label:'项目关系'}</h2></div>{selected&&<Status value={selected.status}/>}</header>{selectedEdges.length===0?<Empty title="暂无直接关系"/>:<div>{selectedEdges.map((edge,index)=><RelationRow key={`${edge.from}-${edge.to}-${edge.relation}-${index}`} edge={edge} nodes={nodeByKey} focusKey={focusKey}/>)}</div>}</section>
        <section className="section lineage-impact"><header className="section-header"><div><span className="section-kicker">Impact</span><h2>影响与动作</h2></div><span className="section-count">{impact?.items.length||0} 项</span></header>{!impact||impact.items.length===0?<Empty title="当前没有待传播影响"/>:<div className="impact-table">{impact.items.map(item=><article key={item.node.key}><span className={`impact-severity impact-${item.severity}`}/><div><header><strong>{item.node.label}</strong><Status value={item.current_status}/></header><p>{item.reason}</p><footer>{item.recommended_action}</footer></div></article>)}</div>}</section>
      </div>
    </>}
  </ProjectPage>
}

function LineageNodeButton({node,active,onClick}:{node:LineageNode;active:boolean;onClick:()=>void}) {
  return <button className={`lineage-node ${active?'active':''}`} onClick={onClick} title={`${typeName(node.type)} · ${node.id}`}><span>{typeName(node.type)}</span><strong>{node.label}</strong><footer><code>{node.id.slice(0,8)}</code><Status value={node.status}/></footer></button>
}

function RelationRow({edge,nodes,focusKey}:{edge:LineageEdge;nodes:Map<string,LineageNode>;focusKey:string}) {
  const from=nodes.get(edge.from),to=nodes.get(edge.to);
  return <article className="relation-row"><div className={edge.to===focusKey?'relation-focus':''}><span>{from?.label||edge.from}</span><small>{typeName(from?.type||'')}</small></div><div><ArrowRight size={14}/><code>{relationName(edge.relation)}</code></div><div className={edge.from===focusKey?'relation-focus':''}><span>{to?.label||edge.to}</span><small>{typeName(to?.type||'')}</small></div><p>{edge.reason}</p></article>
}

function splitKey(key:string) {const separator=key.indexOf(':');return {type:key.slice(0,separator),id:key.slice(separator+1)}}
const typeName=(value:string)=>({source:'来源',source_revision:'来源修订',asset:'素材',rights_record:'权利记录',knowledge_item:'知识',benchmark:'对标',content_framework:'框架',shot_pattern:'镜头模式',selling_point:'卖点',visualization_plan:'可视化方案',brief_version:'Brief',task_run:'本地任务',script_version:'剧本版本',artifact:'交付工件',performance_import_batch:'导入批次',performance_observation:'效果观察',rating_decision:'评级决策'}[value]||value);
const relationName=(value:string)=>({has_revision:'修订',supports:'支撑',frozen_into:'冻结',starts:'启动',produces:'产出',exported_as:'导出',measured_by:'度量',evidence_for:'证据',rated_by:'评级',supersedes:'替代',used_by:'使用',referenced_by:'引用',selected_by:'选择'}[value]||value);
