import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  Archive,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleHelp,
  Clipboard,
  ClipboardCheck,
  Clock3,
  Download,
  ExternalLink,
  FileCheck2,
  File,
  FileSearch,
  FileSpreadsheet,
  FileText,
  Folder,
  FolderPlus,
  Globe2,
  Image as ImageIcon,
  LayoutGrid,
  Library,
  Lightbulb,
  ListTodo,
  LoaderCircle,
  MoreHorizontal,
  MonitorUp,
  Music2,
  PackageCheck,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Search,
  ShieldCheck,
  Sparkles,
  TerminalSquare,
  Upload,
  Video,
  WandSparkles,
  Workflow,
  X,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useState, type ChangeEvent, type FormEvent, type ReactNode } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { Button, IconButton } from '../components/ui';
import { buildBootstrapPrompt } from '../connectBootstrap';
import { contentTypeLabel, deliveryDestinationLabel, statusLabel } from '../uiLabels';
import { studioApi } from './studioApi';
import { customerTaskTone, studioStepStateLabel } from './studioData';
import { useStudio } from './StudioContext';
import type {
  StudioAssetCatalog,
  StudioAssetItem,
  StudioAssetSurface,
  StudioConnectSession,
  StudioCustomerStep,
  StudioDecision,
  StudioExperience,
  StudioExecutionClient,
  StudioProject,
  StudioTaskSummary,
  StudioTaskView,
  WorkspaceFolderItem,
  WorkspaceMaterialItem,
} from './studioTypes';

type TaskFilter='active'|'attention'|'completed';
type AssetCategory='all'|'persona'|'script'|'storyboard'|'image'|'video';
type AssetStatus='all'|'draft'|'pending_confirmation'|'changes_requested'|'confirmed'|'delivered'|'superseded'|'blocked';
type MaterialCategory='all'|'folder'|WorkspaceMaterialItem['material_kind'];

const filterLabels:Record<TaskFilter,string>={active:'进行中',attention:'等待处理',completed:'已完成'};
const assetCategoryLabels:Record<AssetCategory,string>={all:'所有结果',persona:'人物原型',script:'剧本',storyboard:'分镜',image:'图片',video:'视频'};
const assetStatusLabels:Record<AssetStatus,string>={all:'全部状态',draft:'草稿',pending_confirmation:'待确认',changes_requested:'需修改',confirmed:'已确认',delivered:'已交付',superseded:'已被替代',blocked:'已阻止'};
const materialCategoryLabels:Record<MaterialCategory,string>={all:'全部',folder:'文件夹',document:'文档',image:'图片',video:'视频',audio:'音频',table:'表格',other:'其他'};
const attentionStatuses=['waiting_gate','needs_input','blocked'];
const completedStatuses=['delivered','cancelled','canceled'];
const marketingCustomerStepTitles=['灵感采集','人物原型','营销剧本','视频分镜','候选成片','交付准备'];

export function StudioHomePage(){
  const {bootstrap}=useStudio();
  const {tasks,loading,error,reload}=useTasks();
  const attention=tasks.filter(task=>attentionStatuses.includes(task.status));
  const recent=tasks.slice(0,4);
  const firstName=bootstrap.session.user.display_name.trim().split(/\s+/)[0]||bootstrap.session.user.display_name;

  return <div className="studio-view studio-home">
    <PageHeading eyebrow="今天" title={`${firstName}，今天想推进哪一部分？`} detail="从一个工作面板开始；流程、版本和执行方式由 Content Work OS 在后台承接。"/>
    {error&&<StudioNotice kind="error" onRetry={reload}>{error}</StudioNotice>}
    {bootstrap.experiences.length===0?<StudioUnavailableWorkbench tasks={tasks} operationsPath={bootstrap.session.can_view_operations?bootstrap.session.operations_path:undefined}/>:bootstrap.experiences.map(experience=><StudioExperienceWorkbench key={experience.id} experience={experience} tasks={tasks} canCreate={bootstrap.session.can_create}/>)}
    <div className="studio-home-grid">
      <section className="studio-section"><SectionHeading icon={<ClipboardCheck size={17}/>} title="等待你处理" count={attention.length}/>{loading?<StudioLoading label="正在整理待办…"/>:attention.length===0?<CompactEmpty icon={<CheckCircle2 size={21}/>} title="当前没有待确认事项" detail="需要补资料或确认的任务会显示在这里。"/>:<div className="studio-task-stack">{attention.slice(0,4).map(task=><TaskRow key={task.id} task={task}/>)}</div>}</section>
      <section className="studio-section"><SectionHeading icon={<Clock3 size={17}/>} title="最近任务" count={recent.length} action={<Link to="/studio/tasks">查看全部 <ArrowRight size={14}/></Link>}/>{loading?<StudioLoading label="正在读取任务…"/>:recent.length===0?<CompactEmpty icon={<Sparkles size={21}/>} title="还没有创作任务" detail="从上方选择一个创作目标开始。"/>:<div className="studio-task-stack">{recent.map(task=><TaskRow key={task.id} task={task}/>)}</div>}</section>
    </div>
  </div>;
}

export function StudioConnectPage(){
  const {bootstrap,refresh}=useStudio();
  const navigate=useNavigate();
  const [searchParams]=useSearchParams();
  const requestedSession=searchParams.get('session')||'';
  const requestedProject=searchParams.get('project')||'';
  const activeProjects=bootstrap.projects.filter(project=>project.status!=='archived');
  const initiallySelected=activeProjects.find(project=>project.id===requestedProject)||activeProjects.find(project=>!project.execution_client_connected)||activeProjects[0];
  const [projectID,setProjectID]=useState(initiallySelected?.id||'');
  const [clients,setClients]=useState<StudioExecutionClient[]>([]);
  const [clientsLoaded,setClientsLoaded]=useState(false);
  const [session,setSession]=useState<StudioConnectSession>();
  const [loading,setLoading]=useState(Boolean(requestedSession));
  const [busy,setBusy]=useState('');
  const [error,setError]=useState('');
  const [copied,setCopied]=useState(false);
  const [configurationError,setConfigurationError]=useState('');
  const project=activeProjects.find(item=>item.id===projectID);
  const client=clients.find(item=>item.id==='codex'&&item.available);
  const connectedCount=activeProjects.reduce((sum,item)=>sum+item.connected_client_count,0);
  const hasConnectionContract=Object.prototype.hasOwnProperty.call(bootstrap.session,'can_connect_execution_client');

  useEffect(()=>{let active=true;studioApi.executionClients().then(value=>{if(active)setClients(value.clients)}).catch(value=>{if(!active)return;if((value as {status?:number}).status===404){setConfigurationError('当前创作台与服务版本不一致，请刷新页面或重启开发服务后重试。');setClients([])}else setError(value instanceof Error?value.message:'连接服务暂不可用')}).finally(()=>{if(active)setClientsLoaded(true)});return()=>{active=false}},[]);
  useEffect(()=>{
    if(!requestedSession){setLoading(false);return}
    let active=true;setLoading(true);setError('');
    studioApi.connectSession(requestedSession).then(value=>{if(active){setSession(value);setProjectID(value.project_id)}}).catch(value=>{if(active)setError(value instanceof Error?value.message:'连接状态读取失败')}).finally(()=>{if(active)setLoading(false)});
    return()=>{active=false};
  },[requestedSession]);
  useEffect(()=>{
    if(!session||['connected','failed','expired','canceled'].includes(session.status))return;
    let active=true;
    const timer=window.setInterval(()=>{studioApi.connectSession(session.id).then(value=>{if(active)setSession(value)}).catch(()=>{})},2000);
    return()=>{active=false;window.clearInterval(timer)};
  },[session?.id,session?.status]);
  useEffect(()=>{if(session?.status==='connected')void refresh()},[refresh,session?.status]);

  const start=async()=>{
    if(!project||!client)return;
    setBusy('start');setError('');
    try{const value=await studioApi.createConnectSession(project.id);setSession(value);navigate(`/studio/connect?session=${encodeURIComponent(value.id)}`,{replace:true})}
    catch(value){setError(value instanceof Error?value.message:'创作电脑连接发起失败')}
    finally{setBusy('')}
  };
  const decide=async(decision:'approve'|'deny')=>{
    if(!session)return;setBusy(decision);setError('');
    try{setSession(decision==='approve'?await studioApi.approveConnectSession(session.id):await studioApi.denyConnectSession(session.id))}
    catch(value){setError(value instanceof Error?value.message:'连接确认失败')}
    finally{setBusy('')}
  };
  const cancel=async()=>{
    if(!session)return;setBusy('cancel');setError('');
    try{await studioApi.cancelConnectSession(session.id);setSession(undefined);navigate(project?`/studio/connect?project=${encodeURIComponent(project.id)}`:'/studio/connect',{replace:true})}
    catch(value){setError(value instanceof Error?value.message:'取消连接失败')}
    finally{setBusy('')}
  };
  const reset=()=>{setSession(undefined);setError('');navigate(project?`/studio/connect?project=${encodeURIComponent(project.id)}`:'/studio/connect',{replace:true})};
  const prompt=session&&project?buildBootstrapPrompt({serverURL:window.location.origin,sessionID:session.id,projectName:`${project.brand_name} / ${project.product_name}`}):'';
  const copyPrompt=async()=>{try{await navigator.clipboard.writeText(prompt);setCopied(true);window.setTimeout(()=>setCopied(false),1600)}catch{setError('无法访问剪贴板，请检查浏览器权限后重试')}};

  if(activeProjects.length===0)return <div className="studio-view"><PageHeading eyebrow="开始创作" title="项目准备好后即可开始" detail="当前还没有可用的创作项目。"/><CompactEmpty icon={<MonitorUp size={22}/>} title="等待项目准备" detail="项目创建完成后，这里会自动出现开始入口。" action={bootstrap.session.can_view_operations?<Link className="studio-secondary-link" to={bootstrap.session.operations_path||'/admin/dashboard'}>前往运营与管理</Link>:undefined}/></div>;
  return <div className="studio-view studio-connect-view">
    <PageHeading eyebrow="开始创作" title={session?'连接创作电脑':connectedCount>0?'管理已连接电脑':'连接你的创作电脑'} detail="连接完成后，就可以直接使用团队已经配置好的创作流水线。" actions={connectedCount>0?<span className="studio-connected-count"><CheckCircle2 size={16}/>{connectedCount} 台已连接</span>:undefined}/>
    {error&&<StudioNotice kind="error" onClose={()=>setError('')}>{error}</StudioNotice>}
    {loading?<StudioLoading label="正在读取连接状态…"/>:session?<section className={`studio-connection-state is-${session.status}`}>
      <header><span>{session.status==='connected'?<CheckCircle2 size={24}/>:session.status==='confirmation_required'?<ShieldCheck size={24}/>:session.status==='failed'||session.status==='expired'||session.status==='canceled'?<AlertCircle size={24}/>:<LoaderCircle className={session.status==='connecting'?'is-spinning':''} size={24}/>}</span><div><small>{project?.brand_name} · {client?.display_name||'Codex'}</small><h2>{connectionTitle(session.status)}</h2><p>{session.message}</p></div></header>
      {session.status==='waiting_for_computer'&&<div className="studio-connect-instruction"><ol><li><span>1</span><div><strong>打开 Codex</strong><p>新建一个会话，并选择用于这个项目的本地目录。</p></div></li><li><span>2</span><div><strong>发送连接指令</strong><p>Codex 会先检查环境，再请你回到此页确认电脑。</p></div></li></ol><pre><code>{prompt}</code></pre><div><Button onClick={()=>void copyPrompt()}>{copied?<Check size={16}/>:<Clipboard size={16}/>} {copied?'已复制':'复制连接指令'}</Button><Button variant="secondary" disabled={busy==='cancel'} onClick={()=>void cancel()}>取消</Button></div></div>}
      {session.status==='confirmation_required'&&<div className="studio-connect-confirm"><span>核对代码</span><code>{session.verification_code}</code><p>只有 Codex 中显示相同代码时，才确认连接。</p><div><Button variant="danger" disabled={Boolean(busy)} onClick={()=>void decide('deny')}>不是这台电脑</Button><Button disabled={Boolean(busy)} onClick={()=>void decide('approve')}><ShieldCheck size={16}/>{busy==='approve'?'确认中…':'确认连接'}</Button></div></div>}
      {session.status==='connecting'&&<div className="studio-connect-wait"><LoaderCircle className="is-spinning" size={19}/><span>连接完成后会自动更新。{session.support_code&&<> 支持编号 <code>{session.support_code}</code></>}</span></div>}
      {session.status==='connected'&&<div className="studio-connect-success"><div><CheckCircle2 size={18}/><span><strong>可以开始创作</strong><small>不同步骤仍会按流水线选择本地客户端、平台能力、外部服务或人工处理。</small></span></div><Button onClick={()=>navigate('/studio',{replace:true})}>进入创作台<ArrowRight size={15}/></Button></div>}
      {['failed','expired','canceled'].includes(session.status)&&<div className="studio-connect-recovery">{session.support_code&&<span>支持编号 <code>{session.support_code}</code></span>}<Button onClick={reset}><RefreshCw size={15}/>重新连接</Button></div>}
    </section>:<section className="studio-connect-setup">
      {activeProjects.length>1&&<div className="studio-connect-field"><span>选择创作项目</span><select value={projectID} onChange={event=>setProjectID(event.target.value)}>{activeProjects.map(item=><option value={item.id} key={item.id}>{item.brand_name} · {item.product_name}{item.execution_client_connected?` · 已连接 ${item.connected_client_count}`:''}</option>)}</select></div>}
      {!clientsLoaded&&<StudioLoading label="正在准备连接…"/>}
      {configurationError&&!session&&<StudioNotice kind="error">{configurationError}</StudioNotice>}
      <div className="studio-connect-action"><div><ShieldCheck size={19}/><span><strong>只连接你确认过的电脑</strong><small>当前使用 Codex 完成项目连接；连接后，系统会按需分配本地步骤，其余工作仍由平台自动完成。</small></span></div>{!hasConnectionContract||configurationError?<span className="studio-connect-permission">连接服务暂不可用，请刷新后重试</span>:clientsLoaded&&!client?<span className="studio-connect-permission">当前还没有可用的 Codex 连接方式</span>:bootstrap.session.can_connect_execution_client?<Button disabled={!project||!client||!clientsLoaded||Boolean(busy)} onClick={()=>void start()}><MonitorUp size={16}/>{busy?'正在发起…':'连接我的创作电脑'}</Button>:<span className="studio-connect-permission">当前账号暂不能连接，请联系团队负责人</span>}</div>
    </section>}
  </div>;
}

function StudioExperienceWorkbench({experience,tasks,canCreate}:{experience:StudioExperience;tasks:StudioTaskSummary[];canCreate:boolean}){
  const {bootstrap}=useStudio();
  const scopedTasks=tasks.filter(task=>task.experience_id===experience.id);
  const activeTasks=scopedTasks.filter(task=>!completedStatuses.includes(task.status));
  const primaryTask=activeTasks.find(task=>attentionStatuses.includes(task.status))||activeTasks[0];
  const hasProject=experience.project_ids.length>0;
  const hasConnectedProject=bootstrap.projects.some(project=>experience.project_ids.includes(project.id)&&project.execution_client_connected&&project.status!=='archived');
  const newTaskPath=`/studio/tasks/new?experience=${encodeURIComponent(experience.id)}`;
  const startPath=hasConnectedProject?newTaskPath:'/studio/connect';
  const stepTitles=experience.content_type==='marketing_video'?marketingCustomerStepTitles:experience.step_titles;
  const stepTitle=(index:number,fallback:string)=>stepTitles[index]||fallback;
  const panels=[
    {id:'direction',title:'灵感与人物',detail:'收集可信参考，确认人物定位、受众关系和表达边界。',tone:'source',icon:<Lightbulb size={19}/>,stepIDs:['inspiration','persona'],steps:[stepTitle(0,'灵感采集'),stepTitle(1,'人物原型')],href:startPath,label:hasConnectedProject?'开始策划':'连接创作电脑'},
    {id:'production',title:'剧本与分镜',detail:'确认营销剧本版本，锁定镜头、画面、素材和连续性。',tone:'strategy',icon:<FileText size={19}/>,stepIDs:['script','storyboard'],steps:[stepTitle(2,'营销剧本'),stepTitle(3,'视频分镜')],href:'/studio/tasks',label:'选择创作任务'},
    {id:'delivery',title:'成片与交付',detail:'选择候选成片，完成最终确认并下载固定交付包。',tone:'production',icon:<Video size={19}/>,stepIDs:['media','delivery'],steps:[stepTitle(4,'候选成片'),stepTitle(5,'交付准备')],href:'/studio/deliveries',label:'查看成片与交付'},
    {id:'assets',title:'资产复用',detail:'整理自有资料，并从已确认的人物原型、剧本、分镜、图片或视频结果开始下一次创作。',tone:'knowledge',icon:<Archive size={19}/>,stepIDs:[],steps:['我的资产','创作结果'],href:'/studio/assets',label:'打开资产'},
  ];
  return <section className="studio-workbench" aria-labelledby={`experience-${experience.id}`}>
    <header className="studio-workbench-header"><div><span><WandSparkles size={15}/>已发布创作流水线</span><h2 id={`experience-${experience.id}`}>{experience.name}</h2><p>{experience.description}</p></div><div>{primaryTask&&<Link className="studio-secondary-link" to={`/studio/tasks/${encodeURIComponent(primaryTask.id)}`}><Play size={15}/>继续当前任务</Link>}{canCreate&&hasProject?<Link className="studio-primary-link" to={hasConnectedProject?newTaskPath:'/studio/connect'}>{hasConnectedProject?<Plus size={15}/>:<MonitorUp size={15}/>} {hasConnectedProject?'新建创作任务':'连接创作电脑'}</Link>:<span className="studio-workbench-unavailable">{!hasProject?'等待运营绑定项目':'当前角色仅可查看'}</span>}</div></header>
    <div className="studio-workbench-panels">{panels.map(panel=>{
      const currentTask=activeTasks.find(task=>panel.stepIDs.includes(task.current_step_id));
      const href=currentTask?`/studio/tasks/${encodeURIComponent(currentTask.id)}`:panel.href;
      const label=currentTask?'继续当前任务':panel.label;
      return <article className={`studio-work-panel is-${panel.tone}`} key={panel.id}><header><span>{panel.icon}</span><div><small>创作面板</small><h3>{panel.title}</h3></div>{currentTask&&<StatusBadge task={currentTask}/>}</header><p>{panel.detail}</p><div className="studio-work-panel-steps">{panel.steps.map((step,index)=><span key={step}><b>{String(index+1).padStart(2,'0')}</b>{step}</span>)}</div><Link to={href}>{label}<ArrowRight size={14}/></Link></article>;
    })}</div>
  </section>;
}

function StudioUnavailableWorkbench({tasks,operationsPath}:{tasks:StudioTaskSummary[];operationsPath?:string}){
  const activeTask=tasks.find(task=>!completedStatuses.includes(task.status));
  return <section className="studio-workbench is-unavailable"><header className="studio-workbench-header"><div><span><CircleHelp size={15}/>暂时不能新建任务</span><h2>先继续已有工作，新的创作场景准备好后会出现在这里</h2><p>你仍然可以处理任务、整理资料、复用创作结果和下载已经确认的交付文件。</p></div>{operationsPath&&<Link className="studio-secondary-link" to={operationsPath}>前往运营工作台<ArrowRight size={14}/></Link>}</header><div className="studio-recovery-actions">{activeTask&&<Link to={`/studio/tasks/${encodeURIComponent(activeTask.id)}`}><Play size={17}/><span><strong>继续当前任务</strong><small>{activeTask.title} · {activeTask.next_action}</small></span><ArrowRight size={14}/></Link>}<Link to="/studio/tasks"><ListTodo size={17}/><span><strong>查看创作任务</strong><small>继续已有任务或处理待确认事项</small></span><ArrowRight size={14}/></Link><Link to="/studio/assets"><Archive size={17}/><span><strong>打开资产</strong><small>整理资料并查找可复用结果</small></span><ArrowRight size={14}/></Link><Link to="/studio/deliveries"><PackageCheck size={17}/><span><strong>查看交付</strong><small>下载交付包并核对发布记录</small></span><ArrowRight size={14}/></Link></div></section>;
}

export function StudioTasksPage(){
  const {bootstrap}=useStudio();
  const [filter,setFilter]=useState<TaskFilter>('active');
  const [query,setQuery]=useState('');
  const {tasks,loading,error,reload}=useTasks();
  const counts=useMemo(()=>({active:tasks.filter(task=>!completedStatuses.includes(task.status)).length,attention:tasks.filter(task=>attentionStatuses.includes(task.status)).length,completed:tasks.filter(task=>completedStatuses.includes(task.status)).length}),[tasks]);
  const visible=tasks.filter(task=>{
    const matchesFilter=filter==='attention'?attentionStatuses.includes(task.status):filter==='completed'?completedStatuses.includes(task.status):!completedStatuses.includes(task.status);
    return matchesFilter&&(!query.trim()||task.title.toLowerCase().includes(query.trim().toLowerCase()));
  });
  const canStartNewTask=bootstrap.projects.some(project=>project.status!=='archived'&&project.execution_client_connected);
  return <div className="studio-view">
    <PageHeading eyebrow="创作任务" title="所有创作任务" detail="按需要处理的状态查看即可。" actions={bootstrap.session.can_create?<Link className="studio-primary-link" to={canStartNewTask?'/studio/tasks/new':'/studio/connect'}>{canStartNewTask?<Plus size={16}/>:<MonitorUp size={16}/>} {canStartNewTask?'新建任务':'连接创作电脑'}</Link>:undefined}/>
    {error&&<StudioNotice kind="error" onRetry={reload}>{error}</StudioNotice>}
    <div className="studio-toolbar"><div className="studio-segments" role="tablist" aria-label="任务筛选">{(Object.keys(filterLabels) as TaskFilter[]).map(id=><button type="button" role="tab" aria-selected={filter===id} className={filter===id?'is-active':''} key={id} onClick={()=>setFilter(id)}>{filterLabels[id]}<span>{counts[id]}</span></button>)}</div><label className="studio-search"><Search size={16}/><input value={query} onChange={event=>setQuery(event.target.value)} placeholder="搜索任务" aria-label="搜索任务"/></label></div>
    <section className="studio-section studio-task-list" aria-label={`${filterLabels[filter]}任务`}>{loading?<StudioLoading label="正在读取创作任务…"/>:visible.length===0?<CompactEmpty icon={<FileSearch size={22}/>} title={tasks.length?'没有匹配的任务':'还没有创作任务'} detail={tasks.length?'换一个筛选条件或搜索词。':'从一个明确的创作目标开始。'} action={!tasks.length&&bootstrap.session.can_create?<Link className="studio-secondary-link" to={canStartNewTask?'/studio/tasks/new':'/studio/connect'}>{canStartNewTask?<Plus size={15}/>:<MonitorUp size={15}/>} {canStartNewTask?'新建任务':'连接创作电脑'}</Link>:undefined}/>:visible.map(task=><TaskRow key={task.id} task={task} roomy/>)}</section>
  </div>;
}

export function StudioNewTaskPage(){
  const {bootstrap}=useStudio();
  const navigate=useNavigate();
  const [searchParams]=useSearchParams();
  const requestedProject=searchParams.get('project')||'';
  const requestedAssetRef=searchParams.get('asset_ref')||'';
  const requestedMaterialRef=searchParams.get('material_ref')||'';
  const initialExperience=bootstrap.experiences.find(item=>item.id===searchParams.get('experience'))||bootstrap.experiences[0];
  const [experienceID,setExperienceID]=useState(initialExperience?.id||'');
  const experience=bootstrap.experiences.find(item=>item.id===experienceID);
  const projects=bootstrap.projects.filter(project=>experience?.project_ids.includes(project.id));
  const [projectID,setProjectID]=useState(projects.find(item=>item.id===requestedProject)?.id||projects.find(item=>item.execution_client_connected)?.id||projects[0]?.id||'');
  const [title,setTitle]=useState('');
  const [goal,setGoal]=useState('');
  const [inspiration,setInspiration]=useState('');
  const [catalog,setCatalog]=useState<StudioAssetCatalog>();
  const [materials,setMaterials]=useState<WorkspaceMaterialItem[]>([]);
  const [selectedRefs,setSelectedRefs]=useState<string[]>([]);
  const [selectedMaterialRefs,setSelectedMaterialRefs]=useState<string[]>([]);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');
  const project=projects.find(item=>item.id===projectID);
  const canSubmit=Boolean(experience&&project&&title.trim().length>=3&&goal.trim().length>=5);

  useEffect(()=>{
    const nextProjects=bootstrap.projects.filter(item=>bootstrap.experiences.find(value=>value.id===experienceID)?.project_ids.includes(item.id));
    if(!nextProjects.some(item=>item.id===projectID))setProjectID(nextProjects.find(item=>item.id===requestedProject)?.id||nextProjects.find(item=>item.execution_client_connected)?.id||nextProjects[0]?.id||'');
  },[bootstrap.experiences,bootstrap.projects,experienceID,projectID,requestedProject]);
  useEffect(()=>{setSelectedRefs([]);setSelectedMaterialRefs([]);setCatalog(undefined);setMaterials([]);if(projectID)void studioApi.assets(projectID).then(nextSurface=>{const nextCatalog=nextSurface.creative_results;setCatalog(nextCatalog);setMaterials(nextSurface.workspace.materials);if(requestedAssetRef&&nextCatalog.items.some(item=>item.ref===requestedAssetRef&&item.reusable))setSelectedRefs([requestedAssetRef]);if(requestedMaterialRef&&nextSurface.workspace.materials.some(item=>item.material_ref===requestedMaterialRef&&item.processing_state!=='failed'))setSelectedMaterialRefs([requestedMaterialRef])}).catch(value=>setError(value instanceof Error?value.message:'资产加载失败'))},[projectID,requestedAssetRef,requestedMaterialRef]);

  const submit=async(event:FormEvent)=>{
    event.preventDefault();
    if(!experience||!project||!canSubmit)return;
    setBusy(true);setError('');
    try{
      const created=await studioApi.createTask({experience_id:experience.id,project_id:project.id,title:title.trim(),goal:goal.trim(),inspiration:inspiration.trim(),asset_refs:selectedRefs,material_refs:selectedMaterialRefs,idempotency_key:createIdempotencyKey()});
      navigate(`/studio/tasks/${encodeURIComponent(created.task.id)}`);
    }catch(value){setError(value instanceof Error?value.message:'任务创建失败')}finally{setBusy(false)}
  };

  if(!bootstrap.session.can_create)return <div className="studio-view"><PageHeading eyebrow="新建任务" title="当前账号没有创作权限" detail="你仍可以查看任务、资产和交付结果。"/><CompactEmpty icon={<CircleHelp size={22}/>} title="需要创作权限" detail="请联系团队管理员调整当前账号的角色。" action={<Link className="studio-secondary-link" to="/studio">返回今天</Link>}/></div>;
  if(!experience||!project)return <div className="studio-view"><PageHeading eyebrow="新建任务" title="当前还不能开始新的创作" detail="新的创作场景准备好后，会自动出现在今天页面。"/><CompactEmpty icon={<CircleHelp size={22}/>} title="暂时没有可用场景" detail="请联系团队负责人，或先继续已有任务。" action={<Link className="studio-secondary-link" to="/studio/tasks">查看已有任务</Link>}/></div>;
  if(!project.execution_client_connected)return <div className="studio-view"><PageHeading eyebrow="新建任务" title="先连接你的创作电脑" detail="连接完成后即可开始新的创作，已有任务、资产和交付仍可查看。"/><CompactEmpty icon={<MonitorUp size={22}/>} title={`${project.brand_name} 尚未连接创作电脑`} detail="连接完成后，就可以使用团队已经配置好的创作流水线。" action={<Link className="studio-primary-link" to={`/studio/connect?project=${encodeURIComponent(project.id)}`}><MonitorUp size={15}/>连接创作电脑</Link>}/></div>;

  const reusableAssets=(catalog?.items||[]).filter(item=>item.reusable);
  const reusableMaterials=materials.filter(item=>item.processing_state!=='failed');
  return <div className="studio-view studio-new-task">
    <button className="studio-back" type="button" onClick={()=>navigate(-1)}><ArrowLeft size={15}/>返回</button>
    <PageHeading eyebrow="新建创作任务" title={`制作一条${experience.name}`} detail="先说清楚这次想实现的业务目标，其余执行细节由创作流水线承接。"/>
    <form className="studio-form-layout" onSubmit={submit}>
      <main className="studio-form-main">
        {bootstrap.experiences.length>1&&<label className="studio-field"><span>创作目标</span><select value={experienceID} onChange={event=>setExperienceID(event.target.value)}>{bootstrap.experiences.map(item=><option value={item.id} key={item.id}>{item.name}</option>)}</select></label>}
        {projects.length>1&&<label className="studio-field"><span>所属项目</span><select value={projectID} onChange={event=>setProjectID(event.target.value)}>{projects.map(item=><option value={item.id} key={item.id}>{item.brand_name} · {item.product_name}</option>)}</select></label>}
        <label className="studio-field"><span>任务名称</span><input value={title} onChange={event=>setTitle(event.target.value)} placeholder="例如：春季新品主理人人设短片" autoFocus/><small>用团队一眼能识别的名称。</small></label>
        <label className="studio-field"><span>这次想达成什么？</span><textarea value={goal} onChange={event=>setGoal(event.target.value)} rows={5} placeholder="例如：让第一次接触品牌的用户认识主理人，并愿意收藏或咨询。"/><small>写业务结果、目标受众和必须遵守的边界即可。</small></label>
        <label className="studio-field"><span>已有灵感或参考 <em>可选</em></span><textarea value={inspiration} onChange={event=>setInspiration(event.target.value)} rows={4} placeholder="粘贴链接、描述一个人物、写下观察，或说明想参考的内容方向。"/></label>
        <MaterialPicker items={reusableMaterials} selectedRefs={selectedMaterialRefs} onChange={setSelectedMaterialRefs}/>
        <AssetPicker items={reusableAssets} selectedRefs={selectedRefs} onChange={setSelectedRefs}/>
        <div className="studio-form-actions"><Button variant="secondary" type="button" onClick={()=>navigate(-1)}>取消</Button><Button type="submit" disabled={!canSubmit||busy}><Sparkles size={16}/>{busy?'正在创建…':'创建并进入任务'}</Button></div>
        {error&&<StudioNotice kind="error">{error}</StudioNotice>}
      </main>
      <aside className="studio-form-summary"><span>本次使用</span><strong>{experience.name}</strong><ol>{experience.step_titles.map((step,index)=><li key={step}>{index===0?<Lightbulb size={15}/>:index<3?<FileText size={15}/>:index<5?<Video size={15}/>:<PackageCheck size={15}/>} {step}</li>)}</ol><small>{project.brand_name} · {contentTypeLabel(project.content_type)} · 已选 {selectedRefs.length} 项资产</small></aside>
    </form>
  </div>;
}

export function StudioTaskPage(){
  const {bootstrap}=useStudio();
  const {taskID}=useParams();
  const navigate=useNavigate();
  const [view,setView]=useState<StudioTaskView>();
  const [loading,setLoading]=useState(true);
  const [busy,setBusy]=useState('');
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const reload=useCallback(async()=>{if(!taskID)return;setError('');try{setView(await studioApi.task(taskID))}catch(value){setError(value instanceof Error?value.message:'任务读取失败')}},[taskID]);
  useEffect(()=>{setLoading(true);void reload().finally(()=>setLoading(false))},[reload]);
  const run=async(key:string,operation:()=>Promise<StudioTaskView>,success:string)=>{setBusy(key);setError('');setNotice('');try{setView(await operation());setNotice(success)}catch(value){setError(value instanceof Error?value.message:'操作失败')}finally{setBusy('')}};

  if(loading)return <div className="studio-view"><StudioLoading label="正在打开创作任务…"/></div>;
  if(error&&!view)return <div className="studio-view"><button className="studio-back" type="button" onClick={()=>navigate('/studio/tasks')}><ArrowLeft size={15}/>返回任务列表</button><CompactEmpty icon={<AlertCircle size={22}/>} title="任务暂时不可用" detail={error} action={<Button variant="secondary" onClick={()=>void reload()}>重试</Button>}/></div>;
  if(!view)return null;
  const {task}=view;
  const current=view.steps.find(step=>['working','ready','needs_input','needs_decision','blocked'].includes(step.status))||view.steps[view.steps.length-1];
  const action=view.allowed_actions.find(value=>['start','resume','retry'].includes(value));
  const actionLabel=action?{start:'开始创作',resume:'继续创作',retry:'重新尝试'}[action]:'';
  const experience=bootstrap.experiences.find(item=>item.id===task.experience_id);

  return <div className="studio-view studio-task-detail">
    <button className="studio-back" type="button" onClick={()=>navigate('/studio/tasks')}><ArrowLeft size={15}/>返回任务列表</button>
    <header className="studio-task-header"><div><span>{task.project.brand_name} · {experience?.name||'创作任务'}</span><h1>{task.title}</h1><p>{task.intent||'本任务将按已配置的创作流水线推进。'}</p></div><div><StatusBadge task={task}/>{action&&<Button disabled={Boolean(busy)} onClick={()=>void run(`action-${action}`,()=>studioApi.taskAction(task.id,action),`${actionLabel}已提交。`)}>{action==='retry'?<RefreshCw size={15}/>:<Play size={15}/>} {busy?`${actionLabel}中…`:actionLabel}</Button>}{view.allowed_actions.includes('pause')&&<Button variant="secondary" disabled={Boolean(busy)} onClick={()=>void run('pause',()=>studioApi.taskAction(task.id,'pause'),'任务已暂停。')}><Pause size={15}/>暂停</Button>}</div></header>
    {error&&<StudioNotice kind="error" onRetry={reload}>{error}</StudioNotice>}{notice&&<StudioNotice kind="success" onClose={()=>setNotice('')}>{notice}</StudioNotice>}
    <CustomerProgress steps={view.steps}/>
    <div className="studio-task-layout">
      <main>
        <section className="studio-current-work"><header><span>当前步骤</span><h2>{current.title}</h2><p>{current.outcome_description}</p></header>{current.id==='inspiration'?<InspirationStage taskID={task.id} inspirations={view.inspirations} experience={experience} canAdd={bootstrap.session.can_create} onChanged={setView}/>:view.pending_decisions.length?<DecisionPanel decisions={view.pending_decisions} busy={busy} onDecision={(decisionID,decision)=>void run(`decision-${decisionID}-${decision}`,()=>studioApi.decide(task.id,decisionID,decision),decision==='approved'?'已确认，任务会继续进入下一步。':'修改意见已记录。')}/>:<CurrentStepSummary task={task} step={current}/>}</section>
        <section className="studio-section studio-results"><SectionHeading icon={<FileCheck2 size={17}/>} title="当前成果" count={view.results.length}/>{view.results.length===0?<CompactEmpty icon={<Workflow size={21}/>} title="成果正在形成" detail="每个需要你判断的版本都会固定保存在这里。"/>:<div className="studio-result-list">{view.results.map(result=><ResultRow key={result.id} result={result} taskID={task.id}/>)}</div>}</section>
        <section className="studio-section studio-attached-assets"><SectionHeading icon={<Archive size={17}/>} title="本次使用的创作结果" count={view.attached_assets.length} action={<Link to={`/studio/assets?task_id=${encodeURIComponent(task.id)}`}>加入资产 <Plus size={14}/></Link>}/>{view.attached_assets.length===0?<CompactEmpty icon={<Archive size={20}/>} title="还没有复用已有结果" detail="可以从资产入口加入工作区资料，或复用已确认的创作结果。"/>:<div>{view.attached_assets.map(item=><AssetRow key={item.ref} item={item}/>)}</div>}</section>
      </main>
      <aside className="studio-task-aside"><span>下一步</span><strong>{task.next_action}</strong><p>{task.status==='waiting_gate'?'你的决定会固定当前版本，并决定流水线下一步。':'系统会在需要补资料或确认时通知你。'}</p><dl><div><dt>当前项目</dt><dd>{task.project.brand_name}</dd></div><div><dt>最近更新</dt><dd>{formatDate(task.updated_at)}</dd></div><div><dt>任务输入</dt><dd>{task.asset_count} 项</dd></div></dl><Link to={`/studio/assets?task_id=${encodeURIComponent(task.id)}`}><Archive size={15}/>加入资料或结果</Link></aside>
    </div>
  </div>;
}

export function StudioAssetsPage(){
  const {bootstrap}=useStudio();
  const [searchParams]=useSearchParams();
  const requestedTaskID=searchParams.get('task_id')||'';
  const [surface,setSurface]=useState<StudioAssetSurface>();
  const [view,setView]=useState<'mine'|'results'|'recent'>('mine');
  const [materialCategory,setMaterialCategory]=useState<MaterialCategory>('all');
  const [category,setCategory]=useState<AssetCategory>('all');
  const [status,setStatus]=useState<AssetStatus>('all');
  const [query,setQuery]=useState('');
  const [projectID,setProjectID]=useState(bootstrap.projects.find(project=>project.status!=='archived')?.id||'');
  const [folderRef,setFolderRef]=useState('');
  const [folderName,setFolderName]=useState('');
  const [showFolderForm,setShowFolderForm]=useState(false);
  const [loading,setLoading]=useState(true);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const load=useCallback(async()=>{setLoading(true);setError('');try{setSurface(await studioApi.assets(projectID||undefined))}catch(value){setError(value instanceof Error?value.message:'资产加载失败')}finally{setLoading(false)}},[projectID]);
  useEffect(()=>{void load()},[load]);
  const catalog=surface?.creative_results;
  const materials=surface?.workspace.materials||[];
  const folders=surface?.workspace.folders||[];
  const currentFolder=folders.find(folder=>folder.folder_ref===folderRef);
  const visibleResults=(catalog?.items||[]).filter(item=>{
    const matchesCategory=category==='all'||item.result_type===category;
    const matchesStatus=status==='all'||item.status===status;
    const matchesQuery=!query.trim()||`${item.title} ${item.summary} ${item.project_name}`.toLowerCase().includes(query.trim().toLowerCase());
    return matchesCategory&&matchesStatus&&matchesQuery;
  });
  const visibleFolders=folders.filter(folder=>{
    const matchesParent=(folder.parent_ref||'')===folderRef;
    const matchesQuery=!query.trim()||`${folder.name} ${folder.project_name}`.toLowerCase().includes(query.trim().toLowerCase());
    return matchesParent&&matchesQuery&&(materialCategory==='all'||materialCategory==='folder');
  });
  const visibleMaterials=materials.filter(item=>{
    const matchesFolder=!folderRef||item.folder_ref===folderRef;
    const matchesCategory=materialCategory==='all'||(materialCategory!=='folder'&&item.material_kind===materialCategory);
    const matchesQuery=!query.trim()||`${item.title} ${item.file_name} ${item.project_name}`.toLowerCase().includes(query.trim().toLowerCase());
    return matchesFolder&&matchesCategory&&matchesQuery;
  });
  const createFolder=async(event:FormEvent)=>{event.preventDefault();if(!projectID||!folderName.trim())return;setBusy(true);setError('');try{await studioApi.createFolder({project_id:projectID,parent_ref:folderRef||undefined,name:folderName.trim()});setFolderName('');setShowFolderForm(false);setNotice('文件夹已创建');void load()}catch(value){setError(value instanceof Error?value.message:'文件夹创建失败')}finally{setBusy(false)}};
  const upload=async(event:ChangeEvent<HTMLInputElement>)=>{const file=event.target.files?.[0];event.target.value='';if(!file||!projectID)return;setBusy(true);setError('');try{await studioApi.uploadMaterial({project_id:projectID,folder_ref:folderRef||undefined,file});setNotice(`“${file.name}”已加入我的资产`);void load()}catch(value){setError(value instanceof Error?value.message:'资料上传失败')}finally{setBusy(false)}};
  const recentMaterials=surface?.recent.materials||[];
  const recentResults=surface?.recent.results||[];
  const chooseView=(next:'mine'|'results'|'recent')=>{setView(next);setQuery('');setShowFolderForm(false)};
  const changeProject=(next:string)=>{setProjectID(next);setFolderRef('')};

  return <div className="studio-view studio-assets">
    <PageHeading eyebrow="资产" title="资产工作区" detail="文件资料与创作结果" actions={view==='mine'?<Button variant="secondary" disabled={!projectID} onClick={()=>setShowFolderForm(value=>!value)}><FolderPlus size={15}/>{showFolderForm?'取消创建':'新建文件夹'}</Button>:undefined}/>
    {error&&<StudioNotice kind="error" onRetry={load}>{error}</StudioNotice>}{notice&&<StudioNotice kind="success" onClose={()=>setNotice('')}>{notice}</StudioNotice>}
    <div className="studio-asset-toolbar">
      <div className="studio-asset-views" role="tablist" aria-label="资产视图">
        <button type="button" role="tab" aria-selected={view==='mine'} className={view==='mine'?'is-active':''} onClick={()=>chooseView('mine')}>我的资产<span>{(surface?.workspace.counts.all||0)+folders.length}</span></button>
        <button type="button" role="tab" aria-selected={view==='results'} className={view==='results'?'is-active':''} onClick={()=>chooseView('results')}>创作结果<span>{catalog?.counts.all||0}</span></button>
        <button type="button" role="tab" aria-selected={view==='recent'} className={view==='recent'?'is-active':''} onClick={()=>chooseView('recent')}>最近使用</button>
      </div>
      <div className="studio-asset-filters">
        <label className="studio-asset-type-filter"><span>项目</span><select value={projectID} onChange={event=>changeProject(event.target.value)}><option value="">全部项目</option>{bootstrap.projects.filter(project=>project.status!=='archived').map(project=><option value={project.id} key={project.id}>{project.brand_name}</option>)}</select></label>
        {view==='results'&&<label className="studio-asset-type-filter"><span>状态</span><select value={status} onChange={event=>setStatus(event.target.value as AssetStatus)}>{(Object.keys(assetStatusLabels) as AssetStatus[]).map(id=><option value={id} key={id}>{assetStatusLabels[id]}</option>)}</select></label>}
        <label className="studio-search"><Search size={16}/><input value={query} onChange={event=>setQuery(event.target.value)} placeholder={view==='results'?'搜索创作结果':'搜索资产'} aria-label="搜索资产"/></label>
      </div>
    </div>
    {showFolderForm&&<form className="studio-folder-form" onSubmit={createFolder}><label><span>所属项目</span><select value={projectID} onChange={event=>setProjectID(event.target.value)} required><option value="">选择项目</option>{bootstrap.projects.filter(project=>project.status!=='archived').map(project=><option value={project.id} key={project.id}>{project.brand_name}</option>)}</select></label><label><span>文件夹名称</span><input value={folderName} onChange={event=>setFolderName(event.target.value)} placeholder="例如：品牌素材" autoFocus required/></label><Button disabled={busy||!projectID||!folderName.trim()}><FolderPlus size={15}/>创建文件夹</Button></form>}
    {loading?<StudioLoading label="正在整理资产…"/>:view==='mine'?<>
      <AssetCategoryTabs categories={(Object.keys(materialCategoryLabels) as MaterialCategory[])} active={materialCategory} labels={materialCategoryLabels} count={id=>id==='all'?materials.length+folders.length:id==='folder'?folders.length:surface?.workspace.counts[id]||0} onChange={setMaterialCategory}/>
      <div className="studio-asset-workspace">
        <section className="studio-asset-library">
          <div className="studio-asset-location"><div><LayoutGrid size={16}/><strong>{currentFolder?.name||'全部资产'}</strong><span>{visibleFolders.length+visibleMaterials.length} 项</span></div>{currentFolder&&<button type="button" onClick={()=>setFolderRef(currentFolder.parent_ref||'')}>返回上一级</button>}</div>
          <div className="studio-asset-grid">
            {visibleFolders.map(folder=><FolderAssetCard key={folder.folder_ref} folder={folder} onOpen={()=>setFolderRef(folder.folder_ref)}/>)}
            {visibleMaterials.map(item=><WorkspaceMaterialCard key={item.material_ref} item={item} taskID={requestedTaskID}/>)}
            {materialCategory==='folder'?<button className="studio-asset-import-card" type="button" disabled={!projectID} onClick={()=>setShowFolderForm(true)}><FolderPlus size={28}/><strong>新建文件夹</strong><small>{projectID?'整理当前项目资产':'先选择一个项目'}</small></button>:<label className={`studio-asset-import-card ${!projectID||busy?'is-disabled':''}`}><Upload size={28}/><strong>{busy?'正在导入…':'导入资产'}</strong><small>{projectID?'文档、图片、视频、音频或表格':'先选择一个项目'}</small><input type="file" accept=".pdf,.docx,.xlsx,.pptx,.png,.jpg,.jpeg,.mp4,.mov,.webm,.mp3,.wav,.m4a,.csv,.md,.txt" onChange={upload} disabled={busy||!projectID}/></label>}
          </div>
        </section>
      </div>
    </>:view==='results'?<>
      <AssetCategoryTabs categories={(Object.keys(assetCategoryLabels) as AssetCategory[])} active={category} labels={assetCategoryLabels} count={id=>catalog?.counts[id]||0} onChange={setCategory}/>
      <div className="studio-asset-workspace">
        <section className="studio-asset-library">
          {visibleResults.length===0?<CompactEmpty icon={<Library size={22}/>} title="没有符合条件的创作结果" detail="换一个类型、状态或搜索词。"/>:<div className="studio-asset-grid">{visibleResults.map(item=><CreativeResultCard key={`${item.result_type}:${item.project_id}:${item.ref||item.title}`} item={item} taskID={requestedTaskID}/>)}</div>}
        </section>
      </div>
    </>:<div className="studio-recent-layout">
      <section className="studio-recent-group"><SectionHeading icon={<Clock3 size={17}/>} title="最近使用的工作区资料" count={recentMaterials.length}/>{recentMaterials.length===0?<CompactEmpty icon={<Archive size={20}/>} title="还没有使用记录" detail="把资料加入创作后会显示在这里。"/>:<div className="studio-asset-grid">{recentMaterials.map(item=><WorkspaceMaterialCard key={item.material_ref} item={item} taskID={requestedTaskID}/>)}</div>}</section>
      <section className="studio-recent-group"><SectionHeading icon={<Sparkles size={17}/>} title="最近的创作结果" count={recentResults.length}/>{recentResults.length===0?<CompactEmpty icon={<Library size={20}/>} title="还没有创作结果" detail="已生成的结果会显示在这里。"/>:<div className="studio-asset-grid">{recentResults.map(item=><CreativeResultCard key={item.ref} item={item} taskID={requestedTaskID}/>)}</div>}</section>
    </div>}
  </div>;
}

function AssetCategoryTabs<T extends string>({categories,active,labels,count,onChange}:{categories:T[];active:T;labels:Record<T,string>;count:(id:T)=>number;onChange:(id:T)=>void}){return <div className="studio-asset-kind-tabs" role="tablist" aria-label="资产类型">{categories.map(id=><button type="button" role="tab" aria-selected={active===id} className={active===id?'is-active':''} key={id} onClick={()=>onChange(id)}><MaterialCategoryIcon id={id as MaterialCategory|AssetCategory}/><span>{labels[id]}</span><b>{count(id)}</b></button>)}</div>}

function MaterialCategoryIcon({id}:{id:MaterialCategory|AssetCategory}){if(id==='folder')return <Folder size={16}/>;if(id==='document'||id==='script')return <FileText size={16}/>;if(id==='image')return <ImageIcon size={16}/>;if(id==='video')return <Video size={16}/>;if(id==='audio')return <Music2 size={16}/>;if(id==='table')return <FileSpreadsheet size={16}/>;if(id==='persona')return <Library size={16}/>;if(id==='storyboard')return <Clipboard size={16}/>;return <LayoutGrid size={16}/>}

function FolderAssetCard({folder,onOpen}:{folder:WorkspaceFolderItem;onOpen:()=>void}){return <button type="button" className="studio-asset-card studio-folder-card" onClick={onOpen}><span className="studio-asset-card-preview"><Folder size={34}/></span><span className="studio-asset-card-copy"><strong>{folder.name}</strong><small>{folder.child_count} 项资料</small></span><MoreHorizontal size={16} className="studio-asset-card-menu"/></button>}

function WorkspaceMaterialCard({item,taskID}:{item:WorkspaceMaterialItem;taskID:string}){const id=item.material_ref.replace(/^material:/,'');return <Link className="studio-asset-card" to={`/studio/assets/materials/${encodeURIComponent(id)}${assetTaskQuery(taskID)}`}><span className={`studio-asset-card-preview is-${item.material_kind}`}><MaterialPreview item={item}/></span><span className="studio-asset-card-copy"><strong>{item.title}</strong><small>{workspaceMaterialKindLabel(item.material_kind)} · {formatBytes(item.byte_size)}</small></span><b className="studio-asset-card-state is-material-state">{workspaceMaterialStateLabel(item.processing_state)}</b></Link>}

function CreativeResultCard({item,taskID}:{item:StudioAssetItem;taskID:string}){const id=item.ref.replace(/^result:/,'');return <Link className="studio-asset-card" to={`/studio/assets/results/${encodeURIComponent(item.task_id)}/${encodeURIComponent(id)}${assetTaskQuery(taskID)}`}><span className={`studio-asset-card-preview is-result-${item.result_type}`}><ResultPreview item={item}/></span><span className="studio-asset-card-copy"><strong>{item.title}</strong><small>{assetCategoryLabels[item.result_type]} · {item.project_name}</small></span><b className={`studio-asset-card-state ${item.reusable?'is-reusable':'is-blocked'}`}>{assetUseLabel(item)}</b></Link>}

function MaterialPreview({item}:{item:WorkspaceMaterialItem}){if(item.material_kind==='image')return <img src={workspaceMaterialHref(item)} alt="" loading="lazy"/>;if(item.material_kind==='video')return <><Video size={32}/><i><Play size={12} fill="currentColor"/></i></>;if(item.material_kind==='audio')return <Music2 size={32}/>;if(item.material_kind==='table')return <FileSpreadsheet size={32}/>;if(item.material_kind==='document')return <FileText size={32}/>;return <File size={32}/>}
function ResultPreview({item}:{item:StudioAssetItem}){const image=item.downloads.find(file=>file.media_type.startsWith('image/'));if(item.result_type==='image'&&image)return <img src={image.href} alt="" loading="lazy"/>;if(item.result_type==='video')return <><Video size={32}/><i><Play size={12} fill="currentColor"/></i></>;return <AssetIcon resultType={item.result_type}/>}

export function StudioDeliveriesPage(){
  const [deliveries,setDeliveries]=useState<Awaited<ReturnType<typeof studioApi.deliveries>>>();
  const [loading,setLoading]=useState(true);
  const [error,setError]=useState('');
  const load=useCallback(async()=>{setLoading(true);setError('');try{setDeliveries(await studioApi.deliveries())}catch(value){setError(value instanceof Error?value.message:'交付记录加载失败')}finally{setLoading(false)}},[]);
  useEffect(()=>{void load()},[load]);
  return <div className="studio-view"><PageHeading eyebrow="交付" title="准备好的文件与真实发布记录" detail="交付包表示文件已固定；渠道状态只有在外部回执确认后才会显示为已发布。"/>{error&&<StudioNotice kind="error" onRetry={load}>{error}</StudioNotice>}{loading?<StudioLoading label="正在读取交付记录…"/>:<div className="studio-delivery-layout"><section className="studio-section"><SectionHeading icon={<PackageCheck size={17}/>} title="交付包" count={deliveries?.packages.length||0}/>{!deliveries?.packages.length?<CompactEmpty icon={<PackageCheck size={22}/>} title="还没有交付包" detail="最终成果确认并完成交付准备后，文件会出现在这里。"/>:<div className="studio-package-list">{deliveries.packages.map(pkg=><article key={pkg.id}><header><div><span>{pkg.project_name}</span><strong>{pkg.files.length} 个交付文件</strong><small>{formatDate(pkg.created_at)} · 交付包已准备</small></div><span className="studio-state is-ready">已准备</span></header><div>{pkg.files.map(file=><a key={file.id} href={file.href} download><FileText size={16}/><span><strong>{file.file_name}</strong><small>{file.media_type} · {formatBytes(file.byte_size)}</small></span><Download size={15}/></a>)}</div></article>)}</div>}</section><section className="studio-section"><SectionHeading icon={<ExternalLink size={17}/>} title="渠道状态" count={deliveries?.publications.length||0}/>{!deliveries?.publications.length?<CompactEmpty icon={<ExternalLink size={22}/>} title="当前没有渠道发布意图" detail="下载或生成交付包不会自动标记为已发布。"/>:<div className="studio-publish-list">{deliveries.publications.map(item=><article key={item.id}><CheckCircle2 size={18}/><div><strong>{deliveryDestinationLabel(item.destination)}</strong><span>{statusLabel(item.status)} · {formatDate(item.published_at||item.updated_at)}</span></div></article>)}</div>}</section></div>}</div>;
}

function InspirationStage({taskID,inspirations,experience,canAdd,onChanged}:{taskID:string;inspirations:StudioTaskView['inspirations'];experience?:StudioExperience;canAdd:boolean;onChanged:(view:StudioTaskView)=>void}){
  const [title,setTitle]=useState('');
  const [body,setBody]=useState('');
  const [keepAsProjectReference,setKeepAsProjectReference]=useState(true);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');
  const available=new Set(experience?.available_collection_methods||[]);
  const submit=async(event:FormEvent)=>{event.preventDefault();if(!title.trim()||!body.trim())return;setBusy(true);setError('');try{onChanged(await studioApi.addInspiration(taskID,{title:title.trim(),body:body.trim(),keep_as_project_reference:keepAsProjectReference,idempotency_key:createIdempotencyKey()}));setTitle('');setBody('')}catch(value){setError(value instanceof Error?value.message:'灵感保存失败')}finally{setBusy(false)}};
  return <div className="studio-inspiration"><div className="studio-collection-methods"><CollectionMethod icon={<Globe2 size={18}/>} title="平台搜索" detail="搜索 API 与垂直数据源" available={available.has('platform_search')}/><CollectionMethod icon={<FileSearch size={18}/>} title="受控采集" detail="网页采集与来源留痕" available={available.has('controlled_fetch')}/><CollectionMethod icon={<TerminalSquare size={18}/>} title="本地工具" detail="Codex、Claude Code 或 MCP" available={available.has('local_agent')} href="/docs/clients/codex"/></div>{canAdd&&<form className="studio-inspiration-form" onSubmit={submit}><header><Lightbulb size={18}/><div><strong>人工补充一条灵感</strong><span>灵感会作为当前任务输入保存，不会进入创作结果资产库。</span></div></header><label><span>一句话标题</span><input value={title} onChange={event=>setTitle(event.target.value)} placeholder="例如：真实主理人的一天"/></label><label><span>灵感内容</span><textarea value={body} onChange={event=>setBody(event.target.value)} rows={4} placeholder="写下观察、参考链接、可借鉴的表达，或需要继续验证的问题。"/></label><label className="studio-checkbox"><input type="checkbox" checked={keepAsProjectReference} onChange={event=>setKeepAsProjectReference(event.target.checked)}/><span>保留为项目参考，供后续任务选择</span></label><Button disabled={busy||!title.trim()||!body.trim()}><Plus size={15}/>{busy?'保存中…':'加入当前任务'}</Button>{error&&<StudioNotice kind="error">{error}</StudioNotice>}</form>}<div className="studio-inspiration-list"><SectionHeading icon={<Archive size={16}/>} title="已收集灵感" count={inspirations.length}/>{inspirations.length===0?<CompactEmpty icon={<Lightbulb size={20}/>} title="还没有灵感记录" detail={canAdd?'先手动补充，或从已配置的采集方式导入。':'有权限的团队成员补充后会显示在这里。'}/>:inspirations.map(item=><article key={item.id}><Lightbulb size={16}/><div><strong>{item.title}</strong><p>{item.summary}</p><small>{item.source_label} · {item.saved_as_project_reference?'已保留为项目参考':'仅用于本次任务'} · {formatDate(item.created_at)}</small></div></article>)}</div></div>;
}

function DecisionPanel({decisions,busy,onDecision}:{decisions:StudioDecision[];busy:string;onDecision:(decisionID:string,decision:'approved'|'changes_requested')=>void}){return <div className="studio-gate-panel"><header><ClipboardCheck size={20}/><div><strong>当前结果等待确认</strong><span>确认会固定当前版本；要求修改会把意见送回创作流水线。</span></div></header>{decisions.map(decision=><article key={decision.id}><div><strong>{decision.title}</strong><span>{decision.summary} · {decision.result_count} 项结果</span></div>{decision.can_decide?<div><Button variant="secondary" disabled={Boolean(busy)} onClick={()=>onDecision(decision.id,'changes_requested')}><X size={15}/>需要修改</Button><Button disabled={Boolean(busy)} onClick={()=>onDecision(decision.id,'approved')}><Check size={15}/>确认并继续</Button></div>:<span className="studio-gate-waiting">等待指定确认人处理</span>}</article>)}</div>}

function CurrentStepSummary({task,step}:{task:StudioTaskSummary;step:StudioCustomerStep}){const copy=task.status==='running'?'创作流水线正在处理当前步骤。结果准备好后，会在这里请你确认。':task.status==='blocked'?'当前步骤需要协助，运营人员会看到详细原因并处理。':task.status==='delivered'?'任务已经完成，交付成果已固定。':'当前步骤尚未开始。';return <div className={`studio-step-summary ${customerTaskTone(task.status)}`}><span><Workflow size={20}/></span><div><strong>{task.next_action}</strong><p>{copy}</p><small>{step.title} · {task.status_label}</small></div></div>}
function CustomerProgress({steps}:{steps:StudioCustomerStep[]}){return <ol className="studio-progress" aria-label="创作进度">{steps.map((step,index)=><li key={step.id} className={`is-${step.status}`}><span>{step.status==='completed'?<Check size={14}/>:String(index+1).padStart(2,'0')}</span><div><strong>{step.title}</strong><small>{studioStepStateLabel(step.status)}</small></div></li>)}</ol>}
function CollectionMethod({icon,title,detail,available,href}:{icon:ReactNode;title:string;detail:string;available:boolean;href?:string}){const enabled=available&&Boolean(href);const body=<><span>{icon}</span><div><strong>{title}</strong><small>{detail}</small></div><b>{available?'可用':'待运营配置'}</b>{enabled&&<ChevronRight size={15}/>}</>;return enabled?<Link className="studio-collection-method" to={href||''}>{body}</Link>:<div className={`studio-collection-method ${available?'':'is-disabled'}`} aria-disabled={!available}>{body}</div>}
function MaterialPicker({items,selectedRefs,onChange}:{items:WorkspaceMaterialItem[];selectedRefs:string[];onChange:(refs:string[])=>void}){return <fieldset className="studio-asset-picker"><legend>从我的资产带入 <em>可选</em></legend><p>选择已上传或导入的文档、图片、视频、音频或表格，创建任务时会固定当前文件版本。</p>{items.length===0?<small>当前项目还没有工作区资料。</small>:<div>{items.slice(0,8).map(item=><label key={item.material_ref}><input type="checkbox" checked={selectedRefs.includes(item.material_ref)} onChange={event=>onChange(event.target.checked?[...selectedRefs,item.material_ref]:selectedRefs.filter(ref=>ref!==item.material_ref))}/><span><strong>{item.title}</strong><small>{workspaceMaterialKindLabel(item.material_kind)} · {item.file_name} · {formatBytes(item.byte_size)}</small></span></label>)}</div>}</fieldset>}
function AssetPicker({items,selectedRefs,onChange}:{items:StudioAssetItem[];selectedRefs:string[];onChange:(refs:string[])=>void}){return <fieldset className="studio-asset-picker"><legend>从创作结果带入 <em>可选</em></legend><p>选择已确认的人物原型、剧本、分镜、图片或视频结果，创建后会作为本次任务的正式输入。</p>{items.length===0?<small>当前还没有可复用的创作结果。</small>:<div>{items.slice(0,8).map(item=><label key={item.ref}><input type="checkbox" checked={selectedRefs.includes(item.ref)} onChange={event=>onChange(event.target.checked?[...selectedRefs,item.ref]:selectedRefs.filter(ref=>ref!==item.ref))}/><span><strong>{item.title}</strong><small>{assetCategoryLabels[item.result_type]} · {item.summary} · {item.version}</small></span></label>)}</div>}</fieldset>}
function ResultRow({result,taskID}:{result:StudioTaskView['results'][number];taskID:string}){const icon=result.kind==='delivery_package'?<PackageCheck size={17}/>:result.kind==='approved_result'?<CheckCircle2 size={17}/>:<FileText size={17}/>;const href=result.kind==='delivery_package'?'/studio/deliveries':`/studio/assets/results/${encodeURIComponent(taskID)}/${encodeURIComponent(result.id)}`;return <article><span>{icon}</span><div><small>{resultKindLabel(result.kind)}</small><strong>{result.title}</strong><p>{result.summary} · {formatDate(result.created_at)}</p><Link to={href}>{result.kind==='delivery_package'?'查看交付':'打开独立详情'}<ArrowRight size={12}/></Link>{result.downloads.map(file=><a href={file.href} download key={file.id}><Download size={13}/>{file.file_name}</a>)}</div></article>}
function assetUseLabel(item:StudioAssetItem){if(item.reusable)return item.status==='delivered'?'已交付，可复用':'已确认，可复用';return assetStatusLabels[item.status as AssetStatus]||item.blocked_reason||'暂不可用'}
function AssetIcon({resultType}:{resultType:StudioAssetItem['result_type']}){return resultType==='video'?<Video size={18}/>:resultType==='image'?<ImageIcon size={18}/>:resultType==='storyboard'?<Clipboard size={18}/>:resultType==='script'?<FileCheck2 size={18}/>:<Library size={18}/>}
function AssetRow({item}:{item:StudioAssetItem}){return <article><div><strong>{item.title}</strong><small>{assetCategoryLabels[item.result_type]} · {item.summary} · {item.version}</small></div><span>{item.project_name}</span><b>{assetUseLabel(item)}</b></article>}
function workspaceMaterialKindLabel(kind:WorkspaceMaterialItem['material_kind']){return {document:'文档',image:'图片',video:'视频',audio:'音频',table:'表格',other:'其他文件'}[kind]||'其他文件'}
function workspaceMaterialStateLabel(state:WorkspaceMaterialItem['processing_state']){return {uploading:'上传中',processing:'处理中',ready:'可预览',failed:'处理失败'}[state]||'处理中'}
function workspaceMaterialHref(item:WorkspaceMaterialItem){return `/api/studio/materials/${encodeURIComponent(item.material_ref.replace(/^material:/,''))}/download`}
function assetTaskQuery(taskID:string){return taskID?`?task_id=${encodeURIComponent(taskID)}`:''}
function useTasks(){const [tasks,setTasks]=useState<StudioTaskSummary[]>([]);const [loading,setLoading]=useState(true);const [error,setError]=useState('');const reload=useCallback(async()=>{setLoading(true);setError('');try{setTasks(await studioApi.tasks())}catch(value){setError(value instanceof Error?value.message:'任务加载失败')}finally{setLoading(false)}},[]);useEffect(()=>{void reload()},[reload]);return {tasks,loading,error,reload}}
function PageHeading({eyebrow,title,detail,actions}:{eyebrow:string;title:string;detail:string;actions?:ReactNode}){return <header className="studio-page-heading"><div><span>{eyebrow}</span><h1>{title}</h1><p>{detail}</p></div>{actions&&<div className="studio-page-actions">{actions}</div>}</header>}
function SectionHeading({icon,title,count,action}:{icon:ReactNode;title:string;count?:number;action?:ReactNode}){return <header className="studio-section-heading"><div>{icon}<h2>{title}</h2>{count!==undefined&&<span>{count}</span>}</div>{action}</header>}
function StatusBadge({task}:{task:StudioTaskSummary}){return <span className={`studio-status ${customerTaskTone(task.status)}`}><i/>{task.status_label}</span>}
function TaskRow({task,roomy=false}:{task:StudioTaskSummary;roomy?:boolean}){return <Link className={`studio-task-row ${roomy?'is-roomy':''}`} to={`/studio/tasks/${encodeURIComponent(task.id)}`}><span className="studio-task-icon"><Video size={17}/></span><span className="studio-task-copy"><strong>{task.title}</strong><small>{contentTypeLabel(task.content_type)} · 更新于 {formatDate(task.updated_at)}</small></span><span className="studio-task-next"><small>下一步</small><strong>{task.next_action}</strong></span><StatusBadge task={task}/><ChevronRight size={16}/></Link>}
function StudioLoading({label}:{label:string}){return <div className="studio-loading"><LoaderCircle className="is-spinning" size={18}/>{label}</div>}
function StudioNotice({children,kind='info',onRetry,onClose}:{children:ReactNode;kind?:'info'|'success'|'error';onRetry?:()=>void|Promise<void>;onClose?:()=>void}){return <div className={`studio-notice is-${kind}`}><span>{kind==='success'?<CheckCircle2 size={17}/>:kind==='error'?<AlertCircle size={17}/>:<CircleHelp size={17}/>}</span><div>{children}</div>{onRetry&&<button type="button" onClick={()=>void onRetry()}><RefreshCw size={14}/>重试</button>}{onClose&&<IconButton label="关闭提示" onClick={onClose}><X size={15}/></IconButton>}</div>}
function CompactEmpty({icon,title,detail,action}:{icon:ReactNode;title:string;detail:string;action?:ReactNode}){return <div className="studio-empty"><span>{icon}</span><div><strong>{title}</strong><p>{detail}</p>{action}</div></div>}
function formatDate(value?:string){if(!value)return'未记录';const date=new Date(value);return Number.isNaN(date.getTime())?'未知时间':new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(date)}
function formatBytes(value:number){if(value<1024)return`${value} B`;if(value<1024*1024)return`${(value/1024).toFixed(1)} KB`;return`${(value/(1024*1024)).toFixed(1)} MB`}
function resultKindLabel(kind:string){return {content_revision:'内容版本',approved_result:'已确认结果',delivery_package:'交付包'}[kind]||'创作成果'}
function connectionTitle(status:StudioConnectSession['status']){return {waiting_for_computer:'在 Codex 中继续',connecting:'正在完成连接',confirmation_required:'确认这台电脑',connected:'创作电脑已连接',failed:'连接未完成',expired:'连接已过期',canceled:'连接已取消'}[status]}
function createIdempotencyKey(){return typeof crypto!=='undefined'&&'randomUUID'in crypto?crypto.randomUUID():`${Date.now()}-${Math.random().toString(36).slice(2)}`}
