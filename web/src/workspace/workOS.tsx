import {
  AlertCircle,
  ArrowLeft,
  ArrowRight,
  BookOpen,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleHelp,
  ClipboardCheck,
  Clock3,
  FileCheck2,
  FileInput,
  Filter,
  GitBranch,
  Inbox,
  ListTodo,
  MoreHorizontal,
  PackageCheck,
  PlayCircle,
  Plus,
  Search,
  Settings2,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  UserRound,
  Workflow,
  X
} from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate, useOutletContext, useParams } from 'react-router-dom';
import { api, patch, post, upload } from '../api';
import type { AdminWorkOSView, Environment, EvidenceSpan, GateDefinition, InputItem, KnowledgeObject, KnowledgePack, KnowledgeQueryResult, KnowledgeSnapshot, Project, ProjectSOPView, SOPSummary, SOPVersion, Source, SourceRevision, WorkTask as ApiWorkTask, WorkTaskView } from '../types';
import { Button, Empty, Field, IconButton, Modal, Status } from '../components/ui';
import { AdminEnvironmentPanel as ConfigEnvironmentPanel, AdminSOPPanel as ConfigSOPPanel } from '../admin/WorkOSConfigPanels';
import { useWorkspace } from './context';
import { normalizeAdminWorkOSView, normalizeProjectSOPView, normalizeWorkTaskList, normalizeWorkTaskView } from './workOSData';
import './workOS.css';

type TaskStatus = 'needs_input' | 'ready' | 'running' | 'paused' | 'waiting_gate' | 'blocked' | 'accepted' | 'delivered' | 'cancelled';
type TaskFilter = 'all' | 'mine' | 'waiting_gate' | 'running' | 'delivered';
const taskFilterLabels: Record<TaskFilter, string> = {all: '所有任务', mine: '我的任务', waiting_gate: '待决定', running: '运行中', delivered: '已交付'};

const statusLabel: Record<TaskStatus, string> = {needs_input: '待补输入', ready: '可开始', running: '运行中', paused: '已暂停', waiting_gate: '待决定', blocked: '已阻断', accepted: '已接受', delivered: '已交付', cancelled: '已取消'};
const knowledgeStatusLabel: Record<string, string> = {approved: '已批准', needs_review: '待审核', candidate: '候选', open: '知识缺口', conflicted: '有冲突', blocked: '已阻断', published: '已发布', draft: '草稿', retired: '已退役'};
const inputStatusLabel: Record<string, string> = {untriaged: '待分流', needs_info: '待补信息', routed: '已转负责人', task_created: '已创建任务', task_merged: '已并入任务', project_material: '已归档为资料', archived: '已归档'};

export function taskPath(task: Pick<ApiWorkTask, 'id'|'project_id'>): string {
  return `/projects/${encodeURIComponent(task.project_id)}/tasks/${encodeURIComponent(task.id)}`;
}

function workspaceTaskPath(taskID: string): string {
  return `/workspace/tasks/${encodeURIComponent(taskID)}`;
}

export function WorkOSHomePage() {
  const {dashboard} = useWorkspace();
  const navigate = useNavigate();
  const {openCreateProject} = useOutletContext<{openCreateProject: () => void}>();
  const project = dashboard.projects[0];
  const [tasks, setTasks] = useState<ApiWorkTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  useEffect(() => {
    if (!project) {
      setTasks([]);
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError('');
    api<ApiWorkTask[]>(`/api/bff/tasks?project_id=${encodeURIComponent(project.id)}`).then(value => { if (!cancelled) setTasks(normalizeWorkTaskList(value)); }).catch(value => { if (!cancelled) setError(value instanceof Error ? value.message : '任务加载失败'); }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [project?.id]);
  if (!project) return <div className="workos-page"><PageHeader eyebrow="工作区 / 今天" title="今天" description="从一个真实 Project 开始承接内容工作。" actions={<Button onClick={openCreateProject}><Plus size={16}/>创建 Project</Button>}/><Empty title="还没有 Project" detail="当前账号还没有客户项目或业务目标，创建 Project 后这里会显示真实任务和运行状态。"/></div>;
  const actionable = tasks.filter(task => task.status !== 'delivered').slice(0, 4);
  const runningCount = tasks.filter(task => task.status === 'running').length;
  const waitingCount = tasks.filter(task => task.status === 'waiting_gate' || task.status === 'blocked').length;
  const deliveredCount = tasks.filter(task => task.status === 'delivered').length;
  return <div className="workos-page">
    <PageHeader eyebrow="工作区 / 今天" title="今天" description="只看需要你做决定或推进的工作。" actions={<Button onClick={() => navigate('/workspace/tasks/new')}><Plus size={16}/>新建任务</Button>}/>
    <section className="workos-command-strip"><div className="command-lead"><Sparkles size={18}/><div><strong>下一步从任务开始</strong><span>把本地执行、输入、知识和交付都绑定到一个业务目标。</span></div></div><div className="command-links"><button onClick={() => navigate('/workspace/inbox')}><Inbox size={16}/>输入收集 <b>{tasks.filter(task => task.status === 'needs_input').length}</b></button><button onClick={() => navigate(`/projects/${encodeURIComponent(project.id)}/knowledge`)}><BookOpen size={16}/>知识库 <b>{project.knowledge_ready || 0}</b></button></div></section>
    <section className="workos-metrics" aria-label="工作区状态"><Metric icon={ListTodo} label="待处理任务" value={String(actionable.length)} detail="真实 WorkTask" tone="ink"/><Metric icon={PlayCircle} label="本地运行中" value={String(runningCount)} detail="服务端当前状态" tone="production"/><Metric icon={ShieldAlert} label="待决定 / 阻断" value={String(waitingCount)} detail="按配置产生的 Gate" tone="review"/><Metric icon={PackageCheck} label="已交付任务" value={String(deliveredCount)} detail="当前 Project 累计" tone="success"/></section>
    <div className="workos-home-grid">
      <section className="workos-section"><SectionHeader kicker="任务队列" title="现在该做什么" action={<button className="text-action" onClick={() => navigate('/workspace/tasks')}>查看全部 <ArrowRight size={14}/></button>}/>{error && <div className="workos-notice is-error"><AlertCircle size={16}/>{error}</div>}{loading ? <div className="workos-loading">正在读取真实任务...</div> : <div className="task-queue">{actionable.length === 0 ? <Empty title="还没有待处理任务" detail="创建第一个业务任务后，下一动作会显示在这里。" action={<Button onClick={() => navigate('/workspace/tasks/new')}><Plus size={15}/>新建任务</Button>}/> : actionable.map(task => <TaskRow key={task.id} task={task} onClick={() => navigate(`/workspace/tasks/${task.id}`)}/>)}</div>}</section>
      <section className="workos-section"><SectionHeader kicker="输入收集" title="还没有分流的输入" action={<button className="text-action" onClick={() => navigate('/workspace/inbox')}>打开收件箱 <ArrowRight size={14}/></button>}/><div className="inbox-list"><Empty title="暂时没有待分流输入" detail="本地客户端确认导入后，Brief、资料或 Evidence 候选会出现在这里。" action={<Button variant="secondary" onClick={() => navigate('/workspace/inbox')}>查看输入收集</Button>}/></div></section>
    </div>
    <section className="workos-section workos-project-section"><SectionHeader kicker="项目" title={project.brand_name + ' · ' + project.product_name} action={<button className="text-action" onClick={() => navigate(`/projects/${project.id}/tasks`)}>进入项目 <ArrowRight size={14}/></button>}/><div className="project-context-line"><div className="project-avatar">{project.brand_name.slice(0, 1)}</div><div><strong>{project.stage_objective || '内容生产与治理'}</strong><span>{project.connected_devices || 0} 个本地 Workspace 在线 · 知识对象 {project.knowledge_ready || 0} 个</span></div><Status value={project.status}/></div></section>
  </div>;
}

export function WorkOSTaskListPage({projectID: explicitProjectID}: {projectID?: string} = {}) {
  const {dashboard, session} = useWorkspace();
  const location = useLocation();
  const {projectID: routeProjectID} = useParams();
  const projectID = explicitProjectID ?? routeProjectID;
  const project = dashboard.projects.find(item => item.id === projectID) || dashboard.projects[0];
  const routerNavigate = useNavigate();
  const navigate = (path: string) => {
    if (path === '/workspace/tasks/new' && projectID && project) {
      routerNavigate(`/projects/${encodeURIComponent(project.id)}/tasks/new`);
      return;
    }
    routerNavigate(path);
  };
  const isMine = location.pathname.endsWith('/my-tasks');
  const [filter, setFilter] = useState<TaskFilter>(isMine ? 'mine' : 'all');
  const [filterOpen, setFilterOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [tasks, setTasks] = useState<ApiWorkTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  useEffect(() => { setFilter(isMine ? 'mine' : 'all'); }, [isMine]);
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError('');
    const path = project ? `/api/bff/tasks?project_id=${encodeURIComponent(project.id)}` : '/api/bff/tasks';
    api<ApiWorkTask[]>(path).then(value => { if (!cancelled) setTasks(normalizeWorkTaskList(value)); }).catch(value => { if (!cancelled) setError(value instanceof Error ? value.message : '任务加载失败'); }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [project?.id]);
  const filteredTasks = useMemo(() => tasks.filter(task => {
    const matchesFilter = filter === 'all' || (filter === 'mine' ? task.assignee_user_id === session.user.id : filter === 'waiting_gate' ? task.status === 'waiting_gate' || task.status === 'blocked' : filter === 'running' ? task.status === 'running' : task.status === 'delivered');
    return matchesFilter && (query.trim() === '' || `${task.title}${task.sop_id}${task.current_stage_id}`.toLowerCase().includes(query.trim().toLowerCase()));
  }), [filter, tasks, query, session.user.id]);
  const countFor = (id: TaskFilter) => id === 'all' ? tasks.length : tasks.filter(task => id === 'mine' ? task.assignee_user_id === session.user.id : id === 'waiting_gate' ? task.status === 'waiting_gate' || task.status === 'blocked' : task.status === id).length;
  if (!project) return <div className="workos-page"><PageHeader eyebrow="工作区 / 任务" title="任务" description="先创建一个 Project，再开始承接内容工作。"/><Empty title="还没有 Project" detail="任务必须绑定一个真实 Project，创建后这里会显示任务队列。"/></div>;
  return <div className="workos-page">
    <PageHeader eyebrow={projectID ? `项目 / ${project.brand_name}` : `工作区 / ${isMine ? '我的任务' : '任务中心'}`} title={isMine ? '我的任务' : projectID ? '项目任务' : '任务中心'} description={projectID ? '围绕当前 Project 的业务任务和运行状态。' : isMine ? '只显示由你负责或参与的任务。' : '按下一动作管理所有内容工作。'} actions={<Button onClick={() => navigate('/workspace/tasks/new')}><Plus size={16}/>新建任务</Button>}/>
    <div className="workos-tabs" role="tablist">{([['all', '所有任务'], ['mine', '我的任务'], ['waiting_gate', '待决定'], ['running', '运行中'], ['delivered', '已交付']] as const).map(([id, label]) => <button key={id} className={filter === id ? 'is-active' : ''} onClick={() => setFilter(id)} role="tab" aria-selected={filter === id}>{label}<b>{countFor(id)}</b></button>)}</div>
    <div className="workos-toolbar"><label className="search-field"><Search size={16}/><input value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索任务、SOP 或 Stage" aria-label="搜索任务"/></label><div className="filter-control"><button className={`filter-button ${filter !== 'all' ? 'is-active' : ''}`} aria-expanded={filterOpen} aria-haspopup="menu" onClick={() => setFilterOpen(value => !value)}><Filter size={15}/>{filter === 'all' ? '筛选' : taskFilterLabels[filter]} <ChevronRight size={14}/></button>{filterOpen && <div className="filter-menu" role="menu"><strong>按状态筛选</strong>{(Object.entries(taskFilterLabels) as [TaskFilter, string][]).map(([id, label]) => <button key={id} role="menuitemradio" aria-checked={filter === id} className={filter === id ? 'is-selected' : ''} onClick={() => {setFilter(id); setFilterOpen(false);}}>{label}<span>{countFor(id)}</span></button>)}</div>}</div><span className="toolbar-count">{filteredTasks.length} / {tasks.length} 项</span></div>
    {error && <div className="workos-notice is-error"><AlertCircle size={16}/>{error}</div>}
    <section className="workos-section task-table-section">{loading ? <div className="workos-loading">正在读取任务...</div> : <><div className="task-table-head"><span>Task</span><span>SOP / Stage</span><span>状态</span><span>负责人 / 执行方式</span><span>下一动作</span><span>更新时间</span><span aria-hidden="true"></span></div>{filteredTasks.length === 0 ? <Empty title={tasks.length === 0 ? '还没有任务' : '没有匹配的任务'} detail={tasks.length === 0 ? '从一个真实业务目标开始创建任务，任务会固定当前 Project 的 SOP 版本。' : '换一个筛选条件，或创建新的内容任务。'} action={<Button onClick={() => navigate('/workspace/tasks/new')}><Plus size={15}/>新建任务</Button>}/> : filteredTasks.map(task => <button className="task-table-row" key={task.id} onClick={() => navigate(`${projectID ? `/projects/${project.id}/tasks/${task.id}` : `/workspace/tasks/${task.id}`}`)}><div className="task-title-cell"><span className={`object-mark is-${task.status === 'delivered' ? 'success' : task.status === 'blocked' ? 'review' : 'production'}`}><ListTodo size={15}/></span><span><strong>{task.title}</strong><small>{task.content_type || '内容任务'} · {project.brand_name}</small></span></div><div><strong>{task.sop_id} <small>v{task.sop_version}</small></strong><small className="stage-text"><Workflow size={13}/>{task.current_stage_id || '未开始'}</small></div><StatusText value={task.status}/><div><strong>{task.assignee_user_id || '未分派'}</strong><small>{task.environment_id}</small></div><div className="next-action-cell"><span>{task.next_action}</span><ArrowRight size={14}/></div><time>{formatDateTime(task.updated_at)}</time><MoreHorizontal size={16}/></button>)}</>}</section>
  </div>;
}

export function WorkOSInboxPage() {
  const {dashboard} = useWorkspace();
  const navigate = useNavigate();
  const project = dashboard.projects[0];
  const [items, setItems] = useState<InputItem[]>([]);
  const [status, setStatus] = useState('untriaged');
  const [selected, setSelected] = useState<InputItem>();
  const [missingFields, setMissingFields] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const load = () => {
    setLoading(true);
    setError('');
    const query = status ? `?status=${encodeURIComponent(status)}` : '';
    return api<InputItem[]>(`/api/bff/input-items${query}`).then(setItems).catch(value => setError(value instanceof Error ? value.message : '输入收集加载失败')).finally(() => setLoading(false));
  };
  useEffect(() => { void load(); }, [status]);
  const triage = async (item: InputItem, action: string, extra: Record<string, unknown> = {}) => {
    if (action === 'create_task' && !project?.id && !item.project_id) {
      setError('创建任务前请先选择或创建 Project');
      return;
    }
    setBusy(item.id);
    setError('');
    try {
      await post<InputItem>(`/api/bff/input-items/${encodeURIComponent(item.id)}/triage`, {action, expected_version: item.row_version, project_id: item.project_id || project?.id || '', content_type: 'video_script', ...extra});
      await load();
    } catch (value) {
      setError(value instanceof Error ? value.message : '输入分流失败');
    } finally {
      setBusy('');
    }
  };
  const openMissing = (item: InputItem) => { setSelected(item); setMissingFields(item.missing_fields.join('\n')); };
  const submitMissing = async () => {
    if (!selected) return;
    const fields = missingFields.split(/[\n,，]/).map(value => value.trim()).filter(Boolean);
    if (!fields.length) {
      setError('请填写至少一个缺少的信息');
      return;
    }
    await triage(selected, 'mark_missing', {missing_fields: fields});
    setSelected(undefined);
  };
  const sourceLabel: Record<string, string> = {brief: 'Brief', workspace_file: 'Workspace 文件', comment: '评论', external_request: '外部需求', trigger: '触发事件', conversation_bundle: '对话摘要', other: '其他'};
  const tabs = [['untriaged', '待分流'], ['needs_info', '待补信息'], ['routed', '已转负责人'], ['task_created', '已进任务'], ['project_material', '项目资料'], ['archived', '已归档'], ['', '全部']] as const;
  return <div className="workos-page"><PageHeader eyebrow="工作区 / 输入收集" title="输入收集" description="先分流，再进入任务。这里不会自动把每一轮本地对话上传到云端。" actions={<Button onClick={() => navigate('/workspace/tasks/new')}><Plus size={16}/>新建任务</Button>}/>{error && <div className="workos-notice is-error"><AlertCircle size={16}/><span>{error}</span><IconButton label="关闭错误" onClick={() => setError('')}><X size={15}/></IconButton></div>}<div className="workos-tabs" role="tablist">{tabs.map(([id, label]) => <button key={label} className={status === id ? 'is-active' : ''} onClick={() => setStatus(id)} role="tab" aria-selected={status === id}>{label}</button>)}</div><section className="workos-section inbox-section"><SectionHeader kicker="输入事实" title={`${items.length} 条记录`} action={<span className="section-count">来源由本地 Adapter 解析，云端只保存确认后的摘要或候选</span>}/>{loading ? <div className="workos-loading">正在读取输入收集...</div> : items.length === 0 ? <Empty title={status ? '当前没有输入记录' : '还没有输入记录'} detail="Codex、Claude Code 或 Workspace Adapter 在本地选择范围、脱敏并确认后，输入才会出现在这里。"/> : <div className="input-table">{items.map(item => <article key={item.id}><span className="object-mark is-source"><FileInput size={15}/></span><div><strong>{item.title}</strong><span>{item.summary || item.body || '没有摘要'} · {sourceLabel[item.source_type] || item.source_type}</span></div><span className="input-type">{item.project_id ? '已绑定 Project' : '租户输入'}</span><StatusText value={item.status}/><div className="input-actions">{item.status === 'untriaged' && <><Button variant="secondary" disabled={busy === item.id} onClick={() => void triage(item, 'create_task')}><ListTodo size={14}/>创建任务</Button><Button variant="secondary" disabled={busy === item.id} onClick={() => void triage(item, 'archive_project')}><BookOpen size={14}/>归档资料</Button><Button variant="ghost" disabled={busy === item.id} onClick={() => openMissing(item)}><AlertCircle size={14}/>标记缺口</Button></>}{item.status === 'needs_info' && <Button variant="ghost" disabled={busy === item.id} onClick={() => openMissing(item)}><AlertCircle size={14}/>更新缺口</Button>}{item.target_task_id && <Button variant="ghost" onClick={() => navigate(`/workspace/tasks/${item.target_task_id}`)}><ArrowRight size={14}/>打开任务</Button>}{!['archived', 'project_material', 'task_created', 'task_merged'].includes(item.status) && <Button variant="ghost" disabled={busy === item.id} onClick={() => void triage(item, 'archive')}><X size={14}/>归档</Button>}</div></article>)}</div>}</section><section className="workos-section inbox-guidance"><div><GitBranch size={18}/><strong>输入不是任务，也不是逐轮聊天同步</strong><p>客户端负责识别自己的对话格式、选择范围、脱敏并生成统一 Bundle；这里负责把 Brief、资料、评论或 Evidence 候选分流到任务、Project 资料或补料动作。</p></div><button className="text-action" onClick={() => navigate('/docs')}>查看接入说明 <ArrowRight size={14}/></button></section>{selected && <Modal title="标记缺少信息" onClose={() => setSelected(undefined)}><div className="admin-modal-form"><Field label="缺口（每行一项）" hint="缺口会成为下一步补料动作，不会修改原始输入。"><textarea autoFocus rows={6} value={missingFields} onChange={event => setMissingFields(event.target.value)} placeholder="例如：当前产品规格\n有效素材授权"/></Field><div className="modal-actions"><Button variant="secondary" onClick={() => setSelected(undefined)}>取消</Button><Button onClick={() => void submitMissing()} disabled={busy === selected.id}><Check size={15}/>保存缺口</Button></div></div></Modal>}</div>;
}

export function WorkOSNewTaskPage({projectID: explicitProjectID}: {projectID?: string} = {}) {
  const {dashboard} = useWorkspace();
  const navigate = useNavigate();
  const {projectID: routeProjectID} = useParams();
  const projectID = explicitProjectID ?? routeProjectID;
  const project = dashboard.projects.find(item => item.id === projectID) || dashboard.projects[0];
  const [title, setTitle] = useState('');
  const [contentType, setContentType] = useState('video_script');
  const [input, setInput] = useState('');
  const [sopView, setSopView] = useState<ProjectSOPView>();
  const [created, setCreated] = useState<WorkTaskView>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    if (!project) return;
    api<ProjectSOPView>(`/api/bff/projects/${encodeURIComponent(project.id)}/sop`).then(value => setSopView(normalizeProjectSOPView(value))).catch(value => setError(value instanceof Error ? value.message : '项目 SOP 加载失败'));
  }, [project?.id]);
  const canCreate = title.trim().length > 2;
  const submit = async () => {
    if (!project || !canCreate) return;
    setBusy(true);
    setError('');
    try {
      const value = await post<WorkTaskView>('/api/bff/tasks', {project_id: project.id, title: title.trim(), content_type: contentType, input_refs: input.trim() ? [input.trim()] : [], requested_output: {content_count: 1}, priority: 'normal', risk_profile: 'low'});
      setCreated(normalizeWorkTaskView(value));
    } catch (value) {
      setError(value instanceof Error ? value.message : '任务创建失败');
    } finally {
      setBusy(false);
    }
  };
  if (!project) return <div className="workos-page"><PageHeader eyebrow="工作区 / 新建任务" title="新建任务" description="任务必须绑定一个真实 Project。"/><Empty title="还没有 Project" detail="先创建一个 Project，再把业务目标交给 SOP。"/></div>;
  return <div className="workos-page workos-form-page"><button className="back-link" onClick={() => navigate(-1)}><ArrowLeft size={15}/>返回</button><PageHeader eyebrow="工作区 / 新建任务" title="新建任务" description="只填写业务目标，执行细节由已发布 SOP 承接。"/><div className="new-task-layout"><section className="workos-section new-task-main"><div className="form-progress"><span className="is-current">1 <b>目标</b></span><i></i><span>2 <b>输入</b></span><i></i><span>3 <b>确认</b></span></div><Field label="这次要完成什么？"><textarea value={title} onChange={event => setTitle(event.target.value)} placeholder="例如：为夏季新品生成 3 条短视频脚本" rows={3}/></Field><div className="form-grid"><Field label="Project"><select><option>{project.brand_name} · {project.product_name}</option></select></Field><Field label="内容类型"><select value={contentType} onChange={event => setContentType(event.target.value)}><option value="video_script">短视频脚本</option><option value="wechat_article">公众号文章</option></select></Field><Field label="使用 SOP"><select disabled><option>{sopView ? `${sopView.sop.name} · v${sopView.sop.version}` : '正在读取项目 SOP...'}</option></select></Field><Field label="执行环境"><input value={sopView?.binding.environment_id || '正在读取 Environment...'} readOnly/></Field></div><Field label="补充输入（可选）"><textarea value={input} onChange={event => setInput(event.target.value)} placeholder="Brief、目标受众、截止时间或需要注意的边界" rows={4}/></Field><div className="new-task-actions"><Button variant="secondary" onClick={() => navigate(-1)}>取消</Button><Button disabled={!canCreate || busy || !sopView} onClick={submit}><Plus size={15}/>{busy ? '创建中...' : '创建任务'}</Button></div>{error && <div className="workos-notice is-error"><AlertCircle size={16}/>{error}</div>}{created && <div className="created-task-message"><CheckCircle2 size={17}/><div><strong>任务已创建</strong><span>已固定 SOP v{created.task.sop_version} 和 digest，下一步：{created.task.next_action}。</span></div><Button variant="secondary" onClick={() => navigate(`/workspace/tasks/${created.task.id}`)}>打开任务</Button></div>}</section><aside className="new-task-aside"><div className="aside-label">创建前会检查</div><CheckLine label="Project 可访问且未归档"/><CheckLine label="SOP 已发布且适用当前内容类型"/><CheckLine label="Environment 有可用的本地执行方式"/><CheckLine label="必填输入和输出 Schema 完整"/><div className="aside-note"><Settings2 size={16}/><span>模型、Prompt、本机目录和 CLI 命令不在这里配置，统一由 SOP 和管理后台托底。</span></div></aside></div></div>;
}

export function WorkOSTaskDetailPage({projectID: explicitProjectID}: {projectID?: string} = {}) {
  const {taskID, projectID: routeProjectID} = useParams();
  const projectID = explicitProjectID ?? routeProjectID;
  const navigate = useNavigate();
  const [view, setView] = useState<WorkTaskView>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [actionError, setActionError] = useState('');
  const [busy, setBusy] = useState(false);
  const [showReport, setShowReport] = useState(false);
  const [stageOutput, setStageOutput] = useState('');
  const [revisionContent, setRevisionContent] = useState('{\n  "title": "",\n  "scenes": []\n}');
  const [showRevision, setShowRevision] = useState(false);
  const [deliveryDestination, setDeliveryDestination] = useState('workspace');
  const [showDelivery, setShowDelivery] = useState(false);
  const reload = async () => {
    if (!taskID) return;
    const value = await api<WorkTaskView>(`/api/bff/tasks/${encodeURIComponent(taskID)}`);
    setView(normalizeWorkTaskView(value));
  };
  useEffect(() => {
    if (!taskID) return;
    setLoading(true);
    setError('');
    api<WorkTaskView>(`/api/bff/tasks/${encodeURIComponent(taskID)}`).then(value => {
      const normalized = normalizeWorkTaskView(value);
      setView(normalized);
      if (!projectID && normalized.project?.id) {
        navigate(taskPath(normalized.task), {replace: true});
      }
    }).catch(value => setError(value instanceof Error ? value.message : '任务加载失败')).finally(() => setLoading(false));
  }, [navigate, projectID, taskID]);
  if (loading) return <div className="workos-page"><div className="workos-loading">正在读取任务...</div></div>;
  if (error || !view) return <div className="workos-page"><Empty title="任务不可用" detail={error || '任务不存在或已被移除。'} action={<Button onClick={() => navigate(projectID ? `/projects/${projectID}/tasks` : '/workspace/tasks')}><ArrowLeft size={15}/>返回任务列表</Button>}/></div>;
  const task = view.task;
  const gate = view.sop.gates.find(candidate => view.sop.stages.some(stage => stage.stage_id === task.current_stage_id && stage.gate_ids.includes(candidate.gate_id)));
  const activeStageRun = view.stage_runs.find(run => run.stage_id === task.current_stage_id);
  const primaryAction = task.status === 'waiting_gate' ? '查看 Gate' : task.status === 'running' ? '上报 Stage' : task.status === 'needs_input' ? '补充输入' : task.status === 'blocked' ? '重试当前 Stage' : task.status === 'accepted' ? '提交 Revision' : task.status === 'delivered' ? '查看交付' : task.status === 'paused' ? '恢复执行' : '开始当前 Stage';
  const action = async (name: string) => {
    if (!taskID) return;
    setBusy(true);
    setNotice('');
    try {
      const next = await post<WorkTaskView>(`/api/bff/tasks/${encodeURIComponent(taskID)}/actions`, {action: name});
      setView(normalizeWorkTaskView(next));
      if (name === 'start' || name === 'resume') setShowReport(true);
      setNotice(name === 'cancel' ? '任务已取消。' : '任务状态已更新。');
    } catch (value) { setNotice(value instanceof Error ? value.message : '任务动作失败'); }
    finally { setBusy(false); }
  };
  const report = async () => {
    if (!taskID || !activeStageRun) return;
    const outputRefs = stageOutput.split('\n').map(value => value.trim()).filter(Boolean);
    setBusy(true);
    try {
      const next = await post<WorkTaskView>(`/api/bff/tasks/${encodeURIComponent(taskID)}/stages/${encodeURIComponent(activeStageRun.stage_id)}/report`, {stage_run_id: activeStageRun.id, stage_id: activeStageRun.stage_id, status: 'completed', output_refs: outputRefs, checks: {passed: true}});
      setView(normalizeWorkTaskView(next));
      setStageOutput('');
      setShowReport(false);
      setNotice('Stage 结果已记录，下一动作已重新计算。');
    } catch (value) { setNotice(value instanceof Error ? value.message : 'Stage 上报失败'); }
    finally { setBusy(false); }
  };
  const decide = async (gateID: string, decision: string) => {
    if (!taskID) return;
    setBusy(true);
    try {
      const next = await post<WorkTaskView>(`/api/bff/tasks/${encodeURIComponent(taskID)}/gates/${encodeURIComponent(gateID)}/decide`, {decision, reason: decision === 'approved' ? '工作台确认通过' : '需要补充或修改'});
      setView(normalizeWorkTaskView(next));
      setNotice(decision === 'approved' ? 'Gate 已通过。' : 'Gate 已退回，任务进入阻断恢复路径。');
    } catch (value) { setNotice(value instanceof Error ? value.message : 'Gate 决定失败'); }
    finally { setBusy(false); }
  };
  const submitRevision = async () => {
    if (!taskID) return;
    let content: unknown;
    try { content = JSON.parse(revisionContent); } catch { setNotice('Revision 内容必须是有效 JSON。'); return; }
    setBusy(true);
    try {
      const revision = await post<{id:string}>('/api/bff/tasks/' + encodeURIComponent(taskID) + '/revisions', {content_type: task.content_type, content});
      await reload();
      setShowRevision(false);
      setNotice(`Revision ${revision.id.slice(0, 8)} 已提交。`);
    } catch (value) { setNotice(value instanceof Error ? value.message : 'Revision 提交失败'); }
    finally { setBusy(false); }
  };
  const deliver = async () => {
    if (!taskID) return;
    const revision = [...view.revisions].reverse().find(value => value.status === 'accepted');
    if (!revision) { setNotice('当前没有可交付的已接受 Revision。'); return; }
    setBusy(true);
    try {
      await post(`/api/bff/tasks/${encodeURIComponent(taskID)}/deliveries`, {revision_id: revision.id, destination: deliveryDestination, deliver: true});
      await reload();
      setShowDelivery(false);
      setNotice('交付已记录。');
    } catch (value) { setNotice(value instanceof Error ? value.message : '交付失败'); }
    finally { setBusy(false); }
  };
  const headerAction = task.status === 'waiting_gate' ? () => document.getElementById('task-gates')?.scrollIntoView({behavior: 'smooth'}) : task.status === 'running' ? () => setShowReport(true) : task.status === 'accepted' ? () => setShowRevision(true) : task.status === 'delivered' ? () => document.getElementById('task-deliveries')?.scrollIntoView({behavior: 'smooth'}) : task.status === 'blocked' ? () => action('retry') : task.status === 'paused' ? () => action('resume') : () => action('start');
  return <div className="workos-page task-detail-page"><button className="back-link" onClick={() => navigate(projectID ? `/projects/${projectID}/tasks` : '/workspace/tasks')}><ArrowLeft size={15}/>返回任务列表</button><header className="task-detail-header"><div><span className="eyebrow">{view.project.brand_name} / Task</span><h1>{task.title}</h1><p>{task.content_type || '内容任务'} · {view.sop.name} v{view.sop.version} · 更新于 {formatDateTime(task.updated_at)}</p></div><div className="task-header-actions"><StatusText value={task.status}/><Button disabled={busy || task.status === 'cancelled'} onClick={headerAction}>{primaryAction}<ArrowRight size={15}/></Button></div></header>{notice && <div className="workos-notice is-info"><CircleHelp size={16}/><span>{notice}</span><IconButton label="关闭提示" onClick={() => setNotice('')}><X size={15}/></IconButton></div>}<div className="task-detail-layout"><main><section className="workos-section task-goal"><SectionHeader kicker="目标与输出" title="把业务目标推进到可交付结果"/><p>{task.intent || '任务目标已记录，输入和输出由当前 SOP 的 Stage Schema 约束。'}</p><div className="detail-tags"><span><FileInput size={14}/>输入 {task.input_refs.length} 项</span><span><GitBranch size={14}/>SOP digest {task.sop_digest.slice(0, 18)}…</span><span><PackageCheck size={14}/>{Object.keys(task.requested_output || {}).length} 项输出要求</span></div></section><section className="workos-section task-stage-section"><SectionHeader kicker="当前 Stage" title={view.sop.stages.find(stage => stage.stage_id === task.current_stage_id)?.name || task.current_stage_id || '尚未开始'} action={<span className="stage-progress">{view.stage_runs.filter(run => run.status === 'completed').length} / {view.sop.stages.length}</span>}/><div className="stage-timeline">{view.sop.stages.map((stage, index) => {const run = view.stage_runs.find(candidate => candidate.stage_id === stage.stage_id); const done = run?.status === 'completed'; const current = stage.stage_id === task.current_stage_id; return <div key={stage.stage_id} className={`timeline-step ${done ? 'is-done' : current ? 'is-current' : ''}`}><span>{done ? <Check size={13}/> : index + 1}</span><div><strong>{stage.name}</strong><small>{done ? '已完成' : current ? run?.status || '当前' : '未开始'}</small></div></div>;})}</div><div className="stage-action"><div><strong>唯一下一动作</strong><span>{task.next_action}</span></div><div className="task-action-group">{task.status === 'running' && <Button variant="secondary" onClick={() => setShowReport(value => !value)}><FileCheck2 size={15}/>上报 Stage</Button>}{task.status === 'paused' && <Button variant="secondary" onClick={() => action('resume')}><PlayCircle size={15}/>恢复</Button>}{task.status === 'blocked' && <Button variant="secondary" onClick={() => action('retry')}><ArrowRight size={15}/>重试</Button>}{['ready', 'needs_input'].includes(task.status) && <Button variant="secondary" onClick={() => action('start')}><PlayCircle size={15}/>开始</Button>}{task.status === 'waiting_gate' && <Button variant="secondary" onClick={() => document.getElementById('task-gates')?.scrollIntoView({behavior: 'smooth'})}><ClipboardCheck size={15}/>处理 Gate</Button>}{task.status === 'accepted' && <Button variant="secondary" onClick={() => setShowRevision(true)}><FileCheck2 size={15}/>提交 Revision</Button>}{task.status !== 'cancelled' && task.status !== 'delivered' && <Button variant="secondary" onClick={() => action('cancel')}><X size={15}/>取消任务</Button>}</div></div>{showReport && task.status === 'running' && <div className="task-inline-editor"><Field label="Stage 输出引用（每行一条）"><textarea value={stageOutput} onChange={event => setStageOutput(event.target.value)} rows={3} placeholder="local://workspace/output/script.json"/></Field><div className="new-task-actions"><Button onClick={report} disabled={busy || !activeStageRun}><Check size={15}/>确认上报</Button></div></div>}</section><section className="workos-section"><SectionHeader kicker="输入与执行记录" title="本次任务的可追溯事实"/><div className="evidence-list">{task.input_refs.length === 0 ? <Empty title="尚未绑定输入" detail="本地客户端确认后的 Brief、知识快照或来源会出现在这里。"/> : task.input_refs.map(ref => <EvidenceRow key={ref} label={ref} detail="任务输入引用" status="已绑定" icon={FileInput}/>)}{view.stage_runs.map(run => <EvidenceRow key={run.id} label={run.stage_id} detail={`${run.execution_mode} · ${run.status}`} status="Stage Run" icon={Workflow}/>)}</div></section><section className="workos-section" id="task-gates"><SectionHeader kicker="Gate 决定" title="按 SOP 配置处理人工门禁"/><div className="evidence-list">{view.gates.length === 0 ? <Empty title="当前没有 Gate 评估" detail="审批是可配置的；没有 Gate 不会阻断低风险任务。"/> : view.gates.map(item => <div className="evidence-row" key={item.id}><span className="object-mark is-review"><ClipboardCheck size={15}/></span><div><strong>{item.gate_id}</strong><small>{item.gate_mode} · {item.reason || '等待决定'}</small></div><StatusText value={item.status}/>{item.status === 'pending' && <div className="input-actions"><Button variant="secondary" disabled={busy} onClick={() => decide(item.id, 'rejected')}>退回</Button><Button disabled={busy} onClick={() => decide(item.id, 'approved')}><Check size={14}/>通过</Button></div>}</div>)}</div></section>{showRevision && <section className="workos-section task-inline-editor"><SectionHeader kicker="Task Revision" title="提交正式内容结果"/><p className="editor-hint">内容必须符合当前 Task 的 Schema；提交后保留历史 Revision，不覆盖旧结果。</p><Field label="Revision JSON"><textarea value={revisionContent} onChange={event => setRevisionContent(event.target.value)} rows={10}/></Field><div className="new-task-actions"><Button variant="secondary" onClick={() => setShowRevision(false)}>取消</Button><Button onClick={submitRevision} disabled={busy}><Check size={15}/>提交 Revision</Button></div></section>}<section className="workos-section" id="task-deliveries"><SectionHeader kicker="Delivery" title="交付结果" action={task.status === 'accepted' ? <Button variant="secondary" onClick={() => setShowDelivery(value => !value)}><PackageCheck size={15}/>准备交付</Button> : undefined}/>{view.deliveries.length === 0 ? <Empty title="还没有交付记录" detail="接受 Revision 后，交付目的地和摘要会固定在这里。"/> : <div className="evidence-list">{view.deliveries.map(item => <EvidenceRow key={item.id} label={item.destination} detail={item.delivery_digest} status={item.status} icon={PackageCheck}/>)}</div>}{showDelivery && <div className="task-inline-editor"><Field label="交付目的地"><input value={deliveryDestination} onChange={event => setDeliveryDestination(event.target.value)}/></Field><div className="new-task-actions"><Button variant="secondary" onClick={() => setShowDelivery(false)}>取消</Button><Button onClick={deliver} disabled={busy}><PackageCheck size={15}/>确认交付</Button></div></div>}</section></main><aside className="task-side-panel"><SideFact label="状态" value={task.status}/><SideFact label="负责人" value={task.assignee_user_id || '未分派'} icon={UserRound}/><SideFact label="Environment" value={view.environment.name} icon={Workflow}/><SideFact label="风险" value={task.risk_profile} icon={task.risk_profile === 'high' ? ShieldAlert : ShieldCheck}/><SideFact label="Gate" value={gate ? `${gate.name} · ${gate.mode}` : '当前 Stage 未配置 Gate'} icon={ClipboardCheck}/><SideFact label="执行记录" value={`${view.runs.length} 次 Run`} icon={PlayCircle}/><SideFact label="Revision" value={`${view.revisions.length} 个`} icon={FileCheck2}/><div className="side-divider"></div><div className="side-block"><span>固定 SOP</span><strong>v{task.sop_version}</strong><small>{task.sop_digest}</small></div><div className="side-block"><span>执行方式</span><strong>{view.sop.default_execution_mode}</strong><small>本地客户端负责实际运行</small></div></aside></div></div>;
}

export function WorkOSKnowledgePage({projectID: explicitProjectID}: {projectID?: string}) {
  const {dashboard} = useWorkspace();
  const {openCreateProject} = useOutletContext<{openCreateProject: () => void}>();
  const {projectID: routeProjectID} = useParams();
  const projectID = explicitProjectID ?? routeProjectID;
  const project = dashboard.projects.find(item => item.id === projectID) || dashboard.projects[0];
  const routerNavigate = useNavigate();
  const navigate = (path: string) => {
    if (path === '/workspace/tasks/new' && project) {
      routerNavigate(`/projects/${encodeURIComponent(project.id)}/tasks/new`);
      return;
    }
    routerNavigate(path);
  };
  const [tab, setTab] = useState<'overview' | 'objects' | 'review' | 'sources' | 'packs' | 'query'>('overview');
  const [objects, setObjects] = useState<KnowledgeObject[]>([]);
  const [packs, setPacks] = useState<KnowledgePack[]>([]);
  const [sources, setSources] = useState<Source[]>([]);
  const [selected, setSelected] = useState<KnowledgeObject>();
  const [query, setQuery] = useState('');
  const [notice, setNotice] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [actionError, setActionError] = useState('');
  useEffect(() => {
    if (!project) {
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError('');
    Promise.all([
      api<KnowledgeObject[]>(`/api/bff/projects/${encodeURIComponent(project.id)}/knowledge-objects`),
      api<KnowledgePack[]>(`/api/bff/projects/${encodeURIComponent(project.id)}/knowledge-packs`),
      api<Source[]>(`/api/bff/projects/${encodeURIComponent(project.id)}/sources`)
    ]).then(([nextObjects, nextPacks, nextSources]) => { if (!cancelled) { setObjects(nextObjects); setPacks(nextPacks); setSources(nextSources); } }).catch(value => { if (!cancelled) setError(value instanceof Error ? value.message : '知识库加载失败'); }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [project?.id]);
  const filtered = objects.filter(item => tab !== 'review' ? true : ['needs_review', 'candidate', 'open', 'conflicted'].includes(item.status)).filter(item => query.trim() === '' || `${item.title}${item.statement}${item.object_type}${item.layer}`.toLowerCase().includes(query.toLowerCase()));
  const transition = async (decision: 'approve' | 'reject', message: string) => {
    if (!selected) return;
    try {
      const result = await post<{object: KnowledgeObject}>(`/api/bff/knowledge-objects/${encodeURIComponent(selected.id)}/transitions`, {expected_version: selected.version, expected_digest: selected.digest, decision, reason: message});
      setObjects(current => current.map(item => item.id === selected.id ? result.object : item));
      setSelected(undefined);
      setNotice(message);
      setActionError('');
    } catch (value) {
      setActionError(value instanceof Error ? value.message : '知识决策失败');
    }
  };
  if (!project) return <div className="workos-page"><PageHeader eyebrow="工作区 / 知识库" title="知识库" description="知识对象必须绑定真实 Project。" actions={<Button onClick={openCreateProject}><Plus size={15}/>创建 Project</Button>}/><Empty title="还没有 Project" detail="创建 Project 后，知识对象、来源和快照才有明确归属。"/></div>;
  if (loading) return <div className="workos-page knowledge-page"><PageHeader eyebrow={projectID ? `项目 / ${project.brand_name} / 知识库` : '工作区 / 知识库'} title="知识库" description="正在读取服务端知识对象和知识包。"/><div className="workos-loading">正在读取真实知识库...</div></div>;
  return <div className="workos-page knowledge-page"><PageHeader eyebrow={projectID ? `项目 / ${project.brand_name} / 知识库` : '工作区 / 知识库'} title="知识库" description="对象、来源、Evidence 和可发布知识快照在同一个工作面完成治理。" actions={<><Button variant="secondary" onClick={() => setTab('sources')}><FileInput size={15}/>登记来源</Button><Button onClick={() => navigate('/workspace/tasks/new')}><Plus size={15}/>创建补料任务</Button></>}/>{error && <div className="workos-notice is-error"><AlertCircle size={16}/>{error}</div>}{actionError && <div className="workos-notice is-error"><AlertCircle size={16}/><span>{actionError}</span><IconButton label="关闭错误" onClick={() => setActionError('')}><X size={15}/></IconButton></div>}<section className="knowledge-summary"><KnowledgeMetric label="对象总数" value={String(objects.length)} detail="服务端当前 Project"/><KnowledgeMetric label="待审核" value={String(objects.filter(item => item.status === 'needs_review' || item.status === 'candidate').length)} detail="需要明确决定" tone="warning"/><KnowledgeMetric label="冲突" value={String(objects.filter(item => item.status === 'conflicted' || item.object_type === 'ConflictRecord').length)} detail="按对象状态统计" tone="danger"/><KnowledgeMetric label="知识缺口" value={String(objects.filter(item => item.object_type === 'KnowledgeGap').length)} detail="可创建补料任务" tone="info"/><KnowledgeMetric label="已发布知识包" value={String(packs.filter(pack => pack.status === 'published').length)} detail={`${sources.length} 个来源`} tone="success"/></section>{notice && <div className="workos-notice is-success"><CheckCircle2 size={16}/><span>{notice}</span><IconButton label="关闭提示" onClick={() => setNotice('')}><X size={15}/></IconButton></div>}<div className="knowledge-tabs">{([['overview', '概览'], ['objects', '对象'], ['review', '待审与冲突'], ['sources', '来源与 Evidence'], ['packs', '知识包与快照'], ['query', '确定性查询']] as const).map(([id, label]) => <button key={id} className={tab === id ? 'is-active' : ''} onClick={() => setTab(id)}>{label}</button>)}</div>{tab === 'overview' ? <KnowledgeOverview objects={objects} onTab={setTab} onSelect={setSelected}/> : tab === 'sources' ? <KnowledgeSources projectID={project.id} sources={sources} onCreated={async () => {try {setSources(await api<Source[]>(`/api/bff/projects/${encodeURIComponent(project.id)}/sources`)); setNotice('来源已登记，等待本地解析和 Evidence 上报。'); setActionError('');} catch (value) {setActionError(value instanceof Error ? value.message : '来源列表刷新失败');}}}/> : tab === 'packs' ? <KnowledgePacks projectID={project.id} packs={packs} objects={objects} onChanged={async () => setPacks(await api<KnowledgePack[]>(`/api/bff/projects/${encodeURIComponent(project.id)}/knowledge-packs`))} onError={setActionError}/> : tab === 'query' ? <KnowledgeQuery projectID={project.id} packs={packs} query={query} setQuery={setQuery} onNotice={setNotice} onError={setActionError}/> : <section className="workos-section knowledge-object-section"><div className="knowledge-toolbar"><label className="search-field"><Search size={16}/><input value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索对象、陈述或类型" aria-label="搜索知识对象"/></label><span>{filtered.length} 个对象</span></div><div className="knowledge-object-table"><div className="knowledge-table-head"><span>对象</span><span>层级 / 类型</span><span>状态</span><span>Evidence</span><span>影响</span><span></span></div>{filtered.length === 0 ? <Empty title={tab === 'review' ? '没有待审核对象' : '还没有知识对象'} detail={tab === 'review' ? '服务端没有需要当前账号处理的对象。' : '本地客户端提交候选并完成来源绑定后，对象会显示在这里。'}/> : filtered.map(item => <button className="knowledge-object-row" key={item.id} onClick={() => setSelected(item)}><div><strong>{item.title}</strong><small><code>{item.id}</code> · v{item.version}</small></div><div><span className="layer-label">{item.layer}</span><small>{item.object_type}</small></div><StatusText value={item.status}/><div className="evidence-count"><FileCheck2 size={14}/>{item.evidence_refs.length ? `${item.evidence_refs.length} 条引用` : '缺 Evidence'}</div><small>{item.impact || '未标记'}</small><ChevronRight size={15}/></button>)}</div></section>}{selected && <KnowledgeDrawer value={selected} onClose={() => setSelected(undefined)} onAccept={() => transition('approve', '对象已生成新批准版本，历史快照不变。')} onEvidence={() => {setSelected(undefined); setNotice('补证据动作已记录为待处理，原对象状态未被伪造修改。');}} onTask={() => {setSelected(undefined); navigate('/workspace/tasks/new')}}/>}</div>;
}

export function WorkOSSOPPage({projectID}: {projectID?: string}) {
  const {dashboard} = useWorkspace();
  const project = dashboard.projects.find(item => item.id === projectID) || dashboard.projects[0];
  const [view, setView] = useState<ProjectSOPView>();
  const [selectedStageID, setSelectedStageID] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    if (!project) return;
    api<ProjectSOPView>(`/api/bff/projects/${encodeURIComponent(project.id)}/sop`).then(value => { const normalized = normalizeProjectSOPView(value); setView(normalized); setSelectedStageID(normalized.sop.stages[0]?.stage_id || ''); }).catch(value => setError(value instanceof Error ? value.message : '项目 SOP 加载失败'));
  }, [project?.id]);
  if (!project) return <div className="workos-page"><PageHeader eyebrow="项目 / SOP" title="SOP" description="SOP 必须绑定真实 Project。"/><Empty title="还没有 Project" detail="创建 Project 后，系统会为它生成可执行的 SOP 绑定。"/></div>;
  if (error) return <div className="workos-page"><Empty title="SOP 暂时不可用" detail={error}/></div>;
  if (!view) return <div className="workos-page"><div className="workos-loading">正在读取项目 SOP...</div></div>;
  const selectedStage = view.sop.stages.find(stage => stage.stage_id === selectedStageID) || view.sop.stages[0];
  const gate = selectedStage?.gate_ids.map(id => view.sop.gates.find(candidate => candidate.gate_id === id)).filter(Boolean)[0] as GateDefinition | undefined;
  return <div className="workos-page sop-page"><PageHeader eyebrow={`项目 / ${project.brand_name} / SOP`} title="SOP" description="项目使用已发布的 SOP 版本；Gate 是否启用由租户配置决定。"/><div className="sop-status-line"><span className="status-dot is-success"></span><strong>{view.sop.name}</strong><span>{view.sop.status === 'published' ? '已发布' : '草稿'} · v{view.sop.version} · {view.sop.digest || '尚未发布 digest'}</span><span className="sop-spacer"></span><span className="text-action">Environment：{view.binding.environment_id}</span></div><div className="sop-layout"><section className="workos-section sop-stages"><SectionHeader kicker="Stage 顺序" title={`${view.sop.name} · v${view.sop.version}`} action={<span className="section-count">{view.sop.stages.length} 个 Stage</span>}/><div className="sop-stage-list">{view.sop.stages.map((stage, index) => <button key={stage.stage_id} className={`sop-stage-row ${selectedStage?.stage_id === stage.stage_id ? 'is-selected' : ''}`} onClick={() => setSelectedStageID(stage.stage_id)}><span className="sop-stage-number">{index + 1}</span><span className="sop-stage-icon"><Workflow size={16}/></span><span><strong>{stage.name}</strong><small>{stage.output_schema} · {stage.execution_modes.join(' / ') || '未声明执行方式'}</small></span><span className={`sop-gate ${stage.gate_ids.length === 0 ? 'is-off' : ''}`}>{stage.gate_ids.length === 0 ? '无 Gate' : `${stage.gate_ids.length} 个 Gate`}</span><ChevronRight size={15}/></button>)}</div></section><aside className="workos-section sop-config"><SectionHeader kicker="当前配置" title={selectedStage?.name || 'Stage'}/>{selectedStage ? <div className="sop-config-card"><div className="selected-stage"><span className="sop-stage-icon"><Workflow size={16}/></span><div><strong>{selectedStage.name}</strong><small>Stage {selectedStage.order} · {selectedStage.output_schema}</small></div></div><dl className="object-facts"><div><dt>输入引用</dt><dd>{selectedStage.input_refs.join('、') || '无'}</dd></div><div><dt>检查</dt><dd>{selectedStage.checks.join('、') || '未配置'}</dd></div><div><dt>所需能力</dt><dd>{selectedStage.required_capabilities.join('、') || '无'}</dd></div><div><dt>Gate</dt><dd>{gate ? `${gate.name} · ${gate.mode}` : '未启用'}</dd></div></dl><div className="sop-checklist"><CheckLine label="Stage 输入 / 输出已定义"/><CheckLine label={gate ? `Gate 模式：${gate.mode}` : '当前 Stage 不需要 Gate'}/><CheckLine label={`执行方式：${selectedStage.execution_modes.join(' / ') || '未声明'}`}/></div></div> : <Empty title="没有 Stage" detail="该 SOP 版本尚未定义 Stage。"/>}</aside></div></div>;
}

export function AdminWorkOSPage({kind}: {kind: 'overview' | 'environment' | 'sop' | 'gate' | 'capability' | 'audit' | 'usage'}) {
  const page = {overview: {eyebrow: '管理后台 / Work OS', title: '运行基础设施', description: '从 Environment、SOP、Gate 和本地能力开始管理内容生产的真实运行边界。'}, environment: {eyebrow: '管理后台 / Environment', title: 'Environment', description: '管理真实租户运行环境、健康状态和默认 SOP。'}, sop: {eyebrow: '管理后台 / SOP Registry', title: 'SOP Registry', description: '从已发布版本创建草稿，编辑后再发布，历史任务不被改写。'}, gate: {eyebrow: '管理后台 / Gate Policy', title: 'Gate 配置', description: '按每个 SOP 版本配置 none、required_check、internal_review 或 client_decision，审批不是固定必经步骤。'}, capability: {eyebrow: '管理后台 / Local Execution', title: '本地能力', description: '查看 Environment 声明的本地能力；实际执行仍在客户端完成。'}, audit: {eyebrow: '管理后台 / Audit', title: '权限与审计', description: '查看真实配置变更、任务创建和版本发布记录。'}, usage: {eyebrow: '管理后台 / Usage', title: '用量与成本', description: '按当前租户的真实 WorkTask 状态查看运行和 Gate 等待。'}}[kind];
  const [data, setData] = useState<AdminWorkOSView>();
  const [tasks, setTasks] = useState<ApiWorkTask[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [query, setQuery] = useState('');
  const load = async () => {
    setLoading(true);
    setError('');
    try {
      const workOS = await api<AdminWorkOSView>('/api/bff/admin/work-os');
      setData(normalizeAdminWorkOSView(workOS));
      try { setTasks(normalizeWorkTaskList(await api<ApiWorkTask[]>('/api/bff/tasks'))); } catch { setTasks([]); }
    } catch (value) { setError(value instanceof Error ? value.message : '管理配置加载失败'); } finally { setLoading(false); }
  };
  useEffect(() => { load(); }, []);
  if (loading) return <div className="workos-page admin-workos-page"><PageHeader eyebrow={page.eyebrow} title={page.title} description={page.description}/><div className="workos-loading">正在读取真实配置...</div></div>;
  if (error || !data) return <div className="workos-page admin-workos-page"><PageHeader eyebrow={page.eyebrow} title={page.title} description={page.description}/><Empty title="后台数据不可用" detail={error || '服务端没有返回配置。'} action={<Button onClick={load}>重试</Button>}/></div>;
  return <div className="workos-page admin-workos-page"><PageHeader eyebrow={page.eyebrow} title={page.title} description={page.description}/>{kind === 'overview' ? <AdminOverviewPanel data={data} tasks={tasks}/> : kind === 'environment' ? <ConfigEnvironmentPanel environments={data.environments} sops={data.sops} onSaved={load}/> : kind === 'sop' ? <ConfigSOPPanel sops={data.sops} onChanged={load}/> : kind === 'gate' ? <AdminGatePanel gates={data.gates} query={query} setQuery={setQuery}/> : kind === 'capability' ? <AdminCapabilityPanel environments={data.environments} capabilities={data.capabilities}/> : kind === 'audit' ? <AdminAuditPanel audit={data.audit} query={query} setQuery={setQuery}/> : <AdminUsagePanel usage={data.usage} generatedAt={data.generated_at}/>}<section className="admin-config-note"><ShieldCheck size={18}/><div><strong>配置只影响后续任务</strong><p>这里读取服务端事实；已经固定 SOP digest 的历史任务不会被静默替换，本地 Workspace 仍由用户确认后执行。</p></div></section></div>;
}

function AdminOverviewPanel({data, tasks}: {data: AdminWorkOSView; tasks: ApiWorkTask[]}) {
  const navigate = useNavigate();
  const publishedVersions = data.sops.flatMap(summary => summary.versions.filter(version => version.status === 'published'));
  const draftVersions = data.sops.flatMap(summary => summary.versions.filter(version => version.status === 'draft'));
  const enabledCapabilities = data.environments.reduce((count, environment) => count + environment.capabilities.filter(capability => capability.enabled).length, 0);
  const requiredGates = data.gates.filter(gate => gate.mode === 'required').length;
  const latestAudit = data.audit.slice(0, 5);
  const activeEnvironments = data.environments.filter(environment => environment.status === 'active');
  const attentionTasks = tasks.filter(task => ['needs_input', 'waiting_gate', 'blocked'].includes(task.status));
  const recentTasks = [...tasks].sort((left, right) => new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()).slice(0, 5);
  return <>
    <section className="admin-control-banner"><div className="admin-control-state"><span className="status-dot is-success"></span><div><strong>租户运行控制面已连接</strong><span>Environment、SOP 和 Gate 的变更会在服务端留痕，并作为后续任务的创建约束。</span></div></div><div className="admin-control-facts"><span><small>活跃 Environment</small><b>{activeEnvironments.length}/{data.environments.length}</b></span><span><small>待处理任务</small><b>{attentionTasks.length}</b></span><span><small>同步时间</small><b>{formatDateTime(data.generated_at)}</b></span></div></section>
    <section className="workos-metrics" aria-label="运行基础设施状态">
      <Metric icon={Workflow} label="运行环境" value={String(data.environments.length)} detail={`${activeEnvironments.length} 个 active`} tone="ink"/>
      <Metric icon={GitBranch} label="已发布 SOP" value={String(publishedVersions.length)} detail={`${draftVersions.length} 个草稿待处理`} tone="production"/>
      <Metric icon={ClipboardCheck} label="Gate 配置" value={String(data.gates.length)} detail={`${requiredGates} 个必选阻断`} tone="review"/>
      <Metric icon={ListTodo} label="WorkTask" value={String(data.usage.task_count)} detail={`${data.usage.running_count} 个正在运行`} tone="success"/>
    </section>
    <section className="workos-section admin-task-snapshot"><SectionHeader kicker="任务运行" title="当前任务状态" action={<button className="text-action" onClick={() => navigate('/workspace/tasks')}>打开任务中心 <ArrowRight size={14}/></button>}/>{recentTasks.length === 0 ? <Empty title="还没有真实任务" detail="后台不会用演示数据填充运行面；创建任务并由本地客户端上报后，状态会出现在这里。" action={<button className="button button-secondary" onClick={() => navigate('/workspace/tasks/new')}><Plus size={15}/>创建任务</button>}/> : <div className="admin-task-table"><div className="admin-task-table-head"><span>任务</span><span>状态</span><span>Stage</span><span>下一动作</span><span>更新时间</span></div>{recentTasks.map(task => <button key={task.id} className="admin-task-row" onClick={() => navigate(`/workspace/tasks/${task.id}`)}><span><strong>{task.title}</strong><small>{task.content_type || '内容任务'} · {task.sop_id} v{task.sop_version}</small></span><StatusText value={task.status}/><span>{task.current_stage_id || '未开始'}</span><span>{task.next_action || '等待客户端继续'}</span><time>{formatDateTime(task.updated_at)}</time><ArrowRight size={14}/></button>)}</div>}</section>
    <div className="workos-home-grid">
      <section className="workos-section admin-config-section">
        <SectionHeader kicker="运行边界" title="Environment" action={<button className="text-action" onClick={() => navigate('/admin/environments')}>管理 Environment <ArrowRight size={14}/></button>}/>
        <div className="admin-config-table">{data.environments.length === 0 ? <Empty title="还没有 Environment" detail="保存第一个租户运行环境后，任务才有稳定的执行边界。"/> : data.environments.map(environment => <article key={environment.id}><div className="admin-row-icon"><Workflow size={17}/></div><div><strong>{environment.name}</strong><span>{environment.slug} · 默认 SOP {environment.default_sop_id ? `v${environment.default_sop_version}` : '未设置'} · {environment.capabilities.filter(capability => capability.enabled).length} 个能力</span></div><span className={`config-state ${environment.status === 'active' ? 'is-success' : 'is-warning'}`}>{environment.status === 'active' ? '可运行' : '已暂停'}</span><span className="text-action">{environment.manifest_digest.slice(0, 15)}…</span></article>)}</div>
      </section>
      <section className="workos-section admin-config-section">
        <SectionHeader kicker="SOP Registry" title="版本状态" action={<button className="text-action" onClick={() => navigate('/admin/sops')}>管理 SOP <ArrowRight size={14}/></button>}/>
        <div className="admin-config-table">{data.sops.length === 0 ? <Empty title="还没有 SOP" detail="发布一个 SOP 后，普通用户才能用低成本方式创建任务。"/> : data.sops.map(summary => <article key={summary.definition.id}><div className="admin-row-icon"><GitBranch size={17}/></div><div><strong>{summary.definition.name}</strong><span>{summary.definition.description || '未填写说明'} · {summary.versions.length} 个版本</span></div><span className="config-state is-info">v{summary.definition.current_version}</span><span className="text-action">{summary.versions.filter(version => version.status === 'draft').length ? `${summary.versions.filter(version => version.status === 'draft').length} 个草稿` : '已同步'}</span></article>)}</div>
      </section>
    </div>
    <div className="workos-home-grid">
      <section className="workos-section admin-config-section">
        <SectionHeader kicker="本地能力" title="能力声明" action={<button className="text-action" onClick={() => navigate('/admin/capabilities')}>查看能力 <ArrowRight size={14}/></button>}/>
        <div className="admin-config-table"><article><div className="admin-row-icon"><Sparkles size={17}/></div><div><strong>Environment 已启用能力</strong><span>由客户端本地执行，云端只记录声明和结果引用</span></div><span className="config-state is-success">{enabledCapabilities} 个</span><span className="text-action">本地执行</span></article><article><div className="admin-row-icon"><Settings2 size={17}/></div><div><strong>已登记客户端能力</strong><span>来自已连接的 Workspace / CLI Adapter</span></div><span className="config-state is-info">{data.capabilities.length} 个</span><span className="text-action">仅查看契约</span></article></div>
      </section>
      <section className="workos-section admin-config-section">
        <SectionHeader kicker="最近审计" title="配置与任务变化" action={<button className="text-action" onClick={() => navigate('/admin/audit')}>查看审计 <ArrowRight size={14}/></button>}/>
        <div className="admin-config-table">{latestAudit.length === 0 ? <Empty title="还没有审计记录" detail="配置保存、SOP 发布和任务创建会写入这里。"/> : latestAudit.map(event => <article key={event.id}><div className="admin-row-icon"><ShieldCheck size={17}/></div><div><strong>{event.action}</strong><span>{event.subject_type} · {event.subject_id}</span></div><span className="config-state is-info">已记录</span><time>{formatDateTime(event.created_at)}</time></article>)}</div>
      </section>
    </div>
  </>;
}

function AdminEnvironmentPanel({environments, sops, onSaved}: {environments: Environment[]; sops: SOPSummary[]; onSaved: () => Promise<void> | void}) {
  const [selectedID, setSelectedID] = useState(environments[0]?.id || '');
  const selected = environments.find(value => value.id === selectedID) || environments[0];
  const [form, setForm] = useState<Environment | undefined>(selected);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState('');
  useEffect(() => { setForm(selected); }, [selectedID, environments]);
  if (!selected || !form) return <Empty title="还没有 Environment" detail="服务端尚未初始化租户运行环境。"/>;
  const updateCapability = (id: string, enabled: boolean) => setForm({...form, capabilities: form.capabilities.map(capability => capability.id === id ? {...capability, enabled} : capability)});
  const save = async () => { setSaving(true); setNotice(''); try { await patch<Environment>(`/api/bff/admin/environments/${encodeURIComponent(form.id)}`, {name: form.name, slug: form.slug, status: form.status, default_sop_id: form.default_sop_id || '', default_sop_version: form.default_sop_version || 0, capabilities: form.capabilities}); setNotice('Environment 已保存，后续新任务会读取这份配置。'); await onSaved(); } catch (value) { setNotice(value instanceof Error ? value.message : 'Environment 保存失败'); } finally { setSaving(false); } };
  return <><section className="workos-section admin-config-section"><div className="admin-config-toolbar"><label className="search-field"><Search size={16}/><input placeholder="搜索 Environment" aria-label="搜索 Environment" value={form.name} onChange={event => setForm({...form, name: event.target.value})}/></label><span className="toolbar-count">{environments.length} 个 Environment</span></div><div className="admin-config-table">{environments.map(environment => <button className={`admin-select-row ${environment.id === form.id ? 'is-selected' : ''}`} key={environment.id} onClick={() => setSelectedID(environment.id)}><span className="admin-row-icon"><Workflow size={17}/></span><span><strong>{environment.name}</strong><small>{environment.slug} · {environment.manifest_digest.slice(0, 18)}…</small></span><span className={`config-state ${environment.status === 'active' ? 'is-success' : 'is-warning'}`}>{environment.status === 'active' ? '运行中' : '已暂停'}</span><ChevronRight size={15}/></button>)}</div></section><section className="workos-section admin-editor-section"><SectionHeader kicker="Environment 配置" title={form.name}/><div className="form-grid"><Field label="名称"><input value={form.name} onChange={event => setForm({...form, name: event.target.value})}/></Field><Field label="标识"><input value={form.slug} onChange={event => setForm({...form, slug: event.target.value})}/></Field><Field label="状态"><select value={form.status} onChange={event => setForm({...form, status: event.target.value})}><option value="active">active · 可创建任务</option><option value="paused">paused · 暂停新任务</option></select></Field><Field label="默认 SOP"><select value={`${form.default_sop_id || ''}:${form.default_sop_version || 0}`} onChange={event => {const [sopID, version] = event.target.value.split(':'); setForm({...form, default_sop_id: sopID, default_sop_version: Number(version)});}}>{sops.flatMap(summary => summary.versions.filter(version => version.status === 'published').map(version => <option key={`${summary.definition.id}:${version.version}`} value={`${summary.definition.id}:${version.version}`}>{summary.definition.name} · v{version.version}</option>))}</select></Field></div><div className="admin-capability-list"><strong>声明的本地能力</strong>{form.capabilities.length === 0 ? <Empty title="没有能力声明" detail="客户端不会收到可执行能力。"/> : form.capabilities.map(capability => <label className="toggle-line" key={capability.id}><span><strong>{capability.id}</strong><small>v{capability.version}</small></span><input type="checkbox" checked={capability.enabled} onChange={event => updateCapability(capability.id, event.target.checked)}/></label>)}</div><div className="new-task-actions"><Button onClick={save} disabled={saving}><Check size={15}/>{saving ? '保存中...' : '保存 Environment'}</Button></div>{notice && <div className="workos-notice is-info"><CircleHelp size={16}/>{notice}</div>}</section></>;
}

function AdminSOPPanel({sops, onChanged}: {sops: SOPSummary[]; onChanged: () => Promise<void> | void}) {
  const [selectedSOPID, setSelectedSOPID] = useState(sops[0]?.definition.id || '');
  const summary = sops.find(value => value.definition.id === selectedSOPID) || sops[0];
  const [selectedVersion, setSelectedVersion] = useState(0);
  const version = summary?.versions.find(value => value.version === selectedVersion) || summary?.versions[0];
  const [draft, setDraft] = useState<SOPVersion | undefined>(version);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  useEffect(() => { setDraft(version); }, [selectedSOPID, selectedVersion, sops]);
  if (!summary || !draft) return <Empty title="还没有 SOP" detail="服务端尚未初始化租户 SOP。"/>;
  const editable = draft.status === 'draft';
  const createDraft = async () => { setBusy(true); setNotice(''); try { const created = await post<SOPVersion>(`/api/bff/admin/sops/${encodeURIComponent(summary.definition.id)}/versions`, {source_version: draft.version}); setSelectedSOPID(created.sop_id); setSelectedVersion(created.version); setNotice(`已创建 v${created.version} 草稿。`); await onChanged(); } catch (value) { setNotice(value instanceof Error ? value.message : '草稿创建失败'); } finally { setBusy(false); } };
  const saveDraft = async () => { if (!editable) return; setBusy(true); setNotice(''); try { const saved = await patch<SOPVersion>(`/api/bff/admin/sops/${encodeURIComponent(draft.sop_id)}/versions/${draft.version}`, {name: draft.name, description: draft.description, content_types: draft.content_types, stages: draft.stages, gates: draft.gates, default_execution_mode: draft.default_execution_mode}); setDraft(saved); setNotice('SOP 草稿已保存。'); await onChanged(); } catch (value) { setNotice(value instanceof Error ? value.message : 'SOP 草稿保存失败'); } finally { setBusy(false); } };
  const publish = async () => { setBusy(true); setNotice(''); try { const published = await post<SOPVersion>(`/api/bff/admin/sops/${encodeURIComponent(draft.sop_id)}/versions/${draft.version}/publish`); setDraft(published); setNotice(`SOP v${published.version} 已发布，digest 已固定。`); await onChanged(); } catch (value) { setNotice(value instanceof Error ? value.message : 'SOP 发布失败'); } finally { setBusy(false); } };
  const setGateMode = (gateID: string, mode: string) => setDraft({...draft, gates: draft.gates.map(gate => gate.gate_id === gateID ? {...gate, mode, blocking: mode === 'required'} : gate)});
  return <><section className="workos-section admin-config-section"><div className="admin-config-toolbar"><label className="search-field"><GitBranch size={16}/><select aria-label="选择 SOP" value={summary.definition.id} onChange={event => {setSelectedSOPID(event.target.value); setSelectedVersion(0);}}>{sops.map(value => <option key={value.definition.id} value={value.definition.id}>{value.definition.name}</option>)}</select></label><span className="toolbar-count">{summary.versions.length} 个版本</span></div><div className="admin-version-tabs">{summary.versions.map(candidate => <button key={candidate.version} className={candidate.version === draft.version ? 'is-active' : ''} onClick={() => setSelectedVersion(candidate.version)}>v{candidate.version} <small>{candidate.status === 'published' ? '已发布' : '草稿'}</small></button>)}</div></section><section className="workos-section admin-editor-section"><SectionHeader kicker="版本配置" title={`${draft.name} · v${draft.version}`} action={editable ? <span className="config-state is-warning">可编辑草稿</span> : <span className="config-state is-success">已发布，不可直接修改</span>}/><Field label="名称"><input value={draft.name} readOnly={!editable} onChange={event => setDraft({...draft, name: event.target.value})}/></Field><Field label="说明"><textarea value={draft.description} readOnly={!editable} onChange={event => setDraft({...draft, description: event.target.value})} rows={3}/></Field><div className="admin-sop-stage-editor"><strong>Stages</strong>{draft.stages.map(stage => <article key={stage.stage_id}><div><strong>{stage.name}</strong><small>#{stage.order} · {stage.output_schema}</small></div><span>{stage.execution_modes.join(' / ') || '未声明'}</span><span>{stage.gate_ids.length ? `${stage.gate_ids.length} 个 Gate` : '无 Gate'}</span></article>)}</div><div className="admin-gate-editor"><strong>Gate 策略</strong>{draft.gates.length === 0 ? <Empty title="没有 Gate" detail="该 SOP 版本不创建 Gate。"/> : draft.gates.map(gate => <div className="admin-gate-row" key={gate.gate_id}><div><strong>{gate.name}</strong><small>{gate.gate_id} · {gate.blocking ? '阻断' : '不阻断'}</small></div><select disabled={!editable} value={gate.mode} onChange={event => setGateMode(gate.gate_id, event.target.value)}><option value="none">none · 不创建</option><option value="advisory">advisory · 可选建议</option><option value="required">required · 必选阻断</option></select></div>)}</div><div className="new-task-actions">{editable ? <><Button variant="secondary" onClick={saveDraft} disabled={busy}><Check size={15}/>保存草稿</Button><Button onClick={publish} disabled={busy}><PackageCheck size={15}/>发布版本</Button></> : <Button onClick={createDraft} disabled={busy}><GitBranch size={15}/>从此版本创建草稿</Button>}</div>{notice && <div className="workos-notice is-info"><CircleHelp size={16}/>{notice}</div>}</section></>;
}

function AdminGatePanel({gates, query, setQuery}: {gates: AdminWorkOSView['gates']; query:string; setQuery:(value:string)=>void}) {
  const filtered = gates.filter(gate => `${gate.sop_name}${gate.name}${gate.mode}`.toLowerCase().includes(query.toLowerCase()));
  return <section className="workos-section admin-config-section"><div className="admin-config-toolbar"><label className="search-field"><Search size={16}/><input value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索 Gate、SOP 或模式" aria-label="搜索 Gate"/></label><span className="toolbar-count">{filtered.length} 个 Gate</span></div><div className="admin-config-table">{filtered.length === 0 ? <Empty title="没有 Gate 配置" detail="发布的 SOP 可以选择完全不创建 Gate。"/> : filtered.map(gate => <article key={`${gate.sop_id}:${gate.sop_version}:${gate.gate_id}`}><div className="admin-row-icon"><ClipboardCheck size={17}/></div><div><strong>{gate.name}</strong><span>{gate.sop_name} · v{gate.sop_version} · 使用 {gate.usage_count} 个任务</span></div><span className={`config-state ${['required', 'internal_review', 'client_decision'].includes(gate.mode) ? 'is-warning' : gate.mode === 'none' ? 'is-muted' : 'is-info'}`}>{gate.mode === 'required' ? '兼容必选 / 阻断' : gate.mode === 'internal_review' ? '内部决定 / 阻断' : gate.mode === 'client_decision' ? '客户决定 / 阻断' : gate.mode === 'required_check' ? '确定性检查' : gate.mode === 'advisory' ? '可选 / 建议' : '未启用'}</span><span className="text-action">在 SOP 版本中编辑 <ArrowRight size={14}/></span></article>)}</div></section>;
}

function AdminCapabilityPanel({environments, capabilities}: {environments:Environment[]; capabilities:AdminWorkOSView['capabilities']}) {
  const declared = environments.flatMap(environment => environment.capabilities.map(capability => ({...capability, environment: environment.name}))).filter(value => value.enabled);
  return <section className="workos-section admin-config-section"><div className="admin-config-toolbar"><span className="toolbar-count">{declared.length} 个已启用声明 · {capabilities.length} 个已登记客户端能力</span></div><div className="admin-config-table">{declared.length === 0 && capabilities.length === 0 ? <Empty title="没有本地能力声明" detail="Environment 未启用任何能力；客户端不会被远程触发。"/> : <>{declared.map(capability => <article key={`${capability.environment}:${capability.id}`}><div className="admin-row-icon"><Sparkles size={17}/></div><div><strong>{capability.id}</strong><span>{capability.environment} · v{capability.version}</span></div><span className="config-state is-success">已启用</span><span className="text-action">仅本地执行</span></article>)}{capabilities.map(capability => <article key={`client:${capability.id}:${capability.version}`}><div className="admin-row-icon"><Settings2 size={17}/></div><div><strong>{capability.id}</strong><span>客户端登记 · v{capability.version} · {capability.kind}</span></div><span className="config-state is-info">已登记</span><span className="text-action">查看契约</span></article>)}</>}</div></section>;
}

function AdminAuditPanel({audit, query, setQuery}: {audit:AdminWorkOSView['audit']; query:string; setQuery:(value:string)=>void}) {
  const filtered = audit.filter(event => `${event.action}${event.subject_type}${event.subject_id}`.toLowerCase().includes(query.toLowerCase()));
  return <section className="workos-section admin-config-section"><div className="admin-config-toolbar"><label className="search-field"><Search size={16}/><input value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索操作或对象" aria-label="搜索审计"/></label><span className="toolbar-count">{filtered.length} 条记录</span></div><div className="admin-config-table">{filtered.length === 0 ? <Empty title="没有审计记录" detail="新的配置变更和任务操作会在服务端写入这里。"/> : filtered.map(event => <article key={event.id}><div className="admin-row-icon"><ShieldCheck size={17}/></div><div><strong>{event.action}</strong><span>{event.subject_type} · {event.subject_id}</span></div><span className="config-state is-info">已记录</span><time>{formatDateTime(event.created_at)}</time></article>)}</div></section>;
}

function AdminUsagePanel({usage, generatedAt}: {usage:AdminWorkOSView['usage']; generatedAt:string}) {
  const modes = Object.entries(usage.by_execution_mode);
  return <><section className="workos-metrics"><Metric icon={ListTodo} label="WorkTask 总数" value={String(usage.task_count)} detail="当前租户真实任务" tone="ink"/><Metric icon={PlayCircle} label="运行中" value={String(usage.running_count)} detail="等待本地客户端" tone="production"/><Metric icon={ClipboardCheck} label="待 Gate 决定" value={String(usage.waiting_gate_count)} detail="不等于所有任务必审" tone="review"/><Metric icon={Clock3} label="数据更新时间" value={formatDateTime(generatedAt)} detail="服务端聚合" tone="success"/></section><section className="workos-section admin-config-section"><SectionHeader kicker="执行方式" title="WorkTask 分布"/>{modes.length === 0 ? <Empty title="还没有执行记录" detail="创建任务并由本地客户端执行后，这里会按执行方式统计。"/> : <div className="admin-config-table">{modes.map(([mode, count]) => <article key={mode}><div className="admin-row-icon"><Workflow size={17}/></div><div><strong>{mode}</strong><span>当前任务执行方式</span></div><span className="config-state is-info">{count} 个</span><span className="text-action">查看任务 <ArrowRight size={14}/></span></article>)}</div>}</section></>;
}

function KnowledgeOverview({objects, onTab, onSelect}: {objects: KnowledgeObject[]; onTab: (tab: 'objects' | 'review' | 'sources' | 'packs' | 'query') => void; onSelect: (object: KnowledgeObject) => void}) { const layers = [['identity', '身份'], ['product', '产品'], ['market', '市场'], ['expression', '表达'], ['operations', '运营'], ['content_engine', '内容引擎'], ['compliance', '合规']]; const statusFor = (layer: string) => { const scoped = objects.filter(item => item.layer === layer); if (scoped.length === 0) return '未建立'; if (scoped.some(item => item.status === 'conflicted' || item.object_type === 'ConflictRecord')) return '有冲突'; if (scoped.some(item => ['needs_review', 'candidate', 'open'].includes(item.status))) return '待处理'; return '已覆盖'; }; const reviewObjects = objects.filter(item => item.status !== 'approved').slice(0, 4); return <div className="knowledge-overview-grid"><section className="workos-section"><SectionHeader kicker="七层覆盖" title="基础建设状态"/><div className="coverage-list">{layers.map(([layer, label]) => { const status = statusFor(layer); return <button key={layer} onClick={() => onTab('objects')}><span className="coverage-bar"><i className={status === '已覆盖' ? 'is-full' : status === '有冲突' ? 'is-danger' : 'is-half'}></i></span><strong>{label}</strong><small>{status}</small><ChevronRight size={14}/></button>; })}</div></section><section className="workos-section"><SectionHeader kicker="最近变化" title="需要你处理的对象" action={<button className="text-action" onClick={() => onTab('review')}>打开待审 <ArrowRight size={14}/></button>}/><div className="knowledge-review-list">{reviewObjects.length === 0 ? <Empty title="没有待处理对象" detail="服务端没有需要当前账号处理的知识对象。"/> : reviewObjects.map(item => <button key={item.id} onClick={() => onSelect(item)}><span className="object-mark is-review"><BookOpen size={15}/></span><span><strong>{item.title}</strong><small>{knowledgeStatusLabel[item.status] || item.status} · {item.impact || '需要确认来源'}</small></span><ChevronRight size={14}/></button>)}</div></section><section className="workos-section knowledge-query-card"><SectionHeader kicker="确定性查询" title="查询前先选范围"/><p>返回 eligible、blocked 和 gaps。查询不会修改知识对象，也不会把 blocked 当作可用事实。</p><button className="text-action" onClick={() => onTab('query')}>运行一次查询 <ArrowRight size={14}/></button></section></div>; }

function KnowledgeSources({projectID, sources, onCreated}: {projectID: string; sources: Source[]; onCreated: () => Promise<void>}) {
  const [name, setName] = useState('');
  const [file, setFile] = useState<File>();
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState('');
  const [selectedID, setSelectedID] = useState('');
  const [revisionFile, setRevisionFile] = useState<File>();
  const [revisions, setRevisions] = useState<SourceRevision[]>([]);
  const [evidence, setEvidence] = useState<EvidenceSpan[]>([]);
  const selected = sources.find(source => source.id === selectedID);
  useEffect(() => {
    if (!selected) { setRevisions([]); setEvidence([]); return; }
    api<SourceRevision[]>(`/api/bff/sources/${encodeURIComponent(selected.id)}/revisions`).then(next => { setRevisions(next); const latest = next[0]; if (latest) return api<EvidenceSpan[]>(`/api/bff/source-revisions/${encodeURIComponent(latest.id)}/evidence`); return []; }).then(setEvidence).catch(() => { setRevisions([]); setEvidence([]); });
  }, [selectedID, sources]);
  const create = async () => {
    if (!name.trim() || !file) { setNotice('请填写来源名称并选择文件。'); return; }
    setBusy(true);
    setNotice('');
    try { const form = new FormData(); form.append('file', file); form.append('name', name.trim()); form.append('source_type', 'local_import'); await upload<SourceRevision>('/api/bff/projects/' + encodeURIComponent(projectID) + '/sources/upload', form); setName(''); setFile(undefined); await onCreated(); setNotice('来源已上传，等待本地解析器生成 Evidence。'); } catch (value) { setNotice(value instanceof Error ? value.message : '来源上传失败'); } finally { setBusy(false); }
  };
  const revise = async () => { if (!selected || !revisionFile) { setNotice('请选择要追加到当前来源的文件。'); return; } setBusy(true); setNotice(''); try { const form = new FormData(); form.append('file', revisionFile); await upload<SourceRevision>(`/api/bff/sources/${encodeURIComponent(selected.id)}/revisions/upload`, form); setRevisionFile(undefined); setNotice('来源修订已上传，旧 Evidence 保留并等待重新解析。'); const next = await api<SourceRevision[]>(`/api/bff/sources/${encodeURIComponent(selected.id)}/revisions`); setRevisions(next); } catch (value) { setNotice(value instanceof Error ? value.message : '来源修订上传失败'); } finally { setBusy(false); } };
  return <div className="knowledge-subview-grid"><section className="workos-section knowledge-subview"><SectionHeader kicker="来源与 Evidence" title="来源登记和证据定位" action={<span className="section-count">{sources.length} 个来源</span>}/><div className="source-create-form"><Field label="来源名称"><input value={name} onChange={event => setName(event.target.value)} placeholder="例如：品牌 Brief / 本地会话摘录"/></Field><Field label="本地文件"><input type="file" accept=".pdf,.docx,.xlsx,.pptx,.png,.jpg,.jpeg,.txt" onChange={event => setFile(event.target.files?.[0])}/></Field><Button onClick={create} disabled={busy || !name.trim() || !file}><Plus size={15}/>{busy ? '上传中...' : '上传来源'}</Button></div>{notice && <div className="workos-notice is-info">{notice}</div>}<div className="source-list">{sources.length === 0 ? <Empty title="暂无已登记来源" detail="选择本地文件上传后，服务端保存 digest，解析器再生成 Evidence。"/> : sources.map(source => <button className={`source-row ${source.id === selectedID ? 'is-selected' : ''}`} key={source.id} onClick={() => setSelectedID(source.id)}><span className="object-mark is-source"><FileInput size={15}/></span><span><strong>{source.name}</strong><small>{source.source_type} · {source.revision_count} 个修订 · {source.status}</small></span><ChevronRight size={15}/></button>)}</div></section><section className="workos-section knowledge-subview"><SectionHeader kicker="Evidence" title={selected ? selected.name : '选择一个来源'}/>{!selected ? <Empty title="还没有选择来源" detail="选择左侧来源查看修订、解析状态和 Evidence 数量。"/> : <><div className="source-detail-facts"><span>最新修订 <b>{revisions[0]?.file_name || '未上传'}</b></span><span>解析状态 <b>{revisions[0]?.processing_status || selected.status}</b></span><span>Evidence <b>{evidence.length}</b></span></div><div className="source-revision-upload"><input type="file" accept=".pdf,.docx,.xlsx,.pptx,.png,.jpg,.jpeg,.txt" onChange={event => setRevisionFile(event.target.files?.[0])}/><Button variant="secondary" onClick={revise} disabled={busy || !revisionFile}><FileInput size={15}/>上传新修订</Button></div><div className="evidence-list">{evidence.length === 0 ? <Empty title="尚无 Evidence" detail="本地解析完成后，Evidence 会保留定位、引用文本和审核状态。"/> : evidence.map(item => <div className="evidence-row" key={item.id}><span className="object-mark is-source"><FileCheck2 size={15}/></span><div><strong>{item.quote_text}</strong><small>{item.locator_kind} · {item.review_status}</small></div><StatusText value={item.review_status}/><ChevronRight size={14}/></div>)}</div></>}</section></div>;
}
function KnowledgePacks({projectID, packs, objects, onChanged, onError}: {projectID: string; packs: KnowledgePack[]; objects: KnowledgeObject[]; onChanged: () => Promise<void>; onError: (value: string) => void}) {
  const [name, setName] = useState('内容生产知识包');
  const [purpose, setPurpose] = useState('content_production');
  const [busy, setBusy] = useState(false);
  const [snapshots, setSnapshots] = useState<Record<string, KnowledgeSnapshot[]>>({});
  const eligible = objects.filter(object => ['approved', 'verified', 'valid', 'active'].includes(object.status));
  useEffect(() => { let cancelled = false; packs.filter(pack => pack.status === 'published').forEach(pack => api<KnowledgeSnapshot[]>(`/api/bff/projects/${encodeURIComponent(projectID)}/knowledge-packs/${encodeURIComponent(pack.id)}/snapshots`).then(next => { if (!cancelled) setSnapshots(current => ({...current, [pack.id]: next})); }).catch(value => { if (!cancelled) onError(value instanceof Error ? value.message : '知识快照加载失败'); })); return () => { cancelled = true; }; }, [onError, packs, projectID]);
  const create = async () => {
    if (!name.trim() || eligible.length === 0) return;
    setBusy(true);
    try { await post<KnowledgePack>(`/api/bff/projects/${encodeURIComponent(projectID)}/knowledge-packs`, {name: name.trim(), purpose, object_refs: eligible.map(object => ({object_id: object.id, version: object.version})), query_policy: {eligible_statuses: ['approved', 'verified', 'valid', 'active'], allowed_object_types: [], require_evidence: true, block_on_conflict: true, block_on_rights_failure: true}}); await onChanged(); onError(''); } catch (value) { onError(value instanceof Error ? value.message : '知识包创建失败'); } finally { setBusy(false); }
  };
  const publish = async (packID: string) => { setBusy(true); try { await post(`/api/bff/knowledge-packs/${encodeURIComponent(packID)}/publish`, {}); await onChanged(); onError(''); } catch (value) { onError(value instanceof Error ? value.message : '知识包发布失败'); } finally { setBusy(false); } };
  return <div className="knowledge-subview-grid"><section className="workos-section"><SectionHeader kicker="知识包" title="业务用途绑定" action={<span className="section-count">{packs.length} 个包</span>}/><div className="pack-create-form"><Field label="名称"><input value={name} onChange={event => setName(event.target.value)}/></Field><Field label="用途"><input value={purpose} onChange={event => setPurpose(event.target.value)}/></Field><span className="pack-selection-note">将固定 {eligible.length} 个已验证对象版本</span><Button onClick={create} disabled={busy || eligible.length === 0}><Plus size={15}/>创建草稿</Button></div><div className="pack-list">{packs.length === 0 ? <Empty title="还没有知识包" detail="先完成对象治理，再按业务用途创建可发布知识包。"/> : packs.map(pack => <article key={pack.id}><div><strong>{pack.name}</strong><span>{pack.purpose || '未填写用途'} · {pack.object_refs.length} 个对象 · v{pack.version}</span></div><StatusText value={pack.status}/>{pack.status === 'draft' && <Button variant="secondary" disabled={busy} onClick={() => publish(pack.id)}><PackageCheck size={14}/>发布</Button>}<ChevronRight size={15}/></article>)}</div></section><section className="workos-section"><SectionHeader kicker="不可变快照" title="发布后生成"/><div className="snapshot-list">{Object.keys(snapshots).length === 0 ? <Empty title="还没有知识快照" detail="发布知识包后，服务端会生成不可变快照供 TaskRun 显式绑定。"/> : Object.entries(snapshots).flatMap(([packID, values]) => values.map(snapshot => <article className="snapshot-row" key={snapshot.id}><div><strong>{snapshot.id.slice(0, 14)}…</strong><span>Pack {packID.slice(0, 14)}… · {snapshot.objects.length} 个对象</span></div><code>{snapshot.digest.slice(0, 18)}…</code></article>))}</div></section></div>;
}
function KnowledgeQuery({projectID, packs, query, setQuery, onNotice, onError}: {projectID: string; packs: KnowledgePack[]; query: string; setQuery: (value: string) => void; onNotice: (value: string) => void; onError: (value: string) => void}) {
  const publishedPacks = useMemo(() => packs.filter(pack => pack.status === 'published'), [packs]);
  const [packID, setPackID] = useState('');
  const [channel, setChannel] = useState('short_video');
  const [result, setResult] = useState<KnowledgeQueryResult>();
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (publishedPacks.length === 0) {
      setPackID('');
      setResult(undefined);
      return;
    }
    if (!publishedPacks.some(pack => pack.id === packID)) setPackID(publishedPacks[0].id);
  }, [packID, publishedPacks]);

  const run = async () => {
    if (!packID) return;
    const objectTypes = query.split(/[\n,，]/).map(value => value.trim()).filter(Boolean);
    setBusy(true);
    onError('');
    try {
      const value = await post<KnowledgeQueryResult>('/api/bff/knowledge/query', {project_id: projectID, pack_id: packID, channel, object_types: objectTypes, at: new Date().toISOString()});
      setResult(value);
      onNotice(`查询完成：${value.eligible.length} 个可用对象，${value.blocked.length} 个被阻断，${value.gaps.length} 个缺口。`);
    } catch (value) {
      onError(value instanceof Error ? value.message : '知识查询失败');
    } finally {
      setBusy(false);
    }
  };

  if (publishedPacks.length === 0) return <div className="query-empty"><BookOpen size={22}/><strong>先发布一个知识包</strong><span>确定性查询只读取不可变快照，不直接读取候选对象。</span></div>;
  return <div className="knowledge-query-layout"><section className="workos-section"><SectionHeader kicker="查询范围" title="从不可变快照读取"/><Field label="知识包"><select value={packID} onChange={event => {setPackID(event.target.value); setResult(undefined);}}>{publishedPacks.map(pack => <option key={pack.id} value={pack.id}>{pack.name} · v{pack.version}</option>)}</select></Field><Field label="渠道"><select value={channel} onChange={event => setChannel(event.target.value)}><option value="short_video">短视频</option><option value="wechat_article">公众号文章</option><option value="">不限制渠道</option></select></Field><Field label="对象类型（可选）"><input value={query} onChange={event => setQuery(event.target.value)} placeholder="例如：Claim, BrandRule"/><small className="field-hint">多个类型用逗号分隔；留空表示查询整个快照。</small></Field><Button onClick={run} disabled={busy || !packID}><Search size={15}/>{busy ? '查询中...' : '运行确定性查询'}</Button></section><section className="workos-section query-result"><SectionHeader kicker="查询结果" title={result ? '事实可用性分层' : '尚未运行查询'} action={result ? <span className="query-digest">{result.query_digest}</span> : undefined}/>{!result ? <div className="query-empty"><Search size={22}/><strong>选择范围后运行查询</strong><span>结果只来自已发布快照，查询本身不会改变任何知识对象。</span></div> : <><QueryResultRow label="eligible" value={String(result.eligible.length)} detail="满足状态、Evidence、冲突和权利约束" tone="is-success"/><QueryResultRow label="blocked" value={String(result.blocked.length)} detail="存在阻断原因，不得直接作为内容事实" tone="is-danger"/><QueryResultRow label="gaps" value={String(result.gaps.length)} detail="需要创建补料任务或补充知识" tone="is-warning"/>{result.blocked.length > 0 && <div className="query-detail-list">{result.blocked.slice(0, 5).map(item => <div key={item.object_id}><strong>{item.object_id}</strong><span>{item.reasons?.join('、') || '存在约束未满足'}</span></div>)}</div>}{result.gaps.length > 0 && <div className="query-detail-list">{result.gaps.slice(0, 5).map(item => <div key={item.object_id}><strong>{item.object_id}</strong><span>{item.next_action}</span></div>)}</div>}<button className="text-action" onClick={() => setResult(undefined)}>清除本次结果 <X size={14}/></button></>}</section></div>;
}

function KnowledgeDrawer({value, onClose, onAccept, onEvidence, onTask}: {value: KnowledgeObject; onClose: () => void; onAccept: () => void; onEvidence: () => void; onTask: () => void}) { return <Modal title="知识对象" onClose={onClose}><div className="knowledge-drawer-content"><div className="drawer-object-heading"><span className="object-mark is-knowledge"><BookOpen size={16}/></span><div><strong>{value.title}</strong><span><code>{value.id}</code> · v{value.version}</span></div><StatusText value={value.status}/></div><dl className="object-facts"><div><dt>对象类型</dt><dd>{value.object_type}</dd></div><div><dt>层级</dt><dd>{value.layer}</dd></div><div><dt>Evidence</dt><dd>{value.evidence_refs.length ? value.evidence_refs.join('、') : '缺少来源'}</dd></div><div><dt>影响</dt><dd>{value.impact || '未标记'}</dd></div></dl><section className="drawer-copy"><span>陈述</span><p>{value.statement}</p></section><section className="drawer-copy"><span>确定性约束</span><p>接受只生成新版本；已绑定历史快照的 TaskRun 不会被静默替换。</p></section><div className="modal-actions"><Button variant="secondary" onClick={onEvidence}><AlertCircle size={15}/>要求补证据</Button><Button variant="secondary" onClick={onTask}><ListTodo size={15}/>创建补料任务</Button><Button onClick={onAccept}><Check size={15}/>接受为知识</Button></div></div></Modal>; }

function PageHeader({eyebrow, title, description, actions}: {eyebrow: string; title: string; description: string; actions?: React.ReactNode}) { return <header className="workos-page-header"><div><span className="eyebrow">{eyebrow}</span><h1>{title}</h1><p>{description}</p></div>{actions && <div className="workos-header-actions">{actions}</div>}</header>; }
function SectionHeader({kicker, title, action}: {kicker: string; title: string; action?: React.ReactNode}) { return <header className="workos-section-header"><div><span>{kicker}</span><h2>{title}</h2></div>{action}</header>; }
function Metric({icon: Icon, label, value, detail, tone}: {icon: typeof ListTodo; label: string; value: string; detail: string; tone: string}) { return <article className={`workos-metric is-${tone}`}><Icon size={16}/><span>{label}</span><strong>{value}</strong><small>{detail}</small></article>; }
function KnowledgeMetric({label, value, detail, tone = 'default'}: {label: string; value: string; detail: string; tone?: string}) { return <article className={`knowledge-metric is-${tone}`}><span>{label}</span><strong>{value}</strong><small>{detail}</small></article>; }
function TaskRow({task, onClick}: {task: ApiWorkTask; onClick?: () => void}) { const navigate = useNavigate(); const status = task.status as TaskStatus; const tone = status === 'blocked' ? 'review' : status === 'delivered' ? 'success' : 'production'; const open = () => task.project_id ? navigate(taskPath(task)) : onClick?.(); return <button className="task-queue-row" onClick={open}><span className={`object-mark is-${tone}`}><ListTodo size={15}/></span><span className="task-row-copy"><strong>{task.title}</strong><small>{task.sop_id} v{task.sop_version} · {task.current_stage_id || '未开始'}</small></span><span className="task-row-next"><small>下一动作</small><strong>{task.next_action || '等待客户端继续'}</strong></span><StatusText value={status}/><ChevronRight size={15}/></button>; }
function StatusText({value}: {value: string}) { const label = statusLabel[value as TaskStatus] || knowledgeStatusLabel[value] || inputStatusLabel[value] || value; const tone = value === 'delivered' || value === 'approved' || value === 'project_material' || value === 'task_created' || value === 'task_merged' || value === '已确认' ? 'is-success' : value === 'blocked' || value === 'conflicted' || value === 'archived' ? 'is-danger' : value === 'waiting_gate' || value === 'needs_review' || value === 'candidate' || value === 'open' || value === 'needs_info' || value === 'untriaged' || value === '待审核' || value === '需留意' ? 'is-warning' : value === 'running' || value === 'routed' ? 'is-production' : 'is-muted'; return <span className={`status-text ${tone}`}><i></i>{label}</span>; }
function CheckLine({label}: {label: string}) { return <div className="check-line"><CheckCircle2 size={15}/><span>{label}</span></div>; }
function EvidenceRow({label, detail, status, icon: Icon}: {label: string; detail: string; status: string; icon: typeof FileInput}) { return <div className="evidence-row"><span className="object-mark is-source"><Icon size={15}/></span><div><strong>{label}</strong><small>{detail}</small></div><span className="evidence-status">{status}</span><ChevronRight size={14}/></div>; }
function SideFact({label, value, icon: Icon}: {label: string; value?: string; icon?: typeof UserRound; children?: React.ReactNode}) { return <div className="side-fact"><span>{label}</span><strong>{Icon && <Icon size={14}/>} {value}</strong></div>; }
function QueryResultRow({label, value, detail, tone}: {label: string; value: string; detail: string; tone: string}) { return <div className="query-result-row"><span className={tone}><i></i>{label}</span><strong>{value}</strong><small>{detail}</small></div>; }
function formatDateTime(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? '未知时间' : new Intl.DateTimeFormat('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'}).format(date); }
