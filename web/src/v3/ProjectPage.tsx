import { AlertTriangle, ArrowRight, BookOpen, CheckCircle2, Clock3, EyeOff, Laptop2, LoaderCircle, MonitorUp, RefreshCw, ShieldCheck } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { capabilityStatus, loadAgentClients, loadProjectAgentHandoff, loadReviewFeedbackAgentHandoff, normalizeDigest as normalizeHandoffDigest, type AgentClient, type AgentHandoff } from '../agentHandoff';
import { api } from '../api';
import { ContinueInAgentModal } from '../components/ContinueInCodexModal';
import { ProjectPageState } from '../components/ProjectPageState';
import { Banner, Button, Empty, IconButton, Loading, Status } from '../components/ui';
import { consolePath } from '../consoleRoutes';
import type { ProjectProjection, ProjectionAction, ProjectionSection, ProjectionSnapshot, ProjectionSubmission, SubmissionRevisionView } from '../types';
import { submissionTypeLabel } from '../uiLabels';
import { loginPath } from '../views/auth/returnPath';
import { useWorkspace } from '../workspace/context';
import { projectFocusFromSearch, projectPageContracts, type ProjectNavigationTarget, type ProjectView } from './page-contracts';
import { inaccessibleProjectIssue, projectPageIssueFromError, summarizeDisclosure, type ProjectPageIssue } from './page-state';
import './narrow-layout.css';

const emptySection:ProjectionSection={status:'empty',count:0,pending:0,blocked:0};
const stageOrder=['onboarding','methodology','knowledge','intelligence','strategy','planning','creative','review','delivery','learning','automation'] as const;
const stageLabels:Record<typeof stageOrder[number],string>={onboarding:'接入',methodology:'上下文',knowledge:'知识',intelligence:'情报',strategy:'策略',planning:'策划',creative:'创意',review:'审核',delivery:'交付',learning:'学习',automation:'自动化'};

export function V3ProjectPage({view}:{view:ProjectView}) {
  const {projectID}=useParams();
  const navigate=useNavigate();
  const location=useLocation();
  const {dashboard}=useWorkspace();
  const project=dashboard.projects.find(item=>item.id===projectID);
  const contract=projectPageContracts[view];
  const [projection,setProjection]=useState<ProjectProjection>();
  const [projectionIssue,setProjectionIssue]=useState<ProjectPageIssue>();
  const [loading,setLoading]=useState(true);
  const [refreshing,setRefreshing]=useState(false);
  const [error,setError]=useState('');
  const [focusedRevision,setFocusedRevision]=useState<SubmissionRevisionView>();
  const [focusLoading,setFocusLoading]=useState(false);
  const [focusIssue,setFocusIssue]=useState<ProjectPageIssue>();
  const [focusReload,setFocusReload]=useState(0);
  const [agentClients,setAgentClients]=useState<AgentClient[]>();
  const [agentHandoff,setAgentHandoff]=useState<AgentHandoff>();
  const [agentHandoffKind,setAgentHandoffKind]=useState<'project'|'review_feedback'>();
  const [agentHandoffBusy,setAgentHandoffBusy]=useState('');
  const [agentHandoffError,setAgentHandoffError]=useState('');
  const pageFocus=projectFocusFromSearch(view,location.search);
  const focusKind=pageFocus.focus?.kind||'';
  const focusID=pageFocus.focus?.id||'';
  const focusDigest=pageFocus.focus?.digest||'';
  const recoverIssue=(issue:ProjectPageIssue,retry:()=>void)=>issue.kind==='auth'?navigate(loginPath(location.pathname+location.search)):retry();

  const load=useCallback(async(background=false)=>{
    if(!projectID||!project)return;
    if(background)setRefreshing(true);else setLoading(true);
    setProjectionIssue(undefined);
    try{setProjection(await api<ProjectProjection>(`/api/bff/projects/${projectID}/projection`))}
    catch(value){setProjectionIssue(projectPageIssueFromError(value))}
    finally{setLoading(false);setRefreshing(false)}
  },[project?.id,projectID]);

  useEffect(()=>{setProjection(undefined);setAgentHandoff(undefined);setAgentHandoffKind(undefined);void load()},[load]);
  useEffect(()=>{
    if(agentClients)return;
    let active=true;
    loadAgentClients().then(value=>{if(active)setAgentClients(value)}).catch(()=>{});
    return()=>{active=false};
  },[agentClients]);
  useEffect(()=>{
    setFocusedRevision(undefined);setFocusIssue(undefined);setFocusLoading(false);
    if(view!=='review'||focusKind!=='submission_revision'||!focusID||!projectID||pageFocus.error)return;
    let active=true;
    setFocusLoading(true);
    api<SubmissionRevisionView>(`/api/bff/projects/${encodeURIComponent(projectID)}/submission-revisions/${encodeURIComponent(focusID)}`)
      .then(value=>{if(active)setFocusedRevision(value)})
      .catch(value=>{if(active)setFocusIssue(projectPageIssueFromError(value))})
      .finally(()=>{if(active)setFocusLoading(false)});
    return()=>{active=false};
  },[focusID,focusKind,focusReload,projectID,view,pageFocus.error?.code]);
  const navigateTarget=(target:ProjectNavigationTarget)=>{
    if(!projectID)return;
    const path=consolePath.projectNavigation(projectID,target);
    if(!path){setError('服务端返回了不受支持的页面导航目标');return}
    navigate(path);
  };
  const openAgentPicker=async(kind:'project'|'review_feedback')=>{
    setAgentHandoffKind(kind);setAgentHandoff(undefined);setAgentHandoffError('');
    if(agentClients)return;
    setAgentHandoffBusy('catalog');
    try{setAgentClients(await loadAgentClients())}
    catch(value){setAgentHandoffError(message(value,'无法读取智能体客户端目录'))}
    finally{setAgentHandoffBusy('')}
  };
  const selectAgent=async(client:AgentClient)=>{
    if(!projectID||!agentHandoffKind||capabilityStatus(client,'interactive_handoff')!=='available')return;
    setAgentHandoffBusy(client.id);setAgentHandoffError('');
    try{
      if(agentHandoffKind==='project')setAgentHandoff(await loadProjectAgentHandoff(projectID,client));
      else if(focusedRevision)setAgentHandoff(await loadReviewFeedbackAgentHandoff(projectID,focusedRevision.revision.id,normalizeHandoffDigest(focusedRevision.revision.content_hash),client));
    }catch(value){setAgentHandoffError(message(value,'无法创建智能体恢复入口'))}
    finally{setAgentHandoffBusy('')}
  };
  const openProjectHandoff=()=>openAgentPicker('project');
  const openReviewHandoff=()=>openAgentPicker('review_feedback');

  if(!project)return <div className="page v3-page"><ProjectPageState issue={inaccessibleProjectIssue()} onBack={()=>navigate(consolePath.dashboard)}/></div>;
  if(loading&&!projection)return <div className="page v3-loading"><Loading/></div>;
  if(projectionIssue&&!projection)return <div className="page v3-page"><ProjectPageState issue={projectionIssue} retryLabel={projectionIssue.kind==='auth'?'重新登录':'重试'} onRetry={()=>recoverIssue(projectionIssue,()=>void load())} onBack={()=>navigate(consolePath.dashboard)}/></div>;

  const section=contract.section?projection?.sections[contract.section]||emptySection:aggregateSections(projection);
  const submissions=(projection?.submissions||[]).filter(item=>contract.submissionTypes.includes(item.type));
  const snapshots=(projection?.snapshots||[]).filter(item=>contract.snapshotTypes.includes(item.type));
  const actualFocusDigest=focusedRevision?normalizeDigest(focusedRevision.revision.content_hash):'';
  const staleFocus=Boolean(focusDigest&&actualFocusDigest&&focusDigest!==actualFocusDigest);

  return <div className="page v3-page">
    <header className="v3-page-heading">
      <div><span className="eyebrow">{project.brand_name} · {contract.eyebrow}</span><h1>{contract.title}</h1><p>{contract.description}</p></div>
      <div className="v3-heading-actions">{project.connected_devices>0&&<Button variant="secondary" disabled={Boolean(agentHandoffBusy)} onClick={()=>void openProjectHandoff()}>{agentHandoffBusy==='catalog'?<LoaderCircle className="is-spinning" size={16}/>:<MonitorUp size={16}/>}在智能体客户端中继续</Button>}<Status value={section.status}/><IconButton label="刷新项目状态" disabled={refreshing} onClick={()=>void load(true)}><RefreshCw className={refreshing?'is-spinning':''} size={17}/></IconButton></div>
    </header>
    {projectionIssue&&projection&&<ProjectPageState compact issue={projectionIssue} retryLabel={projectionIssue.kind==='auth'?'重新登录':'重试'} onRetry={()=>recoverIssue(projectionIssue,()=>void load(true))} onBack={()=>navigate(consolePath.dashboard)}/>}
    {error&&<Banner kind="error" onClose={()=>setError('')}>{error}</Banner>}
    {pageFocus.error&&<Banner kind="error"><b>{pageFocus.error.code}</b>：{pageFocus.error.message}</Banner>}
    {focusLoading&&<Banner kind="info">正在读取目标提交版本…</Banner>}
    {focusIssue&&<ProjectPageState compact issue={focusIssue} backLabel="返回审核页" retryLabel={focusIssue.kind==='auth'?'重新登录':'重试'} onRetry={()=>recoverIssue(focusIssue,()=>setFocusReload(value=>value+1))} onBack={()=>navigate(consolePath.project(project.id,'review'))}/>}
    {focusedRevision&&<FocusedRevision value={focusedRevision} expectedDigest={focusDigest} stale={staleFocus} handoffBusy={Boolean(agentHandoffBusy)} onContinue={openReviewHandoff}/>}
    {view==='setup'&&focusKind==='environment_health'&&<Banner kind="info">当前焦点：创作环境健康状态。云端状态只反映已上报事实；本地写入检查和环境锁定状态仍以工作区诊断结果为准。</Banner>}
    {projection&&projection.schema_version!=='contentcloud.project-projection/3.0'&&<Banner kind="error">服务端返回了不受支持的项目投影版本：{projection.schema_version}</Banner>}
    {view==='setup'?<SetupView projection={projection} clients={agentClients} onOpenCustomerConnect={()=>navigate(`/studio/connect?project=${encodeURIComponent(project.id)}`)}/>:view==='overview'?<Overview projection={projection} onNavigate={next=>navigate(consolePath.project(project.id,next))} onNavigateTarget={navigateTarget}/>:<DomainView view={view} section={section} submissions={submissions} snapshots={snapshots} projection={projection} onNavigate={next=>navigate(consolePath.project(project.id,next))} onNavigateTarget={navigateTarget}/>}
    {agentHandoffKind&&<ContinueInAgentModal clients={agentClients} handoff={agentHandoff} kind={agentHandoffKind} loading={Boolean(agentHandoffBusy)} error={agentHandoffError} onSelect={selectAgent} onBack={()=>{setAgentHandoff(undefined);setAgentHandoffError('')}} onClose={()=>{setAgentHandoff(undefined);setAgentHandoffKind(undefined);setAgentHandoffError('')}}/>}
  </div>;
}

function FocusedRevision({value,expectedDigest,stale,handoffBusy,onContinue}:{value:SubmissionRevisionView;expectedDigest:string;stale:boolean;handoffBusy:boolean;onContinue:()=>Promise<void>}) {
  const revision=value.revision;
  const hasFeedback=value.comments.length>0||value.submission.status==='changes_requested';
  const disclosure=summarizeDisclosure(revision);
  return <><section className={`v3-focus-strip ${stale?'is-stale':''}`} aria-label="当前深链焦点">
    <div><span className="section-kicker">当前审核版本</span><h2>{submissionTypeLabel(value.submission.submission_type)} · 第 {revision.revision_no} 版</h2><p>{shortID(revision.id)} · {value.comments.length} 条审核评论</p></div>
    <dl><div><dt>当前摘要</dt><dd><code>{normalizeDigest(revision.content_hash)}</code></dd></div>{expectedDigest&&<div><dt>链接摘要</dt><dd><code>{expectedDigest}</code></dd></div>}</dl>
    <div className="v3-focus-state">{stale?<><AlertTriangle size={18}/><span><b>版本摘要不匹配</b><small>保留历史读取，禁止将决定静默应用到其他版本。</small></span></>:<><ShieldCheck size={18}/><span><b>不可变版本已验证</b><small>页面焦点与链接摘要一致。</small></span></>}<Status value={value.submission.status}/>{hasFeedback&&<Button variant="secondary" disabled={handoffBusy} onClick={()=>void onContinue()}>{handoffBusy?<LoaderCircle className="is-spinning" size={15}/>:<MonitorUp size={15}/>}在智能体客户端中修订</Button>}</div>
  </section><section className={`v3-disclosure-state ${disclosure.limited?'is-limited':''}`} aria-label="来源披露状态">
    {disclosure.limited?<EyeOff size={18}/>:<ShieldCheck size={18}/>}<div><strong>{disclosure.limited?'证据披露受限':'来源披露范围'}</strong><p>{disclosure.limited?'当前内容版本的证据不足以支持完整审核；页面只显示披露元数据，不展示未授权的证据正文。':disclosure.total?'页面仅按当前内容版本已声明的级别展示来源，不扩大披露范围。':'当前内容版本未声明来源披露。'}</p></div>
    <dl><div><dt>仅元数据</dt><dd>{disclosure.metadataOnly}</dd></div><div><dt>证据包</dt><dd>{disclosure.evidencePack}</dd></div><div><dt>完整来源</dt><dd>{disclosure.fullSource}</dd></div>{disclosure.unknown>0&&<div><dt>未识别</dt><dd>{disclosure.unknown}</dd></div>}</dl>
  </section></>;
}

function SetupView({projection,clients,onOpenCustomerConnect}:{projection?:ProjectProjection;clients?:AgentClient[];onOpenCustomerConnect:()=>void}) {
  const connectedCount=projection?.sections.onboarding?.count||0;
  const connected=connectedCount>0;
  const customerClients=(clients||[]).filter(client=>client.id==='codex'||client.id==='claude-code');
  return <div className="v3-connection-layout">
    <section className="v3-panel v3-connection-status"><header><div><span className="section-kicker">项目连接状态</span><h2>执行客户端</h2></div><Status value={connected?'connected':'waiting_for_computer'}/></header><div className="v3-panel-body v3-setup-state">{connected?<ShieldCheck size={28}/>:<Laptop2 size={28}/>}<div><strong>{connected?`${connectedCount} 个执行客户端已连接`:'尚未连接执行客户端'}</strong><p>{connected?'本地客户端可以接收分配给它的创作步骤；平台 Worker、外部服务和人工节点仍按流水线配置独立执行。':'客户需要先在创作台连接至少一个执行客户端，之后才能开始新的创作任务。'}</p></div></div><footer><a className="button button-ghost" href="/docs/clients/codex" target="_blank" rel="noreferrer"><BookOpen size={15}/>连接文档</a><Button onClick={onOpenCustomerConnect}><MonitorUp size={16}/>{connected?'管理客户连接':'打开客户连接引导'}</Button></footer></section>
    <section className="v3-panel v3-connection-capabilities"><header><div><span className="section-kicker">能力覆盖</span><h2>可连接客户端</h2></div></header><div>{customerClients.map(client=>{const bootstrap=capabilityStatus(client,'workspace_bootstrap')==='available';const automation=capabilityStatus(client,'local_automation')==='available';return <article key={client.id}><span><MonitorUp size={18}/></span><div><strong>{client.display_name}</strong><small>{bootstrap?'客户可自助连接':'连接适配器待发布'} · {automation?'支持本地执行':'本地执行待接入'}</small></div><Status value={bootstrap?'active':'planned'}/></article>})}</div><footer><p>运营后台负责查看连接、能力绑定和异常；连接确认由客户在独立创作台完成。</p></footer></section>
  </div>;
}

function Overview({projection,onNavigate,onNavigateTarget}:{projection?:ProjectProjection;onNavigate:(view:ProjectView)=>void;onNavigateTarget:(target:ProjectNavigationTarget)=>void}) {
  const sections=projection?.sections||{};
  return <div className="v3-overview-layout">
    <div className="v3-layout-next"><NextActions actions={projection?.next_actions||[]} onNavigate={onNavigateTarget}/></div>
    <section className="v3-metrics v3-layout-metrics">
      <Metric label="已连接设备" value={sections.onboarding?.count||0} detail="本地工作区绑定" tone="source"/>
      <Metric label="待审核版本" value={projection?.governance.pending_reviews||0} detail="需要人工决定" tone="warning"/>
      <Metric label="被阻断批次" value={projection?.governance.blocked_content_batches||0} detail="不能进入交付" tone="danger"/>
      <Metric label="正式快照" value={projection?.snapshots.length||0} detail="已批准快照" tone="success"/>
    </section>
    <section className="v3-panel v3-flow v3-layout-flow"><header><div><span className="section-kicker">项目流程</span><h2>业务流程</h2></div><span className="v3-generated">更新于 {formatDate(projection?.generated_at)}</span></header><div className="v3-flow-grid">{stageOrder.map(stage=><button key={stage} onClick={()=>onNavigate(viewForSection(stage))}><span className={`v3-state-dot is-${sections[stage]?.status||'empty'}`}/><strong>{stageLabels[stage]}</strong><small>{stageSummary(sections[stage])}</small></button>)}</div></section>
    <div className="v3-layout-revisions"><RecentSubmissions values={projection?.submissions.slice(0,6)||[]} onReview={()=>onNavigate('review')}/></div>
  </div>;
}

function DomainView({view,section,submissions,snapshots,projection,onNavigate,onNavigateTarget}:{view:ProjectView;section:ProjectionSection;submissions:ProjectionSubmission[];snapshots:ProjectionSnapshot[];projection?:ProjectProjection;onNavigate:(view:ProjectView)=>void;onNavigateTarget:(target:ProjectNavigationTarget)=>void}) {
  const deliveryReady=(projection?.snapshots||[]).some(item=>(item.type==='content_batch'||item.type==='asset_batch')&&item.eligible_count>0);
  const creativeBlocked=view==='creative'&&Boolean(projection?.governance.blocked_content_batches);
  const deliveryBlocked=view==='delivery'&&!deliveryReady;
  return <div className={`v3-domain-layout ${creativeBlocked||deliveryBlocked?'has-alerts':''}`}>
    <div className="v3-layout-next"><NextActions actions={projection?.next_actions||[]} onNavigate={onNavigateTarget}/></div>
    <section className="v3-metrics v3-layout-metrics">
      <Metric label="提交版本" value={section.count} detail="已提交内容" tone="source"/>
      <Metric label="待审核" value={section.pending} detail="内容版本审核" tone="warning"/>
      <Metric label="开放阻断" value={section.blocked} detail="需要补料或修订" tone="danger"/>
      <Metric label="正式快照" value={snapshots.length} detail="已批准快照" tone="success"/>
    </section>
    {(creativeBlocked||deliveryBlocked)&&<div className="v3-layout-alerts">{creativeBlocked&&<Banner kind="warning">当前有 {projection?.governance.blocked_content_batches} 个内容批次被阻断，可以进行方向评审，但不能创建交付包。</Banner>}{deliveryBlocked&&<Banner kind="warning">尚无具备正式资格的内容批次或素材批次快照，交付检查未通过。</Banner>}</div>}
    <div className="v3-layout-revisions"><RecentSubmissions values={submissions} onReview={()=>onNavigate('review')}/></div>
    <div className="v3-layout-snapshots"><SnapshotList values={snapshots}/></div>
  </div>;
}

function RecentSubmissions({values,onReview}:{values:ProjectionSubmission[];onReview:()=>void}) {
  return <section className="v3-panel"><header><div><span className="section-kicker">待审版本</span><h2>提交版本</h2></div>{values.length>0&&<Button variant="ghost" onClick={onReview}>进入审核<ArrowRight size={15}/></Button>}</header>{values.length===0?<Empty title="暂无已提交版本" detail="本地候选内容只有明确发布后才会出现在这里。"/>:<div className="v3-table-wrap"><table className="v3-table"><thead><tr><th>类型</th><th>内容版本</th><th>状态</th><th>更新时间</th></tr></thead><tbody>{values.map(item=><tr key={item.id}><td><strong>{submissionTypeLabel(item.type)}</strong><small>{shortID(item.id)}</small></td><td><code>{shortID(item.current_revision_id)}</code></td><td><Status value={item.status}/></td><td>{formatDate(item.updated_at)}</td></tr>)}</tbody></table></div>}</section>;
}

function SnapshotList({values}:{values:ProjectionSnapshot[]}) {
  return <section className="v3-panel"><header><div><span className="section-kicker">正式事实</span><h2>批准快照</h2></div><ShieldCheck size={18}/></header>{values.length===0?<Empty title="暂无正式快照" detail="只有绑定明确内容版本的人工批准，才能形成正式事实。"/>:<div className="v3-snapshot-list">{values.map(item=><article key={item.id}><CheckCircle2 size={18}/><div><strong>{submissionTypeLabel(item.type)}</strong><small>{shortID(item.id)} · {item.eligible_count} 个可用对象</small></div><time>{formatDate(item.created_at)}</time></article>)}</div>}</section>;
}

function NextActions({actions,onNavigate}:{actions:ProjectionAction[];onNavigate:(target:ProjectNavigationTarget)=>void}) {
  const action=actions[0];
  if(!action)return <section className="v3-panel v3-next-action"><Clock3 size={22}/><div><span className="section-kicker">下一步</span><h2>等待新的正式状态</h2><p>当前没有服务端生成的下一步操作。</p></div></section>;
  const label=action.id==='initialize-workspace'?'连接执行客户端':action.label;
  return <section className="v3-panel v3-next-action"><ArrowRight size={22}/><div><span className="section-kicker">下一步</span><h2>{label}</h2><p>{action.id==='initialize-workspace'?'客户连接执行客户端后，项目即可接收需要本地环境的创作步骤。':action.reason||actionDescription(action)}</p>{action.enabled&&<Button onClick={()=>onNavigate(action.navigation)}>{label}<ArrowRight size={15}/></Button>}</div></section>;
}

function Metric({label,value,detail,tone}:{label:string;value:number;detail:string;tone:'source'|'warning'|'danger'|'success'}) {return <article className={`v3-metric is-${tone}`}><span>{label}</span><strong>{value}</strong><small>{detail}</small></article>}

function aggregateSections(projection?:ProjectProjection):ProjectionSection {
  if(!projection)return emptySection;
  const values=Object.values(projection.sections);
  const count=values.reduce((sum,item)=>sum+item.count,0);const pending=values.reduce((sum,item)=>sum+item.pending,0);const blocked=values.reduce((sum,item)=>sum+item.blocked,0);
  return {status:blocked?'blocked':pending?'pending':count?'ready':'empty',count,pending,blocked};
}
function viewForSection(section:typeof stageOrder[number]):ProjectView {return ({onboarding:'setup',methodology:'context',knowledge:'knowledge',intelligence:'intelligence',strategy:'strategy',planning:'planning',creative:'creative',review:'review',delivery:'delivery',learning:'learning',automation:'automation'} as const)[section]}
function stageSummary(section?:ProjectionSection){if(!section||section.count===0)return'尚无正式记录';if(section.blocked)return`${section.blocked} 项阻断`;if(section.pending)return`${section.pending} 项待审核`;return`${section.count} 项就绪`}
function actionDescription(action:ProjectionAction){return action.kind==='review'?'处理待审内容版本，并将决定绑定到当前内容摘要。':action.kind==='assignment'?'创建受控任务，让本地工作区基于冻结输入继续工作。':'完成当前业务检查后进入下一阶段。'}
function shortID(value:string){return value.length>12?`${value.slice(0,8)}…`:value}
function formatDate(value?:string){if(!value)return'—';return new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(new Date(value))}
function normalizeDigest(value:string){const normalized=value.trim().toLowerCase();return normalized.startsWith('sha256:')?normalized:`sha256:${normalized}`}
function message(value:unknown,fallback:string){return value instanceof Error?value.message:fallback}
