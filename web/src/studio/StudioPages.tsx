import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  Archive,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleHelp,
  ClipboardCheck,
  Clock3,
  Download,
  ExternalLink,
  FileCheck2,
  FileSearch,
  FileText,
  Globe2,
  Library,
  Lightbulb,
  LoaderCircle,
  PackageCheck,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Search,
  Sparkles,
  TerminalSquare,
  Video,
  WandSparkles,
  Workflow,
  X,
} from 'lucide-react';
import { useCallback, useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { Button, IconButton } from '../components/ui';
import { contentTypeLabel, deliveryDestinationLabel, statusLabel } from '../uiLabels';
import { studioApi } from './studioApi';
import { customerTaskTone, studioStepStateLabel } from './studioData';
import { useStudio } from './StudioContext';
import type {
  StudioAssetCatalog,
  StudioAssetItem,
  StudioCustomerStep,
  StudioDecision,
  StudioExperience,
  StudioProject,
  StudioTaskSummary,
  StudioTaskView,
} from './studioTypes';

type TaskFilter='active'|'attention'|'completed';
type AssetCategory='all'|'source'|'inspiration'|'persona'|'knowledge'|'media'|'approved';

const filterLabels:Record<TaskFilter,string>={active:'进行中',attention:'等待处理',completed:'已完成'};
const assetCategoryLabels:Record<AssetCategory,string>={all:'全部',source:'资料',inspiration:'灵感',persona:'人物',knowledge:'知识',media:'媒体',approved:'已确认成果'};
const attentionStatuses=['waiting_gate','needs_input','blocked'];
const completedStatuses=['delivered','cancelled','canceled'];

export function StudioHomePage(){
  const {bootstrap}=useStudio();
  const navigate=useNavigate();
  const {tasks,loading,error,reload}=useTasks();
  const attention=tasks.filter(task=>attentionStatuses.includes(task.status));
  const recent=tasks.slice(0,4);
  const firstName=bootstrap.session.user.display_name.trim().split(/\s+/)[0]||bootstrap.session.user.display_name;

  return <div className="studio-view studio-home">
    <PageHeading eyebrow="今天" title={`${firstName}，今天想创作什么？`} detail="选择一个创作目标，资料、步骤和确认节点会在同一个任务里展开。"/>
    {error&&<StudioNotice kind="error" onRetry={reload}>{error}</StudioNotice>}
    {bootstrap.experiences.length===0?<CompactEmpty icon={<CircleHelp size={22}/>} title="当前还没有可用的创作流水线" detail="运营人员完成项目、能力和流程发布后，创作入口会自动出现在这里。"/>:bootstrap.experiences.map(experience=>{
      const hasProject=experience.project_ids.length>0;
      return <section className="studio-create-band" aria-labelledby={`experience-${experience.id}`} key={experience.id}>
        <div className="studio-create-copy"><span><WandSparkles size={16}/>已发布创作流水线</span><h2 id={`experience-${experience.id}`}>{experience.name}</h2><p>{experience.description}</p><div>{experience.step_titles.map(title=><b key={title}>{title}</b>)}</div></div>
        <button type="button" className="studio-create-action" disabled={!hasProject||!bootstrap.session.can_create} onClick={()=>navigate(`/studio/tasks/new?experience=${encodeURIComponent(experience.id)}`)}><span><Video size={24}/></span><strong>{!hasProject?'需要先配置项目':bootstrap.session.can_create?'开始创作':'当前角色仅可查看'}</strong><small>{!hasProject?'请联系运营人员完成项目配置':bootstrap.session.can_create?'只需填写这次的业务目标':'需要创作权限时请联系团队管理员'}</small><ArrowRight size={18}/></button>
      </section>;
    })}
    <div className="studio-home-grid">
      <section className="studio-section"><SectionHeading icon={<ClipboardCheck size={17}/>} title="等待你处理" count={attention.length}/>{loading?<StudioLoading label="正在整理待办…"/>:attention.length===0?<CompactEmpty icon={<CheckCircle2 size={21}/>} title="当前没有待确认事项" detail="需要补资料或确认的任务会显示在这里。"/>:<div className="studio-task-stack">{attention.slice(0,4).map(task=><TaskRow key={task.id} task={task}/>)}</div>}</section>
      <section className="studio-section"><SectionHeading icon={<Clock3 size={17}/>} title="最近任务" count={recent.length} action={<Link to="/studio/tasks">查看全部 <ArrowRight size={14}/></Link>}/>{loading?<StudioLoading label="正在读取任务…"/>:recent.length===0?<CompactEmpty icon={<Sparkles size={21}/>} title="还没有创作任务" detail="从上方选择一个创作目标开始。"/>:<div className="studio-task-stack">{recent.map(task=><TaskRow key={task.id} task={task}/>)}</div>}</section>
    </div>
  </div>;
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
  return <div className="studio-view">
    <PageHeading eyebrow="创作任务" title="所有创作任务" detail="按需要处理的状态查看，不必理解底层执行过程。" actions={bootstrap.session.can_create?<Link className="studio-primary-link" to="/studio/tasks/new"><Plus size={16}/>新建任务</Link>:undefined}/>
    {error&&<StudioNotice kind="error" onRetry={reload}>{error}</StudioNotice>}
    <div className="studio-toolbar"><div className="studio-segments" role="tablist" aria-label="任务筛选">{(Object.keys(filterLabels) as TaskFilter[]).map(id=><button type="button" role="tab" aria-selected={filter===id} className={filter===id?'is-active':''} key={id} onClick={()=>setFilter(id)}>{filterLabels[id]}<span>{counts[id]}</span></button>)}</div><label className="studio-search"><Search size={16}/><input value={query} onChange={event=>setQuery(event.target.value)} placeholder="搜索任务" aria-label="搜索任务"/></label></div>
    <section className="studio-section studio-task-list" aria-label={`${filterLabels[filter]}任务`}>{loading?<StudioLoading label="正在读取创作任务…"/>:visible.length===0?<CompactEmpty icon={<FileSearch size={22}/>} title={tasks.length?'没有匹配的任务':'还没有创作任务'} detail={tasks.length?'换一个筛选条件或搜索词。':'从一个明确的创作目标开始。'} action={!tasks.length&&bootstrap.session.can_create?<Link className="studio-secondary-link" to="/studio/tasks/new"><Plus size={15}/>新建任务</Link>:undefined}/>:visible.map(task=><TaskRow key={task.id} task={task} roomy/>)}</section>
  </div>;
}

export function StudioNewTaskPage(){
  const {bootstrap}=useStudio();
  const navigate=useNavigate();
  const [searchParams]=useSearchParams();
  const initialExperience=bootstrap.experiences.find(item=>item.id===searchParams.get('experience'))||bootstrap.experiences[0];
  const [experienceID,setExperienceID]=useState(initialExperience?.id||'');
  const experience=bootstrap.experiences.find(item=>item.id===experienceID);
  const projects=bootstrap.projects.filter(project=>experience?.project_ids.includes(project.id));
  const [projectID,setProjectID]=useState(projects[0]?.id||'');
  const [title,setTitle]=useState('');
  const [goal,setGoal]=useState('');
  const [inspiration,setInspiration]=useState('');
  const [catalog,setCatalog]=useState<StudioAssetCatalog>();
  const [selectedRefs,setSelectedRefs]=useState<string[]>([]);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');
  const project=projects.find(item=>item.id===projectID);
  const canSubmit=Boolean(experience&&project&&title.trim().length>=3&&goal.trim().length>=5);

  useEffect(()=>{
    const nextProjects=bootstrap.projects.filter(item=>bootstrap.experiences.find(value=>value.id===experienceID)?.project_ids.includes(item.id));
    if(!nextProjects.some(item=>item.id===projectID))setProjectID(nextProjects[0]?.id||'');
  },[bootstrap.experiences,bootstrap.projects,experienceID,projectID]);
  useEffect(()=>{setSelectedRefs([]);setCatalog(undefined);if(projectID)void studioApi.assets(projectID).then(setCatalog).catch(value=>setError(value instanceof Error?value.message:'资产加载失败'))},[projectID]);

  const submit=async(event:FormEvent)=>{
    event.preventDefault();
    if(!experience||!project||!canSubmit)return;
    setBusy(true);setError('');
    try{
      const created=await studioApi.createTask({experience_id:experience.id,project_id:project.id,title:title.trim(),goal:goal.trim(),inspiration:inspiration.trim(),asset_refs:selectedRefs,idempotency_key:createIdempotencyKey()});
      navigate(`/studio/tasks/${encodeURIComponent(created.task.id)}`);
    }catch(value){setError(value instanceof Error?value.message:'任务创建失败')}finally{setBusy(false)}
  };

  if(!bootstrap.session.can_create)return <div className="studio-view"><PageHeading eyebrow="新建任务" title="当前账号没有创作权限" detail="你仍可以查看任务、资产和交付结果。"/><CompactEmpty icon={<CircleHelp size={22}/>} title="需要创作权限" detail="请联系团队管理员调整当前账号的角色。" action={<Link className="studio-secondary-link" to="/studio">返回今天</Link>}/></div>;
  if(!experience||!project)return <div className="studio-view"><PageHeading eyebrow="新建任务" title="需要一个已发布的创作流水线" detail="项目归属、内容能力和创作流水线由运营人员维护。"/><CompactEmpty icon={<CircleHelp size={22}/>} title="当前没有可用配置" detail="请联系运营人员完成项目、租户能力和流程发布。"/></div>;

  const reusableAssets=(catalog?.items||[]).filter(item=>item.reusable);
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
        <section className="studio-section studio-results"><SectionHeading icon={<FileCheck2 size={17}/>} title="当前成果" count={view.results.length}/>{view.results.length===0?<CompactEmpty icon={<Workflow size={21}/>} title="成果正在形成" detail="每个需要你判断的版本都会固定保存在这里。"/>:<div className="studio-result-list">{view.results.map(result=><ResultRow key={result.id} result={result}/>)}</div>}</section>
        <section className="studio-section studio-attached-assets"><SectionHeading icon={<Archive size={17}/>} title="本次使用的资产" count={view.attached_assets.length} action={<Link to={`/studio/assets?task_id=${encodeURIComponent(task.id)}`}>加入资产 <Plus size={14}/></Link>}/>{view.attached_assets.length===0?<CompactEmpty icon={<Archive size={20}/>} title="还没有复用已有资产" detail="可以从资产库加入已固定版本的资料、人物和历史成果。"/>:<div>{view.attached_assets.map(item=><AssetRow key={item.ref} item={item}/>)}</div>}</section>
      </main>
      <aside className="studio-task-aside"><span>下一步</span><strong>{task.next_action}</strong><p>{task.status==='waiting_gate'?'你的决定会固定当前版本，并决定流水线下一步。':'系统会在需要补资料或确认时通知你。'}</p><dl><div><dt>当前项目</dt><dd>{task.project.brand_name}</dd></div><div><dt>最近更新</dt><dd>{formatDate(task.updated_at)}</dd></div><div><dt>创作资产</dt><dd>{task.asset_count} 项</dd></div></dl><Link to={`/studio/assets?task_id=${encodeURIComponent(task.id)}`}><Archive size={15}/>从资产库加入</Link></aside>
    </div>
  </div>;
}

export function StudioAssetsPage(){
  const [searchParams]=useSearchParams();
  const requestedTaskID=searchParams.get('task_id')||'';
  const [catalog,setCatalog]=useState<StudioAssetCatalog>();
  const [tasks,setTasks]=useState<StudioTaskSummary[]>([]);
  const [selected,setSelected]=useState<StudioAssetItem>();
  const [targetTaskID,setTargetTaskID]=useState(requestedTaskID);
  const [category,setCategory]=useState<AssetCategory>('all');
  const [query,setQuery]=useState('');
  const [loading,setLoading]=useState(true);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const load=useCallback(async()=>{setLoading(true);setError('');try{const [nextCatalog,nextTasks]=await Promise.all([studioApi.assets(),studioApi.tasks()]);setCatalog(nextCatalog);setTasks(nextTasks);if(requestedTaskID&&nextTasks.some(task=>task.id===requestedTaskID))setTargetTaskID(requestedTaskID)}catch(value){setError(value instanceof Error?value.message:'资产库加载失败')}finally{setLoading(false)}},[requestedTaskID]);
  useEffect(()=>{void load()},[load]);
  const visible=(catalog?.items||[]).filter(item=>(category==='all'||item.category===category)&&(!query.trim()||`${item.title} ${item.summary} ${item.project_name}`.toLowerCase().includes(query.trim().toLowerCase())));
  const targetTask=tasks.find(task=>task.id===targetTaskID);
  const canAttach=Boolean(selected?.reusable&&targetTask&&targetTask.project.id===selected.project_id&&!completedStatuses.includes(targetTask.status));
  const attach=async()=>{if(!selected||!targetTask||!canAttach)return;setBusy(true);setError('');try{await studioApi.attachAssets(targetTask.id,[selected.ref]);setNotice(`“${selected.title}”已加入“${targetTask.title}”。`)}catch(value){setError(value instanceof Error?value.message:'资产加入失败')}finally{setBusy(false)}};

  return <div className="studio-view studio-assets">
    <PageHeading eyebrow="资产库" title="把经过确认的内容继续用起来" detail="每一项资产都保留来源、固定版本和使用状态；不可复用的内容会说明原因。"/>
    {error&&<StudioNotice kind="error" onRetry={load}>{error}</StudioNotice>}{notice&&<StudioNotice kind="success" onClose={()=>setNotice('')}>{notice}</StudioNotice>}
    <div className="studio-toolbar"><div className="studio-segments" role="tablist" aria-label="资产分类">{(Object.keys(assetCategoryLabels) as AssetCategory[]).map(id=><button type="button" role="tab" aria-selected={category===id} className={category===id?'is-active':''} key={id} onClick={()=>setCategory(id)}>{assetCategoryLabels[id]}<span>{catalog?.counts[id]||0}</span></button>)}</div><label className="studio-search"><Search size={16}/><input value={query} onChange={event=>setQuery(event.target.value)} placeholder="搜索资产" aria-label="搜索资产"/></label></div>
    {loading?<StudioLoading label="正在整理资产目录…"/>:<div className="studio-asset-browser-layout"><section className="studio-section studio-asset-list">{visible.length===0?<CompactEmpty icon={<Library size={22}/>} title="没有匹配的资产" detail="换一个分类或搜索词。"/>:visible.map(item=><button type="button" className={selected?.ref===item.ref?'is-selected':''} key={`${item.kind}:${item.project_id}:${item.ref||item.title}`} onClick={()=>setSelected(item)}><span className={`studio-asset-kind is-${item.category}`}><AssetIcon category={item.category}/></span><span><strong>{item.title}</strong><small>{item.summary} · {item.project_name}</small></span><b className={item.reusable?'is-reusable':'is-blocked'}>{item.reusable?'可复用':item.blocked_reason||'不可复用'}</b><ChevronRight size={15}/></button>)}</section><aside className={`studio-asset-detail ${selected?'is-open':''}`}>{selected?<><header><span className={`studio-asset-kind is-${selected.category}`}><AssetIcon category={selected.category}/></span><div><small>{assetCategoryLabels[selected.category as AssetCategory]||'创作资产'}</small><h2>{selected.title}</h2></div><IconButton label="关闭详情" onClick={()=>setSelected(undefined)}><X size={16}/></IconButton></header><p>{selected.summary}</p><dl><div><dt>所属项目</dt><dd>{selected.project_name}</dd></div><div><dt>固定版本</dt><dd>{selected.version}</dd></div><div><dt>当前状态</dt><dd>{selected.reusable?'可以复用':selected.blocked_reason||statusLabel(selected.status)}</dd></div></dl>{selected.reusable&&<div className="studio-asset-attach"><label><span>加入创作任务</span><select value={targetTaskID} onChange={event=>setTargetTaskID(event.target.value)}><option value="">选择一个任务</option>{tasks.filter(task=>task.project.id===selected.project_id&&!completedStatuses.includes(task.status)).map(task=><option value={task.id} key={task.id}>{task.title}</option>)}</select></label><Button disabled={!canAttach||busy} onClick={()=>void attach()}><Plus size={15}/>{busy?'正在加入…':'加入当前任务'}</Button></div>}</>:<CompactEmpty icon={<Archive size={22}/>} title="选择一项资产查看详情" detail="这里会显示固定版本、可用状态和可加入的创作任务。"/>}</aside></div>}
  </div>;
}

export function StudioDeliveriesPage(){
  const [deliveries,setDeliveries]=useState<Awaited<ReturnType<typeof studioApi.deliveries>>>();
  const [loading,setLoading]=useState(true);
  const [error,setError]=useState('');
  const load=useCallback(async()=>{setLoading(true);setError('');try{setDeliveries(await studioApi.deliveries())}catch(value){setError(value instanceof Error?value.message:'交付记录加载失败')}finally{setLoading(false)}},[]);
  useEffect(()=>{void load()},[load]);
  return <div className="studio-view"><PageHeading eyebrow="交付" title="准备好的文件与真实发布记录" detail="交付包表示文件已固定；只有出现发布记录，才表示内容已送达目标位置。"/>{error&&<StudioNotice kind="error" onRetry={load}>{error}</StudioNotice>}{loading?<StudioLoading label="正在读取交付记录…"/>:<div className="studio-delivery-layout"><section className="studio-section"><SectionHeading icon={<PackageCheck size={17}/>} title="交付包" count={deliveries?.packages.length||0}/>{!deliveries?.packages.length?<CompactEmpty icon={<PackageCheck size={22}/>} title="还没有交付包" detail="最终成果确认并完成交付准备后，文件会出现在这里。"/>:<div className="studio-package-list">{deliveries.packages.map(pkg=><article key={pkg.id}><header><div><span>{pkg.project_name}</span><strong>{pkg.files.length} 个交付文件</strong><small>{formatDate(pkg.created_at)} · 交付包已准备</small></div><span className="studio-state is-ready">已准备</span></header><div>{pkg.files.map(file=><a key={file.id} href={file.href} download><FileText size={16}/><span><strong>{file.file_name}</strong><small>{file.media_type} · {formatBytes(file.byte_size)}</small></span><Download size={15}/></a>)}</div></article>)}</div>}</section><section className="studio-section"><SectionHeading icon={<ExternalLink size={17}/>} title="发布记录" count={deliveries?.publications.length||0}/>{!deliveries?.publications.length?<CompactEmpty icon={<ExternalLink size={22}/>} title="当前没有发布记录" detail="下载或生成交付包不会自动标记为已发布。"/>:<div className="studio-publish-list">{deliveries.publications.map(item=><article key={item.id}><CheckCircle2 size={18}/><div><strong>{deliveryDestinationLabel(item.destination)}</strong><span>{statusLabel(item.status)} · {formatDate(item.published_at)}</span></div></article>)}</div>}</section></div>}</div>;
}

function InspirationStage({taskID,inspirations,experience,canAdd,onChanged}:{taskID:string;inspirations:StudioTaskView['inspirations'];experience?:StudioExperience;canAdd:boolean;onChanged:(view:StudioTaskView)=>void}){
  const [title,setTitle]=useState('');
  const [body,setBody]=useState('');
  const [saveForReuse,setSaveForReuse]=useState(true);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');
  const available=new Set(experience?.available_collection_methods||[]);
  const submit=async(event:FormEvent)=>{event.preventDefault();if(!title.trim()||!body.trim())return;setBusy(true);setError('');try{onChanged(await studioApi.addInspiration(taskID,{title:title.trim(),body:body.trim(),save_for_reuse:saveForReuse,idempotency_key:createIdempotencyKey()}));setTitle('');setBody('')}catch(value){setError(value instanceof Error?value.message:'灵感保存失败')}finally{setBusy(false)}};
  return <div className="studio-inspiration"><div className="studio-collection-methods"><CollectionMethod icon={<Globe2 size={18}/>} title="平台搜索" detail="搜索 API 与垂直数据源" available={available.has('platform_search')}/><CollectionMethod icon={<FileSearch size={18}/>} title="受控采集" detail="网页采集与来源留痕" available={available.has('controlled_fetch')}/><CollectionMethod icon={<TerminalSquare size={18}/>} title="本地工具" detail="Codex、Claude Code 或 MCP" available={available.has('local_agent')} href="/docs/clients/codex"/></div>{canAdd&&<form className="studio-inspiration-form" onSubmit={submit}><header><Lightbulb size={18}/><div><strong>人工补充一条灵感</strong><span>链接、观察和人物方向都会作为任务输入保留。</span></div></header><label><span>一句话标题</span><input value={title} onChange={event=>setTitle(event.target.value)} placeholder="例如：真实主理人的一天"/></label><label><span>灵感内容</span><textarea value={body} onChange={event=>setBody(event.target.value)} rows={4} placeholder="写下观察、参考链接、可借鉴的表达，或需要继续验证的问题。"/></label><label className="studio-checkbox"><input type="checkbox" checked={saveForReuse} onChange={event=>setSaveForReuse(event.target.checked)}/><span>保存到资产库，供后续任务复用</span></label><Button disabled={busy||!title.trim()||!body.trim()}><Plus size={15}/>{busy?'保存中…':'加入当前任务'}</Button>{error&&<StudioNotice kind="error">{error}</StudioNotice>}</form>}<div className="studio-inspiration-list"><SectionHeading icon={<Archive size={16}/>} title="已收集灵感" count={inspirations.length}/>{inspirations.length===0?<CompactEmpty icon={<Lightbulb size={20}/>} title="还没有灵感记录" detail={canAdd?'先手动补充，或从已配置的采集方式导入。':'有权限的团队成员补充后会显示在这里。'}/>:inspirations.map(item=><article key={item.id}><Lightbulb size={16}/><div><strong>{item.title}</strong><p>{item.summary}</p><small>{item.source_label} · {item.saved_for_reuse?'已保存复用':'仅用于本次任务'} · {formatDate(item.created_at)}</small></div></article>)}</div></div>;
}

function DecisionPanel({decisions,busy,onDecision}:{decisions:StudioDecision[];busy:string;onDecision:(decisionID:string,decision:'approved'|'changes_requested')=>void}){return <div className="studio-gate-panel"><header><ClipboardCheck size={20}/><div><strong>当前结果等待确认</strong><span>确认会固定当前版本；要求修改会把意见送回创作流水线。</span></div></header>{decisions.map(decision=><article key={decision.id}><div><strong>{decision.title}</strong><span>{decision.summary} · {decision.result_count} 项结果</span></div>{decision.can_decide?<div><Button variant="secondary" disabled={Boolean(busy)} onClick={()=>onDecision(decision.id,'changes_requested')}><X size={15}/>需要修改</Button><Button disabled={Boolean(busy)} onClick={()=>onDecision(decision.id,'approved')}><Check size={15}/>确认并继续</Button></div>:<span className="studio-gate-waiting">等待指定确认人处理</span>}</article>)}</div>}

function CurrentStepSummary({task,step}:{task:StudioTaskSummary;step:StudioCustomerStep}){const copy=task.status==='running'?'创作流水线正在处理当前步骤。结果准备好后，会在这里请你确认。':task.status==='blocked'?'当前步骤需要协助，运营人员会看到详细原因并处理。':task.status==='delivered'?'任务已经完成，交付成果已固定。':'当前步骤尚未开始。';return <div className={`studio-step-summary ${customerTaskTone(task.status)}`}><span><Workflow size={20}/></span><div><strong>{task.next_action}</strong><p>{copy}</p><small>{step.title} · {task.status_label}</small></div></div>}
function CustomerProgress({steps}:{steps:StudioCustomerStep[]}){return <ol className="studio-progress" aria-label="创作进度">{steps.map((step,index)=><li key={step.id} className={`is-${step.status}`}><span>{step.status==='completed'?<Check size={14}/>:String(index+1).padStart(2,'0')}</span><div><strong>{step.title}</strong><small>{studioStepStateLabel(step.status)}</small></div></li>)}</ol>}
function CollectionMethod({icon,title,detail,available,href}:{icon:ReactNode;title:string;detail:string;available:boolean;href?:string}){const enabled=available&&Boolean(href);const body=<><span>{icon}</span><div><strong>{title}</strong><small>{detail}</small></div><b>{available?'可用':'待运营配置'}</b>{enabled&&<ChevronRight size={15}/>}</>;return enabled?<Link className="studio-collection-method" to={href||''}>{body}</Link>:<div className={`studio-collection-method ${available?'':'is-disabled'}`} aria-disabled={!available}>{body}</div>}
function AssetPicker({items,selectedRefs,onChange}:{items:StudioAssetItem[];selectedRefs:string[];onChange:(refs:string[])=>void}){return <fieldset className="studio-asset-picker"><legend>从资产库带入 <em>可选</em></legend><p>选择固定版本的人物、资料或历史成果，创建后会作为本次任务的正式输入。</p>{items.length===0?<small>当前项目还没有可复用资产。</small>:<div>{items.slice(0,8).map(item=><label key={item.ref}><input type="checkbox" checked={selectedRefs.includes(item.ref)} onChange={event=>onChange(event.target.checked?[...selectedRefs,item.ref]:selectedRefs.filter(ref=>ref!==item.ref))}/><span><strong>{item.title}</strong><small>{item.summary} · {item.version}</small></span></label>)}</div>}</fieldset>}
function ResultRow({result}:{result:StudioTaskView['results'][number]}){const icon=result.kind==='delivery_package'?<PackageCheck size={17}/>:result.kind==='approved_result'?<CheckCircle2 size={17}/>:<FileText size={17}/>;return <article><span>{icon}</span><div><small>{resultKindLabel(result.kind)}</small><strong>{result.title}</strong><p>{result.summary} · {formatDate(result.created_at)}</p>{result.downloads.map(file=><a href={file.href} download key={file.id}><Download size={13}/>{file.file_name}</a>)}</div></article>}
function AssetIcon({category}:{category:string}){return category==='media'?<Video size={18}/>:category==='inspiration'?<Lightbulb size={18}/>:category==='approved'?<CheckCircle2 size={18}/>:category==='knowledge'||category==='persona'?<Library size={18}/>:<FileText size={18}/>}
function AssetRow({item}:{item:StudioAssetItem}){return <article><div><strong>{item.title}</strong><small>{item.summary} · {item.version}</small></div><span>{item.project_name}</span><b>{item.reusable?'可复用':item.blocked_reason||'不可复用'}</b></article>}
function useTasks(){const [tasks,setTasks]=useState<StudioTaskSummary[]>([]);const [loading,setLoading]=useState(true);const [error,setError]=useState('');const reload=useCallback(async()=>{setLoading(true);setError('');try{setTasks(await studioApi.tasks())}catch(value){setError(value instanceof Error?value.message:'任务加载失败')}finally{setLoading(false)}},[]);useEffect(()=>{void reload()},[reload]);return {tasks,loading,error,reload}}
function PageHeading({eyebrow,title,detail,actions}:{eyebrow:string;title:string;detail:string;actions?:ReactNode}){return <header className="studio-page-heading"><div><span>{eyebrow}</span><h1>{title}</h1><p>{detail}</p></div>{actions&&<div>{actions}</div>}</header>}
function SectionHeading({icon,title,count,action}:{icon:ReactNode;title:string;count?:number;action?:ReactNode}){return <header className="studio-section-heading"><div>{icon}<h2>{title}</h2>{count!==undefined&&<span>{count}</span>}</div>{action}</header>}
function StatusBadge({task}:{task:StudioTaskSummary}){return <span className={`studio-status ${customerTaskTone(task.status)}`}><i/>{task.status_label}</span>}
function TaskRow({task,roomy=false}:{task:StudioTaskSummary;roomy?:boolean}){return <Link className={`studio-task-row ${roomy?'is-roomy':''}`} to={`/studio/tasks/${encodeURIComponent(task.id)}`}><span className="studio-task-icon"><Video size={17}/></span><span className="studio-task-copy"><strong>{task.title}</strong><small>{contentTypeLabel(task.content_type)} · 更新于 {formatDate(task.updated_at)}</small></span><span className="studio-task-next"><small>下一步</small><strong>{task.next_action}</strong></span><StatusBadge task={task}/><ChevronRight size={16}/></Link>}
function StudioLoading({label}:{label:string}){return <div className="studio-loading"><LoaderCircle className="is-spinning" size={18}/>{label}</div>}
function StudioNotice({children,kind='info',onRetry,onClose}:{children:ReactNode;kind?:'info'|'success'|'error';onRetry?:()=>void|Promise<void>;onClose?:()=>void}){return <div className={`studio-notice is-${kind}`}><span>{kind==='success'?<CheckCircle2 size={17}/>:kind==='error'?<AlertCircle size={17}/>:<CircleHelp size={17}/>}</span><div>{children}</div>{onRetry&&<button type="button" onClick={()=>void onRetry()}><RefreshCw size={14}/>重试</button>}{onClose&&<IconButton label="关闭提示" onClick={onClose}><X size={15}/></IconButton>}</div>}
function CompactEmpty({icon,title,detail,action}:{icon:ReactNode;title:string;detail:string;action?:ReactNode}){return <div className="studio-empty"><span>{icon}</span><div><strong>{title}</strong><p>{detail}</p>{action}</div></div>}
function formatDate(value?:string){if(!value)return'未记录';const date=new Date(value);return Number.isNaN(date.getTime())?'未知时间':new Intl.DateTimeFormat('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(date)}
function formatBytes(value:number){if(value<1024)return`${value} B`;if(value<1024*1024)return`${(value/1024).toFixed(1)} KB`;return`${(value/(1024*1024)).toFixed(1)} MB`}
function resultKindLabel(kind:string){return {content_revision:'内容版本',approved_result:'已确认结果',delivery_package:'交付包'}[kind]||'创作成果'}
function createIdempotencyKey(){return typeof crypto!=='undefined'&&'randomUUID'in crypto?crypto.randomUUID():`${Date.now()}-${Math.random().toString(36).slice(2)}`}
