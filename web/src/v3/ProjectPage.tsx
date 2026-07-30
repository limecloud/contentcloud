import { AlertTriangle, ArrowRight, BookOpen, CheckCircle2, Clock3, EyeOff, Laptop2, LoaderCircle, MonitorUp, RefreshCw, ShieldCheck } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { capabilityStatus, loadAgentClients, loadProjectAgentHandoff, loadReviewFeedbackAgentHandoff, normalizeDigest as normalizeHandoffDigest, type AgentClient, type AgentHandoff } from '../agentHandoff';
import { api, post } from '../api';
import { ContinueInAgentModal } from '../components/ContinueInCodexModal';
import { InitializeWorkspaceModal } from '../components/InitializeWorkspaceModal';
import { ProjectPageState } from '../components/ProjectPageState';
import { Banner, Button, Empty, IconButton, Loading, Status } from '../components/ui';
import { connectStateCopy, isActiveConnectState, type BootstrapAttempt, type BootstrapAuthorizationView, type ConnectSession } from '../connectBootstrap';
import { consolePath } from '../consoleRoutes';
import type { ProjectProjection, ProjectionAction, ProjectionSection, ProjectionSnapshot, ProjectionSubmission, SubmissionRevisionView } from '../types';
import { loginPath } from '../views/auth/returnPath';
import { useWorkspace } from '../workspace/context';
import { projectFocusFromSearch, projectPageContracts, type ProjectNavigationTarget, type ProjectView } from './page-contracts';
import { inaccessibleProjectIssue, projectPageIssueFromError, summarizeDisclosure, type ProjectPageIssue } from './page-state';
import './narrow-layout.css';

const emptySection:ProjectionSection={status:'empty',count:0,pending:0,blocked:0};
const stageOrder=['onboarding','methodology','knowledge','intelligence','strategy','planning','creative','review','delivery','learning','automation'] as const;
const stageLabels:Record<typeof stageOrder[number],string>={onboarding:'接入',methodology:'上下文',knowledge:'知识',intelligence:'情报',strategy:'策略',planning:'策划',creative:'创意',review:'审核',delivery:'交付',learning:'学习',automation:'自动化'};
const submissionLabels:Record<string,string>={context:'Context',knowledge:'Knowledge',brief:'Brief',content_batch:'ContentBatch',asset_batch:'AssetBatch',delivery:'Delivery',result:'Result'};

export function V3ProjectPage({view}:{view:ProjectView}) {
  const {projectID}=useParams();
  const navigate=useNavigate();
  const location=useLocation();
  const {session,dashboard,refresh:refreshWorkspace}=useWorkspace();
  const project=dashboard.projects.find(item=>item.id===projectID);
  const contract=projectPageContracts[view];
  const [projection,setProjection]=useState<ProjectProjection>();
  const [projectionIssue,setProjectionIssue]=useState<ProjectPageIssue>();
  const [loading,setLoading]=useState(true);
  const [refreshing,setRefreshing]=useState(false);
  const [error,setError]=useState('');
  const [connect,setConnect]=useState<ConnectSession>();
  const [connectOpen,setConnectOpen]=useState(false);
  const [connectBusy,setConnectBusy]=useState('');
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

  useEffect(()=>{setProjection(undefined);setConnect(undefined);setConnectOpen(false);setAgentHandoff(undefined);setAgentHandoffKind(undefined);void load()},[load]);
  useEffect(()=>{
    if(agentClients)return;
    let active=true;
    loadAgentClients().then(value=>{if(active)setAgentClients(value)}).catch(()=>{});
    return()=>{active=false};
  },[agentClients]);
  useEffect(()=>{
    if(view!=='setup'||!projectID)return;
    const attemptID=focusKind==='bootstrap_attempt'?focusID:'';
    if(!attemptID)return;
    api<BootstrapAuthorizationView>(`/api/bff/projects/${projectID}/bootstrap-attempts/${encodeURIComponent(attemptID)}`).then(value=>{setConnect(value.session);setConnectOpen(true)}).catch(value=>setError(message(value,'无法读取初始化授权')));
  },[focusID,focusKind,projectID,view]);
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
  useEffect(()=>{
    if(!connect||!isActiveConnectState(connect.state))return;
    const timer=window.setInterval(async()=>{
      try{
        const next=await api<ConnectSession>(`/api/bff/connect-sessions/${connect.id}`);
        setConnect(next);
        if(next.state==='connected'){void load(true);void refreshWorkspace()}
      }catch{/* 下一次轮询会重试。 */}
    },2000);
    return()=>window.clearInterval(timer);
  },[connect?.id,connect?.state,load,refreshWorkspace]);

  const createConnect=async()=>{
    if(!projectID)return;
    setConnectBusy('connect');setError('');
    try{const next=await post<ConnectSession>(`/api/bff/projects/${projectID}/connect-sessions`);setConnect(next);setConnectOpen(true)}
    catch(value){setError(message(value,'创建初始化会话失败'))}
    finally{setConnectBusy('')}
  };
  const cancelConnect=async()=>{
    if(!connect)return;
    setConnectBusy('cancel');
    try{setConnect(await post<ConnectSession>(`/api/bff/connect-sessions/${connect.id}/cancel`));setConnectOpen(false)}
    catch(value){setError(message(value,'取消初始化失败'));setConnectOpen(false)}
    finally{setConnectBusy('')}
  };
  const decideAuthorization=async(decision:'approve'|'deny')=>{
    if(!connect?.progress?.attempt_id)return;
    setConnectBusy(decision);setError('');
    try{
      await post<BootstrapAttempt>(`/api/bff/connect-sessions/${connect.id}/attempts/${connect.progress.attempt_id}/${decision}`);
      setConnect(await api<ConnectSession>(`/api/bff/connect-sessions/${connect.id}`));
    }catch(value){setError(message(value,decision==='approve'?'批准初始化失败':'拒绝初始化失败'))}
    finally{setConnectBusy('')}
  };

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
    catch(value){setAgentHandoffError(message(value,'无法读取 Agent 客户端目录'))}
    finally{setAgentHandoffBusy('')}
  };
  const selectAgent=async(client:AgentClient)=>{
    if(!projectID||!agentHandoffKind||capabilityStatus(client,'interactive_handoff')!=='available')return;
    setAgentHandoffBusy(client.id);setAgentHandoffError('');
    try{
      if(agentHandoffKind==='project')setAgentHandoff(await loadProjectAgentHandoff(projectID,client));
      else if(focusedRevision)setAgentHandoff(await loadReviewFeedbackAgentHandoff(projectID,focusedRevision.revision.id,normalizeHandoffDigest(focusedRevision.revision.content_hash),client));
    }catch(value){setAgentHandoffError(message(value,'无法创建 Agent 恢复入口'))}
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
  const canManage=session.role==='tenant_admin'||session.role==='project_manager';
  const initialize=canManage&&project.status!=='archived'?createConnect:undefined;
  const actualFocusDigest=focusedRevision?normalizeDigest(focusedRevision.revision.content_hash):'';
  const staleFocus=Boolean(focusDigest&&actualFocusDigest&&focusDigest!==actualFocusDigest);

  return <div className="page v3-page">
    <header className="v3-page-heading">
      <div><span className="eyebrow">{project.brand_name} · {contract.eyebrow}</span><h1>{contract.title}</h1><p>{contract.description}</p></div>
      <div className="v3-heading-actions">{project.connected_devices>0&&<Button variant="secondary" disabled={Boolean(agentHandoffBusy)} onClick={()=>void openProjectHandoff()}>{agentHandoffBusy==='catalog'?<LoaderCircle className="is-spinning" size={16}/>:<MonitorUp size={16}/>}在 Agent 中继续</Button>}<Status value={section.status}/><IconButton label="刷新项目状态" disabled={refreshing} onClick={()=>void load(true)}><RefreshCw className={refreshing?'is-spinning':''} size={17}/></IconButton></div>
    </header>
    {projectionIssue&&projection&&<ProjectPageState compact issue={projectionIssue} retryLabel={projectionIssue.kind==='auth'?'重新登录':'重试'} onRetry={()=>recoverIssue(projectionIssue,()=>void load(true))} onBack={()=>navigate(consolePath.dashboard)}/>}
    {error&&<Banner kind="error" onClose={()=>setError('')}>{error}</Banner>}
    {pageFocus.error&&<Banner kind="error"><b>{pageFocus.error.code}</b>：{pageFocus.error.message}</Banner>}
    {focusLoading&&<Banner kind="info">正在读取目标 SubmissionRevision…</Banner>}
    {focusIssue&&<ProjectPageState compact issue={focusIssue} backLabel="返回审核页" retryLabel={focusIssue.kind==='auth'?'重新登录':'重试'} onRetry={()=>recoverIssue(focusIssue,()=>setFocusReload(value=>value+1))} onBack={()=>navigate(consolePath.project(project.id,'review'))}/>}
    {focusedRevision&&<FocusedRevision value={focusedRevision} expectedDigest={focusDigest} stale={staleFocus} handoffBusy={Boolean(agentHandoffBusy)} onContinue={openReviewHandoff}/>}
    {view==='setup'&&focusKind==='environment_health'&&<Banner kind="info">当前焦点：创作环境健康。云端状态只反映已上报事实；本地写探针和 Environment Lock 仍以 Workspace doctor 为准。</Banner>}
    {projection&&projection.schema_version!=='contentcloud.project-projection/3.0'&&<Banner kind="error">服务端返回了不受支持的项目投影版本：{projection.schema_version}</Banner>}
    {view==='setup'?<SetupView projection={projection} connect={connect} clients={agentClients} canInitialize={Boolean(initialize)} busy={connectBusy==='connect'} onInitialize={initialize} onNavigate={navigateTarget}/>:view==='overview'?<Overview projection={projection} onNavigate={next=>navigate(consolePath.project(project.id,next))} onNavigateTarget={navigateTarget}/>:<DomainView view={view} section={section} submissions={submissions} snapshots={snapshots} projection={projection} onNavigate={next=>navigate(consolePath.project(project.id,next))} onNavigateTarget={navigateTarget}/>}
    {connect&&connectOpen&&<InitializeWorkspaceModal session={connect} projectName={`${project.brand_name} / ${project.product_name}`} serverURL={window.location.origin} canceling={connectBusy==='cancel'} retrying={connectBusy==='connect'} approving={connectBusy==='approve'} denying={connectBusy==='deny'} onClose={()=>setConnectOpen(false)} onCancel={cancelConnect} onRetry={createConnect} onApprove={()=>decideAuthorization('approve')} onDeny={()=>decideAuthorization('deny')}/>}
    {agentHandoffKind&&<ContinueInAgentModal clients={agentClients} handoff={agentHandoff} kind={agentHandoffKind} loading={Boolean(agentHandoffBusy)} error={agentHandoffError} onSelect={selectAgent} onBack={()=>{setAgentHandoff(undefined);setAgentHandoffError('')}} onClose={()=>{setAgentHandoff(undefined);setAgentHandoffKind(undefined);setAgentHandoffError('')}}/>}
  </div>;
}

function FocusedRevision({value,expectedDigest,stale,handoffBusy,onContinue}:{value:SubmissionRevisionView;expectedDigest:string;stale:boolean;handoffBusy:boolean;onContinue:()=>Promise<void>}) {
  const revision=value.revision;
  const hasFeedback=value.comments.length>0||value.submission.status==='changes_requested';
  const disclosure=summarizeDisclosure(revision);
  return <><section className={`v3-focus-strip ${stale?'is-stale':''}`} aria-label="当前深链焦点">
    <div><span className="section-kicker">Focused Revision</span><h2>{value.submission.submission_type} · Revision {revision.revision_no}</h2><p>{shortID(revision.id)} · {value.comments.length} 条审核评论</p></div>
    <dl><div><dt>当前摘要</dt><dd><code>{normalizeDigest(revision.content_hash)}</code></dd></div>{expectedDigest&&<div><dt>链接摘要</dt><dd><code>{expectedDigest}</code></dd></div>}</dl>
    <div className="v3-focus-state">{stale?<><AlertTriangle size={18}/><span><b>版本摘要不匹配</b><small>保留历史读取，禁止将决定静默应用到其他版本。</small></span></>:<><ShieldCheck size={18}/><span><b>不可变版本已验证</b><small>页面焦点与链接摘要一致。</small></span></>}<Status value={value.submission.status}/>{hasFeedback&&<Button variant="secondary" disabled={handoffBusy} onClick={()=>void onContinue()}>{handoffBusy?<LoaderCircle className="is-spinning" size={15}/>:<MonitorUp size={15}/>}在 Agent 中修订</Button>}</div>
  </section><section className={`v3-disclosure-state ${disclosure.limited?'is-limited':''}`} aria-label="来源披露状态">
    {disclosure.limited?<EyeOff size={18}/>:<ShieldCheck size={18}/>}<div><strong>{disclosure.limited?'证据披露受限':'来源披露范围'}</strong><p>{disclosure.limited?'当前 Revision 的证据不足以支持完整审核；页面只显示披露元数据，不展示未授权 Evidence 正文。':disclosure.total?'页面仅按当前 Revision 已声明的级别展示来源，不扩大披露范围。':'当前 Revision 未声明来源披露。'}</p></div>
    <dl><div><dt>Metadata</dt><dd>{disclosure.metadataOnly}</dd></div><div><dt>Evidence Pack</dt><dd>{disclosure.evidencePack}</dd></div><div><dt>Full Source</dt><dd>{disclosure.fullSource}</dd></div>{disclosure.unknown>0&&<div><dt>Unknown</dt><dd>{disclosure.unknown}</dd></div>}</dl>
  </section></>;
}

function SetupView({projection,connect,clients,canInitialize,busy,onInitialize,onNavigate}:{projection?:ProjectProjection;connect?:ConnectSession;clients?:AgentClient[];canInitialize:boolean;busy:boolean;onInitialize?:()=>Promise<void>;onNavigate:(target:ProjectNavigationTarget)=>void}) {
  const connected=(projection?.sections.onboarding?.count||0)>0;
  const steps=[
    {label:'云端项目',detail:'项目与角色边界已建立',done:Boolean(projection)},
    {label:'本地 Workspace',detail:connected?'至少一台设备已经绑定':'等待 Agent 完成初始化',done:connected,current:!connected},
    {label:'可信知识',detail:'形成可审核的 Knowledge Revision',done:(projection?.sections.knowledge?.count||0)>0,current:connected&&(projection?.sections.knowledge?.count||0)===0},
    {label:'内容生产',detail:'以批准快照启动 ContentBatch',done:(projection?.sections.creative?.count||0)>0}
  ];
  return <>
    {connect&&isActiveConnectState(connect.state)&&<Banner kind="info"><b>{connectStateCopy(connect).title}</b>：{connectStateCopy(connect).detail}</Banner>}
    <section className="v3-steps" aria-label="初始化进度">{steps.map((step,index)=><article key={step.label} className={`${step.done?'is-done':''} ${step.current?'is-current':''}`}><span>{step.done?<CheckCircle2 size={16}/>:index+1}</span><div><strong>{step.label}</strong><small>{step.detail}</small></div></article>)}</section>
    <div className="v3-two-column v3-setup-columns">
      <section className="v3-panel"><header><div><span className="section-kicker">Workspace Binding</span><h2>{connected?'创作环境已连接':'等待初始化'}</h2></div><Status value={connected?'connected':'waiting_for_computer'}/></header><div className="v3-panel-body v3-setup-state">{connected?<ShieldCheck size={28}/>:<Laptop2 size={28}/>}<div><strong>{connected?`${projection?.sections.onboarding.count} 台设备已绑定`:'初始化 V3 Agent Workspace'}</strong><p>{connected?'本地负责创作，服务端负责 Assignment、Submission、Decision 和正式快照。':'当前由 Codex Adapter 执行初始化；其他客户端按能力逐步接入。'}</p></div></div>{!connected&&clients&&<div className="v3-agent-reservation">{clients.map(client=>{const available=capabilityStatus(client,'workspace_bootstrap')==='available';return <a href={`/docs/clients/${client.id}`} target="_blank" rel="noreferrer" key={client.id} className={available?'is-available':''}><b>{client.display_name}</b><small>{available?'可用':'即将支持'}</small></a>})}</div>}{!connected&&<footer><a className="button button-ghost" href="/docs/clients/codex" target="_blank" rel="noreferrer"><BookOpen size={15}/>接入指南</a><Button disabled={!canInitialize||busy} onClick={()=>void onInitialize?.()}>{busy?<LoaderCircle className="is-spinning" size={16}/>:<Laptop2 size={16}/>}使用 Codex 初始化</Button></footer>}</section>
      <NextActions actions={projection?.next_actions||[]} onNavigate={onNavigate} onInitialize={onInitialize}/>
    </div>
  </>;
}

function Overview({projection,onNavigate,onNavigateTarget}:{projection?:ProjectProjection;onNavigate:(view:ProjectView)=>void;onNavigateTarget:(target:ProjectNavigationTarget)=>void}) {
  const sections=projection?.sections||{};
  return <div className="v3-overview-layout">
    <div className="v3-layout-next"><NextActions actions={projection?.next_actions||[]} onNavigate={onNavigateTarget}/></div>
    <section className="v3-metrics v3-layout-metrics">
      <Metric label="已连接设备" value={sections.onboarding?.count||0} detail="Workspace Binding" tone="source"/>
      <Metric label="待审核 Revision" value={projection?.governance.pending_reviews||0} detail="需要人工决定" tone="warning"/>
      <Metric label="被阻断批次" value={projection?.governance.blocked_content_batches||0} detail="不可进入 Delivery" tone="danger"/>
      <Metric label="正式快照" value={projection?.snapshots.length||0} detail="ApprovedSnapshot" tone="success"/>
    </section>
    <section className="v3-panel v3-flow v3-layout-flow"><header><div><span className="section-kicker">Project Flow</span><h2>业务流程</h2></div><span className="v3-generated">更新于 {formatDate(projection?.generated_at)}</span></header><div className="v3-flow-grid">{stageOrder.map(stage=><button key={stage} onClick={()=>onNavigate(viewForSection(stage))}><span className={`v3-state-dot is-${sections[stage]?.status||'empty'}`}/><strong>{stageLabels[stage]}</strong><small>{stageSummary(sections[stage])}</small></button>)}</div></section>
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
      <Metric label="提交版本" value={section.count} detail="Submission" tone="source"/>
      <Metric label="等待决定" value={section.pending} detail="Revision review" tone="warning"/>
      <Metric label="开放阻断" value={section.blocked} detail="需要补料或修订" tone="danger"/>
      <Metric label="正式快照" value={snapshots.length} detail="ApprovedSnapshot" tone="success"/>
    </section>
    {(creativeBlocked||deliveryBlocked)&&<div className="v3-layout-alerts">{creativeBlocked&&<Banner kind="warning">当前有 {projection?.governance.blocked_content_batches} 个 ContentBatch 被阻断，可以进行方向评审，但不能创建 DeliveryPackage。</Banner>}{deliveryBlocked&&<Banner kind="warning">尚无具备正式资格的 ContentBatch 或 AssetBatch 快照，交付门禁未满足。</Banner>}</div>}
    <div className="v3-layout-revisions"><RecentSubmissions values={submissions} onReview={()=>onNavigate('review')}/></div>
    <div className="v3-layout-snapshots"><SnapshotList values={snapshots}/></div>
  </div>;
}

function RecentSubmissions({values,onReview}:{values:ProjectionSubmission[];onReview:()=>void}) {
  return <section className="v3-panel"><header><div><span className="section-kicker">Revision Queue</span><h2>提交版本</h2></div>{values.length>0&&<Button variant="ghost" onClick={onReview}>进入审核<ArrowRight size={15}/></Button>}</header>{values.length===0?<Empty title="暂无已提交版本" detail="本地候选只有显式 publish 后才会出现在这里。"/>:<div className="v3-table-wrap"><table className="v3-table"><thead><tr><th>类型</th><th>Revision</th><th>状态</th><th>更新时间</th></tr></thead><tbody>{values.map(item=><tr key={item.id}><td><strong>{submissionLabels[item.type]||item.type}</strong><small>{shortID(item.id)}</small></td><td><code>{shortID(item.current_revision_id)}</code></td><td><Status value={item.status}/></td><td>{formatDate(item.updated_at)}</td></tr>)}</tbody></table></div>}</section>;
}

function SnapshotList({values}:{values:ProjectionSnapshot[]}) {
  return <section className="v3-panel"><header><div><span className="section-kicker">Formal Facts</span><h2>批准快照</h2></div><ShieldCheck size={18}/></header>{values.length===0?<Empty title="暂无正式快照" detail="只有绑定明确 Revision 的人工批准才能形成正式事实。"/>:<div className="v3-snapshot-list">{values.map(item=><article key={item.id}><CheckCircle2 size={18}/><div><strong>{submissionLabels[item.type]||item.type}</strong><small>{shortID(item.id)} · {item.eligible_count} 个 eligible 对象</small></div><time>{formatDate(item.created_at)}</time></article>)}</div>}</section>;
}

function NextActions({actions,onNavigate,onInitialize}:{actions:ProjectionAction[];onNavigate:(target:ProjectNavigationTarget)=>void;onInitialize?:()=>Promise<void>}) {
  const action=actions[0];
  if(!action)return <section className="v3-panel v3-next-action"><Clock3 size={22}/><div><span className="section-kicker">Next Action</span><h2>等待新的正式状态</h2><p>当前没有服务端生成的下一动作。</p></div></section>;
  const run=()=>{if(action.id==='initialize-workspace'&&onInitialize){void onInitialize();return}onNavigate(action.navigation)};
  return <section className="v3-panel v3-next-action"><ArrowRight size={22}/><div><span className="section-kicker">Next Action</span><h2>{action.label}</h2><p>{action.reason||actionDescription(action)}</p>{action.enabled&&<Button onClick={run}>{action.label}<ArrowRight size={15}/></Button>}</div></section>;
}

function Metric({label,value,detail,tone}:{label:string;value:number;detail:string;tone:'source'|'warning'|'danger'|'success'}) {return <article className={`v3-metric is-${tone}`}><span>{label}</span><strong>{value}</strong><small>{detail}</small></article>}

function aggregateSections(projection?:ProjectProjection):ProjectionSection {
  if(!projection)return emptySection;
  const values=Object.values(projection.sections);
  const count=values.reduce((sum,item)=>sum+item.count,0);const pending=values.reduce((sum,item)=>sum+item.pending,0);const blocked=values.reduce((sum,item)=>sum+item.blocked,0);
  return {status:blocked?'blocked':pending?'pending':count?'ready':'empty',count,pending,blocked};
}
function viewForSection(section:typeof stageOrder[number]):ProjectView {return ({onboarding:'setup',methodology:'context',knowledge:'knowledge',intelligence:'intelligence',strategy:'strategy',planning:'planning',creative:'creative',review:'review',delivery:'delivery',learning:'learning',automation:'automation'} as const)[section]}
function stageSummary(section?:ProjectionSection){if(!section||section.count===0)return'尚无正式记录';if(section.blocked)return`${section.blocked} 项阻断`;if(section.pending)return`${section.pending} 项待决定`;return`${section.count} 项就绪`}
function actionDescription(action:ProjectionAction){return action.kind==='review'?'处理待审 Revision，并将决定绑定到当前内容摘要。':action.kind==='assignment'?'创建受控任务，让本地 Workspace 基于冻结输入继续工作。':'完成当前业务门禁后进入下一阶段。'}
function shortID(value:string){return value.length>12?`${value.slice(0,8)}…`:value}
function formatDate(value?:string){if(!value)return'—';return new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(new Date(value))}
function normalizeDigest(value:string){const normalized=value.trim().toLowerCase();return normalized.startsWith('sha256:')?normalized:`sha256:${normalized}`}
function message(value:unknown,fallback:string){return value instanceof Error?value.message:fallback}
