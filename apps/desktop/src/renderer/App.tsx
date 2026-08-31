import { useEffect, useMemo, useState } from 'react';
import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Activity,
  AlertTriangle,
  ArrowUpRight,
  ArrowRightLeft,
  Archive,
  BookOpenCheck,
  CheckCircle2,
  ChevronRight,
  CircleDashed,
  ClipboardCheck,
  Cloud,
  CloudOff,
  Download,
  FileText,
  FolderTree,
  Eye,
  GitPullRequest,
  History,
  KeyRound,
  LayoutDashboard,
  LogIn,
  LogOut,
  Plus,
  PackageCheck,
  Power,
  RefreshCw,
  RotateCcw,
  Send,
  Server,
  ShieldCheck,
  Square,
  UploadCloud,
  UserRound,
  WifiOff,
  X,
} from 'lucide-react';

import type { DesktopAuthResult, DesktopCommandResult, DesktopCreateServerProjectInput, DesktopDaemonStatus, DesktopDeliveryDownloadResult, DesktopEvent, DesktopExperienceProjection, DesktopFileSelection, DesktopMaterialUploadResult, DesktopReviewAction, DesktopReviewRevisionDetail, DesktopServerBootstrap, DesktopServerConnectSession, DesktopServerProject, DesktopServerResult, DesktopSnapshot, DesktopSnapshotResult, ProjectSnapshot } from '../shared/contracts';

type View = 'overview' | 'review' | 'transfers' | 'runs' | 'experience' | 'delivery';

const viewItems: Array<{ id: View; label: string; icon: typeof LayoutDashboard }> = [
  { id: 'overview', label: '内容目录', icon: FolderTree },
  { id: 'transfers', label: '同步与上传', icon: UploadCloud },
  { id: 'review', label: '审批收件箱', icon: ClipboardCheck },
  { id: 'runs', label: '任务运行', icon: Activity },
  { id: 'experience', label: '经验演化', icon: BookOpenCheck },
  { id: 'delivery', label: '交付状态', icon: PackageCheck },
];

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

export function DesktopApp() {
  return <QueryClientProvider client={queryClient}><DesktopWorkspace /></QueryClientProvider>;
}

function DesktopWorkspace() {
	const [selectedProjectID, setSelectedProjectID] = useState<string>();
	const [view, setView] = useState<View>('overview');
	const [authOpen, setAuthOpen] = useState(false);
	const [createProjectOpen, setCreateProjectOpen] = useState(false);
	const cache = useQueryClient();
  const query = useQuery({
    queryKey: ['desktop-snapshot'],
    queryFn: () => window.contentcloudDesktop.getSnapshot(),
    refetchInterval: false,
	  });
	const authQuery = useQuery({
	  queryKey: ['desktop-auth'],
	  queryFn: () => window.contentcloudDesktop.getAuthSession(),
	  refetchInterval: 60_000,
	});
	const daemonQuery = useQuery({
	  queryKey: ['desktop-daemon'],
	  queryFn: () => window.contentcloudDesktop.getDaemonStatus(),
	  refetchInterval: 3_000,
	});
	const daemonAction = useMutation({
	  mutationFn: (action: 'start' | 'stop' | 'restart') => action === 'start'
	    ? window.contentcloudDesktop.startDaemon()
	    : action === 'stop' ? window.contentcloudDesktop.stopDaemon() : window.contentcloudDesktop.restartDaemon(),
	  onSuccess: (result) => {
	    cache.setQueryData(['desktop-daemon'], result);
	    if (result.state === 'running' || result.state === 'stopped') void query.refetch();
	  },
	});
	const authResult = authQuery.data;
	const authSession = authResult?.status === 'authenticated' ? authResult.session : undefined;
	const serverQuery = useQuery({
	  queryKey: ['server-bootstrap', authSession?.server_url],
	  queryFn: () => window.contentcloudDesktop.getServerBootstrap(),
	  enabled: Boolean(authSession),
	  refetchInterval: 30_000,
	});
	const publish = useMutation({
		mutationFn: (target: ProjectSnapshot) => window.contentcloudDesktop.publishWorkspace({
			workspace_id: target.workspace_id,
			project_id: target.project_id,
			base_revision: target.cloud_revision,
			observed_digest: target.observed_digest ?? '',
		}),
	});
	const retry = useMutation({
		mutationFn: (target: ProjectSnapshot) => window.contentcloudDesktop.retryWorkspace({ workspace_id: target.workspace_id, project_id: target.project_id }),
		onSuccess: (result) => { if (result.status === 'accepted') void query.refetch(); },
	});

	useEffect(() => window.contentcloudDesktop.onSnapshotChanged((result) => {
		cache.setQueryData(['desktop-snapshot'], result);
		if (result.status === 'ready' && !selectedProjectID) setSelectedProjectID(result.snapshot.projects[0]?.project_id);
	}), [cache, selectedProjectID]);

	const state = query.data;
  const snapshot = state?.status === 'ready' ? state.snapshot : undefined;
  const project = useMemo(() => selectProject(snapshot, selectedProjectID), [snapshot, selectedProjectID]);
  const serverBootstrap = serverQuery.data?.status === 'ready' ? serverQuery.data.value : undefined;
  const cloudProject = useMemo(() => selectCloudProject(serverBootstrap, selectedProjectID), [serverBootstrap, selectedProjectID]);
  const refresh = () => query.refetch().catch(() => undefined);
	const signOut = async () => {
	  await window.contentcloudDesktop.logout();
	  cache.removeQueries({ queryKey: ['server-bootstrap'] });
	  await authQuery.refetch();
	};
	const publishProject = () => {
		if (project?.observed_digest) publish.mutate(project);
	};

	useEffect(() => {
	  if (!selectedProjectID && (snapshot?.projects[0]?.project_id || serverBootstrap?.projects[0]?.id)) {
	    setSelectedProjectID(snapshot?.projects[0]?.project_id ?? serverBootstrap?.projects[0]?.id);
	  }
	}, [selectedProjectID, snapshot, serverBootstrap]);
	useEffect(() => {
	  if (serverQuery.data?.status === 'unauthenticated') void authQuery.refetch();
	}, [serverQuery.data?.status, authQuery.refetch]);
	useEffect(() => {
	  if (authResult?.status !== 'authenticated') cache.removeQueries({ queryKey: ['server-bootstrap'] });
	}, [authResult?.status, cache]);

  return (
    <div className="desktop-shell">
      <aside className="sidebar">
        <div className="brand-lockup">
          <span className="brand-mark" aria-hidden="true"><span /><span /><span /><span /></span>
          <span>Content Work OS</span>
        </div>
        <div className="sidebar-label">项目工作面</div>
        <nav className="surface-nav" aria-label="项目工作面">
          {viewItems.map(({ id, label, icon: Icon }) => (
            <button key={id} className={view === id ? 'nav-item active' : 'nav-item'} onClick={() => setView(id)} type="button">
              <Icon size={16} strokeWidth={1.75} />
              <span>{label}</span>
              {id === 'review' && project && project.pending_feedback + project.pending_decision > 0 ? <strong>{project.pending_feedback + project.pending_decision}</strong> : null}
            </button>
          ))}
        </nav>
        <div className="sidebar-footer">
          <div className="sidebar-label">连接</div>
          <DaemonControl result={daemonQuery.data} busy={daemonAction.isPending} onAction={(action) => daemonAction.mutate(action)} />
          <AuthBadge result={authResult} onClick={() => setAuthOpen(true)} />
        </div>
      </aside>

      <main className="workspace-surface">
        <header className="topbar">
          <div>
            <div className="eyebrow">持续项目工作面</div>
            <h1>{project?.name ?? 'Content Work OS Desktop'}</h1>
          </div>
          <div className="topbar-actions">
            <span className="version-label">{daemonQuery.data?.version ? `Daemon ${daemonQuery.data.version}` : daemonLabel(daemonQuery.data)}</span>
            {authSession ? <button className="account-button" onClick={signOut} type="button" title="退出服务端会话"><span className="account-avatar"><UserRound size={14} /></span><span>{authSession.user.display_name || authSession.user.email}</span><LogOut size={14} /></button> : <button className="login-button" onClick={() => setAuthOpen(true)} type="button"><LogIn size={15} />登录服务端</button>}
            <button className="icon-button" onClick={refresh} type="button" aria-label="刷新项目状态" title="刷新项目状态">
              <RefreshCw size={17} className={query.isFetching ? 'spin' : undefined} />
            </button>
          </div>
        </header>

        {state?.status === 'offline' ? <OfflineState message={state.message} onRetry={refresh} onLogin={() => setAuthOpen(true)} /> : null}
        {state?.status === 'ready' && snapshot ? <>
          <ProjectPicker projects={snapshot.projects} cloudProjects={serverBootstrap?.projects ?? []} canCreate={serverBootstrap?.session.can_create ?? false} selectedProjectID={selectedProjectID} onChange={setSelectedProjectID} onCreate={() => setCreateProjectOpen(true)} />
          <CloudSyncStrip authResult={authResult} serverResult={serverQuery.data} bootstrap={serverBootstrap} />
          {project ? <ViewContent project={project} view={view} onPublish={publishProject} onRetry={() => retry.mutate(project)} publishing={publish.isPending} retrying={retry.isPending} publishResult={publish.data ?? retry.data} /> : cloudProject ? <CloudProjectPanel project={cloudProject} canConnect={serverBootstrap?.session.can_connect_execution_client ?? false} /> : <EmptyProjectState />}
        </> : null}
        {state?.status === 'offline' && cloudProject ? <CloudProjectPanel project={cloudProject} canConnect={serverBootstrap?.session.can_connect_execution_client ?? false} /> : null}
        {!state && query.isLoading ? <LoadingState /> : null}
      </main>
      {authOpen ? <AuthDialog initial={authSession?.server_url} onClose={() => setAuthOpen(false)} onAuthenticated={async () => { await authQuery.refetch(); setAuthOpen(false); }} /> : null}
      {createProjectOpen && authSession ? <CreateProjectDialog existingProjectIDs={new Set(serverBootstrap?.projects.map((project) => project.id))} onClose={() => setCreateProjectOpen(false)} onCreated={(result, projectID) => {
        if (result.status === 'ready') {
          cache.setQueryData(['server-bootstrap', authSession.server_url], result.value);
          setSelectedProjectID(projectID ?? result.value.projects[0]?.id);
        }
        setCreateProjectOpen(false);
      }} /> : null}
    </div>
  );
}

function selectProject(snapshot: DesktopSnapshot | undefined, selectedProjectID: string | undefined): ProjectSnapshot | undefined {
  if (!snapshot?.projects.length) return undefined;
  if (selectedProjectID) return snapshot.projects.find((item) => item.project_id === selectedProjectID);
  return snapshot.projects[0];
}

function selectCloudProject(bootstrap: DesktopServerBootstrap | undefined, selectedProjectID: string | undefined): DesktopServerProject | undefined {
  if (!bootstrap?.projects.length) return undefined;
  return bootstrap.projects.find((item) => item.id === selectedProjectID) ?? bootstrap.projects[0];
}

function ProjectPicker({ projects, cloudProjects, canCreate, selectedProjectID, onChange, onCreate }: { projects: ProjectSnapshot[]; cloudProjects: DesktopServerProject[]; canCreate: boolean; selectedProjectID?: string; onChange: (value: string) => void; onCreate: () => void }) {
  return <div className="project-picker">
    <div className="project-picker-leading"><FolderTree size={17} /><span>项目工作区</span></div>
    <select value={selectedProjectID ?? projects[0]?.project_id ?? cloudProjects[0]?.id ?? ''} onChange={(event) => onChange(event.target.value)} aria-label="选择项目">
      {projects.length ? <optgroup label="本地 Workspace">{projects.map((project) => <option key={`local-${project.project_id}`} value={project.project_id}>{project.name} · 已绑定</option>)}</optgroup> : null}
      {cloudProjects.length ? <optgroup label="云端项目">{cloudProjects.map((project) => <option key={`cloud-${project.id}`} value={project.id}>{project.brand_name} · {project.product_name}</option>)}</optgroup> : null}
    </select>
    <span className="picker-note">{projects.length} 个本地 · {cloudProjects.length} 个云端</span>
    <button className="secondary-button project-create-button" type="button" onClick={onCreate} disabled={!canCreate}><Plus size={15} />新建项目</button>
  </div>;
}

function CloudSyncStrip({ authResult, serverResult, bootstrap }: { authResult: DesktopAuthResult | undefined; serverResult: DesktopServerResult<DesktopServerBootstrap> | undefined; bootstrap?: DesktopServerBootstrap }) {
  if (authResult?.status !== 'authenticated') return <div className="cloud-strip muted"><CloudOff size={15} /><span>登录服务端后可读取云端项目和连接状态</span></div>;
  if (serverResult?.status === 'unauthenticated') return <div className="cloud-strip warning"><KeyRound size={15} /><span>服务端会话已失效，请重新登录</span></div>;
  if (serverResult?.status === 'offline') return <div className="cloud-strip warning"><CloudOff size={15} /><span>云端项目暂不可用：{serverResult.message}</span></div>;
  if (serverResult?.status === 'rejected') return <div className="cloud-strip warning"><AlertTriangle size={15} /><span>云端读取被拒绝：{serverResult.message}</span></div>;
  if (!bootstrap) return <div className="cloud-strip muted"><RefreshCw size={15} className="spin" /><span>正在读取云端项目…</span></div>;
  return <div className="cloud-strip ready"><Cloud size={15} /><span>已连接 {bootstrap.session.tenant.name}</span><span className="cloud-strip-separator" /><span>{bootstrap.projects.length} 个云端项目</span><span className="cloud-strip-separator" /><span>账号权限：{bootstrap.session.role}</span></div>;
}

const terminalConnectStatuses = new Set(['connected', 'expired', 'canceled', 'failed']);

function CloudProjectPanel({ project, canConnect }: { project: DesktopServerProject; canConnect: boolean }) {
  const [sessionID, setSessionID] = useState<string>();
  const create = useMutation({
    mutationFn: () => window.contentcloudDesktop.createConnectSession(project.id),
    onSuccess: (result) => {
      if (result.status === 'ready') setSessionID(result.value.id);
    },
  });
  const status = useQuery({
    queryKey: ['desktop-connect-session', sessionID],
    queryFn: () => window.contentcloudDesktop.getConnectSession(sessionID!),
    enabled: Boolean(sessionID),
    refetchInterval: sessionID ? 2500 : false,
  });
  const cancel = useMutation({
    mutationFn: () => window.contentcloudDesktop.cancelConnectSession(sessionID!),
    onSuccess: (result) => {
      if (result.status === 'ready') void status.refetch();
    },
  });
  const connection = status.data?.status === 'ready' ? status.data.value : undefined;
  const sessionTerminal = Boolean(connection && terminalConnectStatuses.has(connection.status));
  useEffect(() => setSessionID(undefined), [project.id]);
  const copySession = () => {
    if (connection?.verification_code) void navigator.clipboard?.writeText(connection.verification_code);
  };

  return <section className="cloud-project-panel">
    <div className="surface-view-heading"><div><div className="eyebrow">Cloud project</div><h2>{project.brand_name} · {project.product_name}</h2><p className="section-description">{project.content_type} · {project.channel || '未设置渠道'} · {project.status}</p></div><span className={`state-pill ${project.execution_client_connected ? 'clean' : 'modified'}`}><span />{project.execution_client_connected ? `${project.connected_client_count} 台设备在线` : '尚未绑定设备'}</span></div>
    <div className="cloud-project-facts"><Metric label="项目 ID" value={project.id} /><Metric label="本地 Workspace" value="未发现" tone="warning" /><Metric label="云端设备" value={String(project.connected_client_count)} tone={project.execution_client_connected ? 'success' : 'neutral'} /></div>
    {!canConnect ? <div className="connect-callout warning"><ShieldCheck size={17} /><div><strong>当前账号没有设备连接权限</strong><span>请让项目负责人或租户管理员在服务端开通连接权限。</span></div></div> : null}
    {canConnect && (!connection || (sessionTerminal && connection.status !== 'connected')) ? <div className="connect-callout"><ArrowRightLeft size={17} /><div><strong>{connection ? '重新发起设备绑定' : '把这台电脑加入项目'}</strong><span>创建一次性连接会话，然后在本机终端运行 bootstrap 完成设备授权和 Workspace 初始化。</span></div><button className="command-button" type="button" onClick={() => { setSessionID(undefined); create.mutate(); }} disabled={create.isPending}><KeyRound size={15} />{create.isPending ? '正在创建' : connection ? '重新绑定' : '开始绑定'}</button></div> : null}
    {create.data?.status === 'rejected' ? <p className="operation-message danger"><AlertTriangle size={14} />{create.data.message}</p> : null}
    {create.data?.status === 'offline' ? <p className="operation-message warning"><CloudOff size={14} />{create.data.message}</p> : null}
    {connection ? <div className="connect-session-card">
      <div className="connect-session-heading"><div><span className="section-ref">CONNECT SESSION</span><h3>{connection.message}</h3></div><span className={`state-pill ${connection.status === 'connected' ? 'clean' : connection.status === 'failed' || connection.status === 'expired' || connection.status === 'canceled' ? 'conflict' : 'modified'}`}><span />{connectStatusLabel(connection.status)}</span></div>
      <div className="connect-session-meta"><Metric label="会话 ID" value={connection.id} /><Metric label="支持编号" value={connection.support_code ?? '等待生成'} /><Metric label="有效期" value={formatDate(connection.expires_at)} /></div>
      {connection.requires_confirmation ? <div className="verification-code"><div><span>确认码</span><strong>{connection.verification_code ?? '等待本机生成'}</strong></div>{connection.verification_code ? <button className="icon-button" type="button" onClick={copySession} aria-label="复制确认码" title="复制确认码"><ClipboardCheck size={16} /></button> : null}</div> : null}
      {connection.status === 'waiting_for_computer' || connection.status === 'connecting' || connection.status === 'confirmation_required' ? <p className="connect-session-help">在本机终端先运行 <code>contentcloud bootstrap plan &lt;目录&gt; --session {connection.id}</code> 获取 plan_id，再运行 <code>contentcloud bootstrap apply &lt;目录&gt; --session {connection.id} --plan-id &lt;plan_id&gt; --accept</code> 完成设备授权、Workspace 注册和 Daemon 启动。完成后此页面会自动切换到本地 Workspace。</p> : null}
      <div className="connect-session-actions">{!sessionTerminal ? <button className="secondary-button" type="button" onClick={() => cancel.mutate()} disabled={cancel.isPending}><X size={15} />取消会话</button> : null}<button className="secondary-button" type="button" onClick={() => void status.refetch()} disabled={status.isFetching}><RefreshCw size={15} className={status.isFetching ? 'spin' : undefined} />刷新状态</button></div>
    </div> : null}
  </section>;
}

function connectStatusLabel(status: string): string {
  const labels: Record<string, string> = { waiting_for_computer: '等待本机', confirmation_required: '等待确认', connecting: '连接中', connected: '已连接', expired: '已过期', canceled: '已取消', failed: '失败' };
  return labels[status] ?? status;
}

function ViewContent({ project, view, onPublish, onRetry, publishing, retrying, publishResult }: { project: ProjectSnapshot; view: View; onPublish: () => void; onRetry: () => void; publishing: boolean; retrying: boolean; publishResult?: DesktopCommandResult }) {
  switch (view) {
    case 'transfers': return <TransferView project={project} onPublish={onPublish} onRetry={onRetry} publishing={publishing} retrying={retrying} result={publishResult} />;
    case 'review': return <ReviewView project={project} />;
    case 'runs': return <RuntimeView project={project} />;
    case 'experience': return <ExperienceView project={project} />;
    case 'delivery': return <DeliveryView project={project} />;
    default: return <ContentDirectory project={project} />;
  }
}

function TransferView({ project, onPublish, onRetry, publishing, retrying, result }: { project: ProjectSnapshot; onPublish: () => void; onRetry: () => void; publishing: boolean; retrying: boolean; result?: DesktopCommandResult }) {
  const cache = useQueryClient();
  const [selectedFiles, setSelectedFiles] = useState<DesktopFileSelection[]>([]);
  const [selectionMessage, setSelectionMessage] = useState('');
  const [uploadResults, setUploadResults] = useState<DesktopMaterialUploadResult[]>([]);
  const [preview, setPreview] = useState<{ data_url: string; mime_type: string; file_name: string }>();
  const [previewingID, setPreviewingID] = useState<string>();
  const assets = useQuery({
    queryKey: ['desktop-assets', project.project_id],
    queryFn: () => window.contentcloudDesktop.getAssets(project.project_id),
    staleTime: 10_000,
    refetchInterval: 30_000,
  });
  const upload = useMutation({
    mutationFn: (fileIDs: string[]) => window.contentcloudDesktop.uploadMaterials(project.project_id, fileIDs),
    onSuccess: (results) => {
      setUploadResults(results);
      const uploaded = new Set(results.filter((item) => item.status === 'uploaded').map((item) => item.file_id));
      setSelectedFiles((files) => files.filter((file) => !uploaded.has(file.id)));
      void assets.refetch();
      void cache.invalidateQueries({ queryKey: ['server-bootstrap'] });
    },
  });
  const selectFiles = async () => {
    setSelectionMessage('');
    const selection = await window.contentcloudDesktop.chooseFiles();
    if (selection.status === 'selected') setSelectedFiles((files) => [...files, ...selection.files.filter((file) => !files.some((current) => current.id === file.id))]);
    else if (selection.status === 'rejected') setSelectionMessage(selection.message ?? '没有可上传的文件');
  };
  const openPreview = async (material: { ref: string }) => {
    setPreviewingID(material.ref);
    const response = await window.contentcloudDesktop.previewMaterial(material.ref);
    setPreviewingID(undefined);
    if (response.status === 'ready') setPreview({ data_url: response.data_url, mime_type: response.mime_type, file_name: response.file_name });
    else setSelectionMessage(response.message);
  };
  const allowed = project.allowed_actions.includes('workspace.publish') && Boolean(project.observed_digest);
  const retryAllowed = project.allowed_actions.includes('workspace.retry');
  const eventQuery = useQuery({
    queryKey: ['desktop-project-events', project.project_id, project.event_cursor],
    queryFn: () => window.contentcloudDesktop.getProjectEvents(project.project_id, Math.max(0, project.event_cursor - 12)),
    staleTime: 5000,
  });
  const eventStream = eventQuery.data?.status === 'ready' ? eventQuery.data.stream : undefined;
  const cloudMaterials = assets.data?.status === 'ready' ? assets.data.value.workspace.materials : [];
  return <section className="transfer-surface">
    <div className="surface-view-heading"><div><div className="eyebrow">Local Sync Engine</div><h2>同步与上传</h2><p className="section-description">所有变更先在本地计算摘要，再以不可变 Revision 进入云端队列</p></div><StatePill state={project.local_state} /></div>
    <div className="sync-compare"><div><span>本地 Revision</span><strong>R{project.local_revision}</strong><small>{project.local_state === 'clean' ? '工作区已核对' : '存在待发布变更'}</small></div><ArrowRightLeft size={18} /><div><span>Cloud Revision</span><strong>{project.cloud_revision}</strong><small>事件游标 {project.cloud_event_cursor}</small></div></div>
    <div className="transfer-metrics"><Metric label="传输状态" value={project.transfer_state} tone={project.transfer_state === 'idle' ? 'success' : 'warning'} /><Metric label="最近同步" value={project.last_synced_at ? formatDate(project.last_synced_at) : '尚未同步'} /><Metric label="内容摘要" value={project.observed_digest ? project.observed_digest.slice(0, 18) + '…' : '等待计算'} /></div>
    <div className="transfer-actions"><button className="command-button" type="button" onClick={onPublish} disabled={!allowed || publishing}><UploadCloud size={15} />{publishing ? '正在排队' : project.transfer_state === 'queued' ? '已进入同步队列' : '发布本地 Revision'}</button>{retryAllowed ? <button className="secondary-button" type="button" onClick={onRetry} disabled={retrying}><RefreshCw size={15} className={retrying ? 'spin' : undefined} />{retrying ? '正在重试' : '重试失败同步'}</button> : null}{result ? <OperationResult result={result} /> : null}</div>
    <section className="material-upload-panel">
      <div className="section-heading"><div><div className="eyebrow">Cloud materials</div><h3>批量上传项目资料</h3><p className="section-description">支持文档、图片、视频和音频；文件先上传到当前云端项目，再由服务端处理。</p></div><button className="secondary-button" type="button" onClick={selectFiles} disabled={upload.isPending}><Plus size={15} />选择文件</button></div>
      {selectionMessage ? <p className="operation-message warning"><AlertTriangle size={14} />{selectionMessage}</p> : null}
      {selectedFiles.length ? <div className="upload-queue">{selectedFiles.map((file) => <div className="upload-queue-item" key={file.id}><FileIcon kind={file.mime_type} /><div><strong>{file.name}</strong><small>{formatBytes(file.size)} · {file.mime_type}</small></div><button className="icon-button" type="button" aria-label={`移除 ${file.name}`} title="移除" onClick={() => setSelectedFiles((files) => files.filter((item) => item.id !== file.id))}><X size={14} /></button></div>)}</div> : <div className="empty-line"><CircleDashed size={15} /><span>尚未选择文件</span></div>}
      <div className="upload-queue-actions"><button className="command-button" type="button" onClick={() => upload.mutate(selectedFiles.map((file) => file.id))} disabled={!selectedFiles.length || upload.isPending}>{upload.isPending ? <RefreshCw size={15} className="spin" /> : <UploadCloud size={15} />}{upload.isPending ? '正在上传' : selectedFiles.length ? `上传 ${selectedFiles.length} 个文件` : '上传文件'}</button>{upload.isError ? <span className="operation-message danger"><AlertTriangle size={14} />上传队列异常，请重试</span> : null}</div>
      {uploadResults.length ? <div className="upload-results">{uploadResults.map((item) => <div className={`upload-result ${item.status}`} key={item.file_id}><span>{item.status === 'uploaded' ? <CheckCircle2 size={14} /> : <AlertTriangle size={14} />}</span><div><strong>{item.file_name}</strong><small>{item.status === 'uploaded' ? '已进入云端资料库' : item.message ?? '上传失败'}</small></div>{item.status !== 'uploaded' ? <button className="secondary-button" type="button" onClick={() => { const file = selectedFiles.find((value) => value.id === item.file_id); if (file) upload.mutate([file.id]); }} disabled={upload.isPending}><RefreshCw size={14} />重试</button> : null}</div>)}</div> : null}
    </section>
    <section className="cloud-materials-panel"><div className="section-heading"><div><div className="eyebrow">Server projection</div><h3>云端项目资料</h3></div><span className="item-count">{cloudMaterials.length}</span></div>{assets.data?.status === 'offline' ? <p className="operation-message warning"><CloudOff size={14} />{assets.data.message}</p> : assets.data?.status === 'unauthenticated' ? <p className="operation-message warning"><KeyRound size={14} />登录服务端后查看云端资料</p> : cloudMaterials.length ? <div className="cloud-material-grid">{cloudMaterials.map((material) => <article className="cloud-material" key={material.ref}><FileIcon kind={material.mime_type} /><div><strong>{material.title || material.file_name}</strong><small>{material.file_name} · {formatBytes(material.byte_size)}</small></div><span className={`material-processing ${material.processing_state}`}>{material.processing_state}</span>{isPreviewable(material.mime_type) ? <button className="icon-button" type="button" onClick={() => void openPreview(material)} disabled={previewingID === material.ref} aria-label={`预览 ${material.file_name}`} title="预览"><Eye size={14} className={previewingID === material.ref ? 'spin' : undefined} /></button> : null}</article>)}</div> : <div className="empty-line"><Archive size={15} /><span>当前项目还没有云端资料</span></div>}</section>
    <SyncActivity events={eventStream?.events ?? []} gap={eventStream?.gap || eventStream?.resync_required || false} loading={eventQuery.isLoading} />
    {preview ? <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.currentTarget === event.target) setPreview(undefined); }}><section className="material-preview-dialog" role="dialog" aria-modal="true" aria-label={`预览 ${preview.file_name}`}><header className="auth-dialog-header"><div><div className="eyebrow">Material preview</div><h3>{preview.file_name}</h3></div><button className="icon-button" type="button" onClick={() => setPreview(undefined)} aria-label="关闭预览" title="关闭"><X size={16} /></button></header>{preview.mime_type.startsWith('image/') ? <img src={preview.data_url} alt={preview.file_name} /> : preview.mime_type.startsWith('video/') ? <video src={preview.data_url} controls /> : <audio src={preview.data_url} controls />}</section></div> : null}
  </section>;
}

function SyncActivity({ events, gap, loading }: { events: DesktopEvent[]; gap: boolean; loading: boolean }) {
  return <div className="sync-activity"><div className="section-heading"><div><div className="eyebrow">Event cursor</div><h3>最近活动</h3></div><span className={gap ? 'activity-status warning' : 'activity-status'}>{gap ? '需要重新同步' : loading ? '读取中' : `${events.length} 条`}</span></div>{events.length ? <ol className="sync-activity-list">{events.slice(-8).reverse().map((event) => <li key={event.id}><span className="activity-cursor">{event.cursor}</span><div><strong>{eventLabel(event.type)}</strong><small>{formatDate(event.created_at)} · {event.id}</small></div></li>)}</ol> : <div className="empty-line"><CircleDashed size={15} /><span>{loading ? '正在读取事件游标…' : '暂无同步活动'}</span></div>}</div>;
}

function eventLabel(type: string): string {
  const labels: Record<string, string> = {
    'project.observed': '已记录本地摘要',
    'workspace.publish.queued': '本地 Revision 已排队',
    'workspace.publish.retrying': '同步将自动重试',
    'workspace.publish.requeued': '失败同步已重新排队',
    'workspace.publish.failed': '同步失败',
    'workspace.publish.conflict': '检测到云端冲突',
    'workspace.publish.auth_required': '需要重新授权设备',
    'workspace.publish.synced': '云端 Revision 已确认',
    'workspace.cloud.reconciled': '已回放云端 Revision',
  };
  return labels[type] ?? type;
}

function OperationResult({ result }: { result: DesktopCommandResult }) {
  if (result.status === 'accepted') return <p className="operation-message success"><CheckCircle2 size={14} />命令已进入持久队列</p>;
  if (result.status === 'rejected') return <p className="operation-message danger"><AlertTriangle size={14} />{result.code}</p>;
  return <p className="operation-message warning"><CloudOff size={14} />{result.message}</p>;
}

function ReviewView({ project }: { project: ProjectSnapshot }) {
  const [selectedRevisionID, setSelectedRevisionID] = useState<string>();
  const [comment, setComment] = useState('');
  const [reason, setReason] = useState('');
  const inbox = useQuery({ queryKey: ['desktop-review-inbox', project.project_id], queryFn: () => window.contentcloudDesktop.getReviewInbox(project.project_id) });
  const items = inbox.data?.status === 'ready' ? inbox.data.value.items : [];
  const selectedID = selectedRevisionID ?? items[0]?.revision.id;
  const detail = useQuery({ queryKey: ['desktop-review-revision', project.project_id, selectedID], queryFn: () => window.contentcloudDesktop.getReviewRevision(project.project_id, selectedID!), enabled: Boolean(selectedID) });
  const detailValue = detail.data?.status === 'ready' ? detail.data.value : undefined;
  const mutation = useMutation({
    mutationFn: async (input: { action: 'approve' | 'reject' | 'request-changes'; reason: string }) => window.contentcloudDesktop.decideReview(project.project_id, detailValue!.revision.id, input.action, { reason: input.reason }),
    onSuccess: () => { setReason(''); void inbox.refetch(); void detail.refetch(); },
  });
  const commentMutation = useMutation({
    mutationFn: () => window.contentcloudDesktop.addReviewComment(project.project_id, { revision_id: detailValue!.revision.id, body: comment }),
    onSuccess: () => { setComment(''); void detail.refetch(); void inbox.refetch(); },
  });

  if (inbox.isLoading) return <LoadingState />;
  if (inbox.data?.status === 'offline') return <section className="status-view"><div className="status-icon"><CloudOff size={20} /></div><div><h2>审批收件箱暂不可用</h2><p>{inbox.data.message}</p></div></section>;
  if (!items.length) return <section className="state-view"><div className="state-view-icon"><ClipboardCheck size={26} /></div><h2>审批收件箱为空</h2><p>当前项目没有待处理的提交内容版本。</p></section>;

  return <section className="review-layout">
    <aside className="review-inbox">
      <div className="section-heading"><div><div className="eyebrow">Review queue</div><h2>审批收件箱</h2></div><span className="item-count">{items.length}</span></div>
      <div className="review-list">{items.map((item) => <button key={item.revision.id} type="button" className={item.revision.id === selectedID ? 'review-list-item active' : 'review-list-item'} onClick={() => setSelectedRevisionID(item.revision.id)}>
        <span className="review-list-item-title">{item.revision.schema_version.replace('contentcloud.', '').replace('/3.0', '')} · R{item.revision.revision_no}</span>
        <span className="review-list-item-meta">{item.submission.status} · {item.pending_comments} 条未解决批注</span>
      </button>)}</div>
    </aside>
    <div className="review-detail">
      {detailValue ? <ReviewDetail detail={detailValue} reason={reason} comment={comment} setReason={setReason} setComment={setComment} onDecision={(action) => mutation.mutate({ action, reason })} onComment={() => commentMutation.mutate()} busy={mutation.isPending || commentMutation.isPending} /> : <LoadingState />}
    </div>
  </section>;
}

function ReviewDetail({ detail, reason, comment, setReason, setComment, onDecision, onComment, busy }: { detail: DesktopReviewRevisionDetail; reason: string; comment: string; setReason: (value: string) => void; setComment: (value: string) => void; onDecision: (action: 'approve' | 'reject' | 'request-changes') => void; onComment: () => void; busy: boolean }) {
  const can = (action: DesktopReviewAction) => detail.allowed_actions.includes(action);
  return <>
    <div className="review-detail-header"><div><div className="eyebrow">{detail.revision.schema_version}</div><h2>Revision {detail.revision.revision_no}</h2><p className="review-meta">{detail.revision.content_hash} · {formatDate(detail.revision.created_at)}</p></div><span className={`state-pill ${detail.submission.status === 'approved' ? 'clean' : detail.submission.status === 'rejected' ? 'conflict' : 'modified'}`}><span />{detail.submission.status}</span></div>
    <div className="review-object-list">{detail.diffs.map((diff) => <article className="review-object" key={`${diff.object_id}-${diff.change}`}><div className="review-object-heading"><div><span className="section-ref">{diff.path || diff.object_id}</span><h3>{diff.object_type}</h3></div><span className={`diff-badge ${diff.change}`}>{diff.change}</span></div><div className="review-object-content"><pre>{diff.current_content ?? diff.base_content ?? ''}</pre></div></article>)}</div>
    <div className="review-actions">
      <label className="field-label" htmlFor="review-reason">审批结论</label><textarea id="review-reason" value={reason} onChange={(event) => setReason(event.target.value)} placeholder="填写可追溯的结论或修改要求" rows={3} />
      <div className="review-action-buttons">{can('approve') ? <button className="command-button" type="button" disabled={busy || !reason.trim()} onClick={() => onDecision('approve')}><CheckCircle2 size={15} />批准</button> : null}{can('request_changes') ? <button className="secondary-button" type="button" disabled={busy || !reason.trim()} onClick={() => onDecision('request-changes')}><RefreshCw size={15} />要求修改</button> : null}{can('reject') ? <button className="danger-button" type="button" disabled={busy || !reason.trim()} onClick={() => onDecision('reject')}><AlertTriangle size={15} />拒绝</button> : null}</div>
    </div>
    <div className="review-comments"><div className="section-heading"><div><div className="eyebrow">Thread</div><h3>批注</h3></div><span className="item-count">{detail.comments.length}</span></div>{detail.comments.map((item) => <div className={item.resolved_at ? 'comment resolved' : 'comment'} key={item.id}><p>{item.body}</p><span>{item.json_pointer || 'Revision'} · {formatDate(item.created_at)}</span></div>)}<div className="comment-compose"><textarea value={comment} onChange={(event) => setComment(event.target.value)} placeholder="添加批注" rows={2} /><button className="secondary-button" type="button" disabled={busy || !comment.trim() || !can('comment')} onClick={onComment}><Send size={15} />添加批注</button></div></div>
  </>;
}

function ContentDirectory({ project }: { project: ProjectSnapshot }) {
  return <section className="content-layout">
    <div className="content-column">
      <div className="section-heading"><div><div className="eyebrow">项目内容</div><h2>内容目录</h2><p className="section-description">从本地工作区到云端 Revision 的持续项目视图</p></div><StatePill state={project.local_state} /></div>
      <OverviewMetrics project={project} />
      <div className="directory-grid">
        {project.content.map((section) => <article className="directory-section" key={section.ref}>
          <div className="directory-heading"><div><span className="section-ref">{section.ref}</span><h3>{section.label}</h3></div><span className="item-count">{section.items.length}</span></div>
          {section.items.length ? <ul className="directory-list">{section.items.map((item) => <li key={item.ref}><FileIcon kind={item.kind} /><span>{item.ref.replace(`${section.ref}/`, '')}</span><ChevronRight size={14} /></li>)}</ul> : <EmptyLine label="目录为空" />}
        </article>)}
      </div>
    </div>
    <aside className="context-column">
      <ContextPanel title="工作区状态" icon={<Cloud size={17} />}>
        <Metric label="来源数量" value={String(project.source_count)} />
        <Metric label="本地变更" value={project.local_state} tone={project.local_state === 'clean' ? 'success' : 'warning'} />
        <Metric label="审批状态" value={project.review_state} />
				<Metric label="本地 Revision" value={String(project.local_revision)} />
      </ContextPanel>
      <ContextPanel title="下一步" icon={<CircleDashed size={17} />}>
        <div className="next-step"><span className="next-step-dot" /><div><strong>{nextStep(project)}</strong><span>{nextStepDetail(project)}</span></div></div>
      </ContextPanel>
    </aside>
  </section>;
}

function OverviewMetrics({ project }: { project: ProjectSnapshot }) {
  const reviewCount = project.pending_feedback + project.pending_decision;
  const syncLabel = project.transfer_state === 'idle' && project.local_state === 'clean' ? '已同步' : project.transfer_state;
  return <div className="overview-metrics" aria-label="项目核心状态">
    <article className="overview-metric"><span className="overview-metric-label">本地变更</span><strong>{project.local_state === 'clean' ? '0' : project.local_state === 'modified' ? '有' : project.local_state}</strong><small>{project.local_state === 'clean' ? '工作区干净' : '等待处理'}</small></article>
    <article className="overview-metric metric-sync"><span className="overview-metric-label">同步状态</span><strong>{syncLabel}</strong><small>Cloud Revision {project.cloud_revision}</small></article>
    <article className="overview-metric metric-review"><span className="overview-metric-label">待审批</span><strong>{reviewCount}</strong><small>{reviewCount ? '需要团队确认' : '没有待处理项'}</small></article>
    <article className="overview-metric metric-runtime"><span className="overview-metric-label">任务运行</span><strong>{project.runtime_state}</strong><small>事件游标 {project.cloud_event_cursor}</small></article>
  </div>;
}

function StatusView({ icon, title, value, detail }: { icon: React.ReactNode; title: string; value: string; detail: string }) {
  return <section className="status-view"><div className="status-icon">{icon}</div><div><div className="eyebrow">项目工作面</div><h2>{title}</h2><div className="status-value">{value}</div><p>{detail}</p></div></section>;
}

function RuntimeView({ project }: { project: ProjectSnapshot }) {
  return <section className="surface-view"><div className="surface-view-heading"><div><div className="eyebrow">Cloud Runtime projection</div><h2>任务运行</h2></div><StatePill state={project.local_state} /></div><div className="runtime-status"><Activity size={22} /><strong>{project.runtime_state}</strong></div><div className="projection-grid"><Metric label="本地 Revision" value={String(project.local_revision)} /><Metric label="Cloud Revision" value={project.cloud_revision} /><Metric label="事件游标" value={String(project.cloud_event_cursor)} /></div><p className="surface-note">Codex 负责任务期推理、生成与工具执行；Desktop 只展示 Daemon 与 Cloud Runtime 的可恢复投影。</p></section>;
}

function ExperienceView({ project }: { project: ProjectSnapshot }) {
  const projection = project.experience ?? emptyExperienceProjection();
  const eventsQuery = useQuery({
    queryKey: ['desktop-experience-events', project.project_id, project.event_cursor],
    queryFn: () => window.contentcloudDesktop.getProjectEvents(project.project_id, Math.max(0, project.event_cursor - 24)),
    staleTime: 5_000,
  });
  const events = eventsQuery.data?.status === 'ready' ? eventsQuery.data.stream.events : [];
  const failureRate = projection.event_count > 0 ? Math.round((projection.failure_count / projection.event_count) * 100) : 0;
  const status = projection.failure_count > 0 ? 'needs_review' : projection.event_count > 0 ? 'observed' : 'empty';

  return <section className="experience-surface">
    <div className="surface-view-heading"><div><div className="eyebrow">Persistent experience layer</div><h2>经验演化</h2><p className="section-description">从不可变项目事件中提炼可回溯模式，为后续 Skill 提案提供证据。</p></div><span className={`state-pill ${status === 'needs_review' ? 'modified' : status === 'observed' ? 'clean' : ''}`}><span />{experienceStatusLabel(status)}</span></div>
    <div className="experience-summary-grid" aria-label="经验统计">
      <article className="experience-stat"><span className="overview-metric-label">执行证据</span><strong>{projection.event_count}</strong><small>不可变事件</small></article>
      <article className="experience-stat success"><span className="overview-metric-label">成功路径</span><strong>{projection.success_count}</strong><small>已确认的同步结果</small></article>
      <article className="experience-stat warning"><span className="overview-metric-label">失败模式</span><strong>{projection.failure_count}</strong><small>{failureRate}% 事件需要复盘</small></article>
      <article className="experience-stat recovery"><span className="overview-metric-label">恢复动作</span><strong>{projection.recovery_count}</strong><small>保留原失败证据</small></article>
    </div>
    <div className="experience-loop" aria-label="经验演化流程">
      <div className="experience-loop-step active"><span><History size={16} /></span><div><strong>执行证据</strong><small>事件游标持续记录</small></div></div>
      <ArrowRightLeft size={16} aria-hidden="true" />
      <div className="experience-loop-step active"><span><BookOpenCheck size={16} /></span><div><strong>模式沉淀</strong><small>{projection.pattern_count} 个当前模式</small></div></div>
      <ArrowRightLeft size={16} aria-hidden="true" />
      <div className="experience-loop-step guarded"><span><GitPullRequest size={16} /></span><div><strong>Skill 提案</strong><small>需通过服务端验证后发布</small></div></div>
    </div>
    <div className="experience-columns">
      <div className="experience-pattern-panel">
        <div className="section-heading"><div><div className="eyebrow">Derived patterns</div><h3>当前模式</h3></div><span className="item-count">{projection.pattern_count}</span></div>
        {projection.patterns.length ? <div className="experience-pattern-list">{projection.patterns.map((pattern) => <article className="experience-pattern" key={pattern.id}><div className="experience-pattern-heading"><div className="experience-pattern-icon"><PatternIcon kind={pattern.kind} /></div><div><strong>{pattern.title}</strong><small>{experienceKindLabel(pattern.kind)} · {pattern.evidence_count} 条证据</small></div><span className={`pattern-status ${pattern.kind}`}>{experiencePatternStatusLabel(pattern.status)}</span></div><p>{pattern.detail}</p>{pattern.evidence.length ? <div className="experience-evidence-refs">{pattern.evidence.slice(0, 2).map((evidence) => <span key={evidence.event_id}>#{evidence.cursor} · {formatDate(evidence.created_at)}</span>)}</div> : null}</article>)}</div> : <div className="experience-empty"><BookOpenCheck size={20} /><strong>还没有可提炼的模式</strong><span>完成一次本地运行或同步后，这里会显示带证据的经验。</span></div>}
      </div>
      <aside className="experience-guardrail-panel">
        <div className="section-heading"><div><div className="eyebrow">Governance</div><h3>演化门禁</h3></div><ShieldCheck size={17} /></div>
        <div className="guardrail-list"><div className="guardrail-item passed"><CheckCircle2 size={15} /><div><strong>事件不可变</strong><small>只追加证据，不覆盖原始结果。</small></div></div><div className="guardrail-item passed"><CheckCircle2 size={15} /><div><strong>证据可追溯</strong><small>模式绑定事件 ID 和游标。</small></div></div><div className="guardrail-item pending"><RotateCcw size={15} /><div><strong>Skill 发布</strong><small>提案必须在沙箱验证并人工确认。</small></div></div></div>
        <div className="guardrail-note"><AlertTriangle size={14} /><span>经验层与客户知识事实源分离，避免把执行失败误写成产品事实。</span></div>
      </aside>
    </div>
    <section className="experience-events"><div className="section-heading"><div><div className="eyebrow">Recent evidence</div><h3>最近证据</h3></div><span className={eventsQuery.data?.status === 'offline' ? 'activity-status warning' : 'activity-status'}>{eventsQuery.data?.status === 'offline' ? '读取失败' : eventsQuery.isLoading ? '读取中' : `${events.length} 条`}</span></div>{events.length ? <ol className="experience-event-list">{events.slice(-10).reverse().map((event) => <li key={event.id}><span className={`experience-event-dot ${experienceEventKind(event.type)}`} /><div><strong>{eventLabel(event.type)}</strong><small>游标 {event.cursor} · {formatDate(event.created_at)} · {event.id}</small></div></li>)}</ol> : <div className="empty-line"><CircleDashed size={15} /><span>{eventsQuery.data?.status === 'offline' ? eventsQuery.data.message : '暂无可展示的项目事件'}</span></div>}</section>
  </section>;
}

function emptyExperienceProjection(): DesktopExperienceProjection {
  return { schema_version: 'contentcloud.desktop-experience/1.0', event_count: 0, success_count: 0, failure_count: 0, recovery_count: 0, pattern_count: 0, patterns: [] };
}

function experienceStatusLabel(status: string): string {
  return status === 'needs_review' ? '需要复盘' : status === 'observed' ? '已记录' : '等待事件';
}

function experienceKindLabel(kind: string): string {
  return ({ failure: '失败模式', recovery: '恢复路径', success: '成功路径', observation: '工作区观察' } as Record<string, string>)[kind] ?? '经验模式';
}

function experiencePatternStatusLabel(status: string): string {
  return ({ needs_review: '待复盘', improving: '恢复中', validated: '已验证', recorded: '已记录' } as Record<string, string>)[status] ?? status;
}

function experienceEventKind(type: string): string {
  if (type.includes('failed') || type.includes('conflict') || type.includes('auth_required') || type.includes('resync')) return 'failure';
  if (type.includes('retry') || type.includes('requeued')) return 'recovery';
  if (type.includes('synced')) return 'success';
  return 'observation';
}

function PatternIcon({ kind }: { kind: string }) {
  if (kind === 'failure') return <AlertTriangle size={15} />;
  if (kind === 'recovery') return <RotateCcw size={15} />;
  if (kind === 'success') return <CheckCircle2 size={15} />;
  return <History size={15} />;
}

function DeliveryView({ project }: { project: ProjectSnapshot }) {
  const query = useQuery({ queryKey: ['desktop-deliveries'], queryFn: () => window.contentcloudDesktop.getDeliveries(), refetchInterval: 30_000 });
  const [downloadResult, setDownloadResult] = useState<DesktopDeliveryDownloadResult>();
  const download = useMutation({ mutationFn: (file: { id: string; file_name: string }) => window.contentcloudDesktop.downloadDelivery(file.id, file.file_name), onSuccess: setDownloadResult });
  const deliveries = query.data?.status === 'ready' ? query.data.value : undefined;
  const packages = deliveries?.packages.filter((item) => item.project_name === project.name) ?? [];
  const publications = deliveries?.publications.filter((item) => item.project_name === project.name) ?? [];
  const downloadMessage = downloadResult?.status === 'unauthenticated' ? '登录服务端后下载交付文件' : downloadResult && 'message' in downloadResult ? downloadResult.message : '';
  return <section className="surface-view"><div className="surface-view-heading"><div><div className="eyebrow">Approved snapshot delivery</div><h2>交付状态</h2></div><span className={`state-pill ${project.lifecycle_state === 'delivered' ? 'clean' : 'modified'}`}><span />{project.lifecycle_state}</span></div><div className="delivery-track"><div className="delivery-step done"><span>1</span><div><strong>内容 Revision</strong><small>{project.cloud_revision}</small></div></div><div className="delivery-step"><span>2</span><div><strong>审批结论</strong><small>{project.review_state}</small></div></div><div className={project.lifecycle_state === 'delivered' ? 'delivery-step done' : 'delivery-step'}><span>3</span><div><strong>交付包</strong><small>{project.lifecycle_state === 'delivered' ? '已生成并可追踪' : '等待批准快照'}</small></div></div></div><section className="delivery-records"><div className="section-heading"><div><div className="eyebrow">Server projection</div><h3>已准备文件</h3></div><span className="item-count">{packages.reduce((count, item) => count + item.files.length, 0)}</span></div>{query.data?.status === 'offline' ? <p className="operation-message warning"><CloudOff size={14} />{query.data.message}</p> : query.data?.status === 'unauthenticated' ? <p className="operation-message warning"><KeyRound size={14} />登录服务端后查看交付文件</p> : packages.length ? <div className="delivery-package-list">{packages.map((item) => <article className="delivery-package" key={item.id}><header><div><strong>{item.project_name}</strong><small>{deliveryStatusLabel(item.status)} · {formatDate(item.created_at)}</small></div><span className="state-pill clean"><span />已准备</span></header>{item.files.map((file) => <div className="delivery-file" key={file.id}><FileText size={15} /><div><strong>{file.file_name}</strong><small>{file.media_type} · {formatBytes(file.byte_size)}</small></div><button className="icon-button" type="button" onClick={() => download.mutate(file)} disabled={download.isPending} aria-label={`下载 ${file.file_name}`} title="下载"><Download size={14} className={download.isPending ? 'spin' : undefined} /></button></div>)}</article>)}</div> : <div className="empty-line"><PackageCheck size={15} /><span>当前项目还没有可下载的交付文件</span></div>}{downloadResult && downloadResult.status !== 'saved' && downloadResult.status !== 'canceled' ? <p className="operation-message warning"><AlertTriangle size={14} />{downloadMessage}</p> : downloadResult?.status === 'saved' ? <p className="operation-message success"><CheckCircle2 size={14} />已保存 {downloadResult.file_name}</p> : null}</section><section className="delivery-records"><div className="section-heading"><div><div className="eyebrow">Publication receipts</div><h3>发布回执</h3></div><span className="item-count">{publications.length}</span></div>{publications.length ? <ul className="publication-list">{publications.map((item) => <li key={item.id}><CheckCircle2 size={15} /><div><strong>{item.destination}</strong><small>{deliveryStatusLabel(item.status)} · {formatDate(item.published_at ?? item.updated_at)}</small></div></li>)}</ul> : <div className="empty-line"><CircleDashed size={15} /><span>暂无外部平台发布回执</span></div>}</section><p className="surface-note">交付只能引用已批准快照和内容摘要，下载动作由主进程完成并写入用户选择的本地目录。</p></section>;
}

function deliveryStatusLabel(status: string): string {
  const labels: Record<string, string> = { ready: '已准备', delivered: '已交付', published: '已发布', pending: '处理中', failed: '失败' };
  return labels[status] ?? status;
}

function ContextPanel({ title, icon, children }: { title: string; icon: React.ReactNode; children: React.ReactNode }) {
  return <section className="context-panel"><div className="context-panel-heading">{icon}<h3>{title}</h3></div>{children}</section>;
}

function Metric({ label, value, tone = 'neutral' }: { label: string; value: string; tone?: 'neutral' | 'success' | 'warning' }) {
  return <div className="metric-row"><span>{label}</span><strong className={`metric-value ${tone}`}>{value}</strong></div>;
}

function daemonLabel(result: DesktopDaemonStatus | undefined): string {
  if (!result) return 'Daemon 检查中';
  const labels: Record<DesktopDaemonStatus['state'], string> = {
    running: 'Daemon 已连接',
    starting: 'Daemon 启动中',
    stopped: 'Daemon 未运行',
    failed: 'Daemon 启动失败',
    unsupported: 'Daemon 不受支持',
  };
  return labels[result.state];
}

function DaemonControl({ result, busy, onAction }: { result: DesktopDaemonStatus | undefined; busy: boolean; onAction: (action: 'start' | 'stop' | 'restart') => void }) {
  const state = result?.state ?? 'starting';
  const tone = state === 'running' ? 'ready' : state === 'failed' ? 'offline' : 'pending';
  return <div className="daemon-control">
    <div className={`connection-badge ${tone}`}><span className="connection-dot" /><span>{daemonLabel(result)}</span></div>
    {result?.message ? <small className="daemon-message">{result.message}</small> : null}
    <div className="daemon-actions">
      {state === 'running' ? <button className="secondary-button" type="button" onClick={() => onAction('stop')} disabled={busy} title={result?.managed ? '停止 Desktop 管理的 Daemon' : '停止系统托管的 Daemon'}><Square size={13} />停止</button> : null}
      {(state === 'stopped' || state === 'failed') ? <button className="secondary-button" type="button" onClick={() => onAction('start')} disabled={busy}><Power size={14} />{busy ? '启动中' : '启动 Daemon'}</button> : null}
      {state === 'running' ? <button className="icon-button" type="button" onClick={() => onAction('restart')} disabled={busy} aria-label="重启 Daemon" title="重启 Daemon"><RefreshCw size={14} className={busy ? 'spin' : undefined} /></button> : null}
    </div>
  </div>;
}

function AuthBadge({ result, onClick }: { result: DesktopAuthResult | undefined; onClick: () => void }) {
  if (result?.status === 'authenticated') return <button className="auth-badge authenticated" type="button" onClick={onClick}><ShieldCheck size={14} /><span>{result.session.tenant.name}</span></button>;
  if (result?.status === 'offline') return <button className="auth-badge offline" type="button" onClick={onClick}><CloudOff size={14} /><span>服务端离线</span><ArrowUpRight size={12} /></button>;
  return <button className="auth-badge" type="button" onClick={onClick}><KeyRound size={14} /><span>登录服务端</span><ArrowUpRight size={12} /></button>;
}

function AuthDialog({ initial, onClose, onAuthenticated }: { initial?: string; onClose: () => void; onAuthenticated: () => Promise<void> }) {
  const [serverURL, setServerURL] = useState(initial ?? 'https://content.zhongcao.run');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState('');

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setFailure('');
    setBusy(true);
    try {
      const result = await window.contentcloudDesktop.login({ server_url: serverURL, email, password });
      if (result.status !== 'authenticated') {
        setFailure(result.status === 'rejected' ? result.message : result.status === 'offline' ? result.message : '登录失败');
        return;
      }
      await onAuthenticated();
    } catch (error) {
      setFailure(error instanceof Error ? error.message : '登录失败');
    } finally {
      setBusy(false);
    }
  };

  return <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.currentTarget === event.target) onClose(); }}>
    <section className="auth-dialog" role="dialog" aria-modal="true" aria-labelledby="auth-dialog-title">
      <header className="auth-dialog-header"><div><div className="eyebrow">Cloud access</div><h2 id="auth-dialog-title">连接 Content Work OS</h2><p>登录后可查看当前租户；本地同步继续使用已授权的设备绑定。</p></div><button className="icon-button" type="button" onClick={onClose} aria-label="关闭登录窗口" title="关闭"><X size={17} /></button></header>
      <form className="auth-form" onSubmit={submit}>
        <label className="field-label" htmlFor="desktop-server-url">服务端地址</label>
        <div className="input-with-icon"><Server size={16} /><input id="desktop-server-url" type="url" value={serverURL} onChange={(event) => setServerURL(event.target.value)} placeholder="https://content.example.com" disabled={busy} required /></div>
        <label className="field-label" htmlFor="desktop-email">邮箱</label>
        <div className="input-with-icon"><UserRound size={16} /><input id="desktop-email" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="name@company.com" disabled={busy} required /></div>
        <label className="field-label" htmlFor="desktop-password">密码</label>
        <div className="input-with-icon"><KeyRound size={16} /><input id="desktop-password" type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} placeholder="请输入密码" disabled={busy} required /></div>
        {failure ? <p className="auth-error"><AlertTriangle size={14} />{failure}</p> : null}
        <button className="command-button auth-submit-button" type="submit" disabled={busy}>{busy ? <RefreshCw size={15} className="spin" /> : <LogIn size={15} />}{busy ? '正在验证' : '登录并连接'}</button>
      </form>
      <p className="auth-dialog-note"><ShieldCheck size={14} />密码只在主进程完成认证；会话 Cookie 使用系统安全存储，不会进入本地 Workspace 或页面存储。</p>
    </section>
  </div>;
}

function CreateProjectDialog({ existingProjectIDs, onClose, onCreated }: { existingProjectIDs: Set<string>; onClose: () => void; onCreated: (result: DesktopServerResult<DesktopServerBootstrap>, projectID?: string) => void }) {
  const [brandName, setBrandName] = useState('');
  const [productName, setProductName] = useState('');
  const [contentType, setContentType] = useState('video_script');
  const [channel, setChannel] = useState('douyin');
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState('');

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setFailure('');
    setBusy(true);
    const input: DesktopCreateServerProjectInput = { brand_name: brandName, product_name: productName, content_type: contentType, channel };
    try {
      const result = await window.contentcloudDesktop.createServerProject(input);
      if (result.status === 'ready') {
        const createdProject = result.value.projects.find((project) => !existingProjectIDs.has(project.id))
          ?? result.value.projects.find((project) => project.brand_name === brandName.trim() && project.product_name === productName.trim());
        onCreated(result, createdProject?.id);
      } else {
        setFailure(result.status === 'rejected' ? result.message : result.status === 'offline' ? result.message : '服务端会话已失效，请重新登录');
      }
    } catch (error) {
      setFailure(error instanceof Error ? error.message : '创建项目失败');
    } finally {
      setBusy(false);
    }
  };

  return <div className="modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.currentTarget === event.target) onClose(); }}>
    <section className="auth-dialog create-project-dialog" role="dialog" aria-modal="true" aria-labelledby="create-project-title">
      <header className="auth-dialog-header"><div><div className="eyebrow">New cloud project</div><h2 id="create-project-title">创建项目</h2><p>项目创建在当前租户内完成，随后可以发起设备绑定。</p></div><button className="icon-button" type="button" onClick={onClose} aria-label="关闭创建项目窗口" title="关闭"><X size={17} /></button></header>
      <form className="auth-form" onSubmit={submit}>
        <label className="field-label" htmlFor="project-brand-name">品牌名称</label>
        <div className="input-with-icon"><LayoutDashboard size={16} /><input id="project-brand-name" type="text" value={brandName} onChange={(event) => setBrandName(event.target.value)} placeholder="例如：好视觉" disabled={busy} required maxLength={120} /></div>
        <label className="field-label" htmlFor="project-product-name">产品或服务</label>
        <div className="input-with-icon"><FileText size={16} /><input id="project-product-name" type="text" value={productName} onChange={(event) => setProductName(event.target.value)} placeholder="例如：春季新品" disabled={busy} required maxLength={120} /></div>
        <label className="field-label" htmlFor="project-content-type">内容类型</label>
        <select id="project-content-type" value={contentType} onChange={(event) => setContentType(event.target.value)} disabled={busy}><option value="video_script">视频剧本</option><option value="marketing_video">营销视频</option><option value="article">文章</option></select>
        <label className="field-label" htmlFor="project-channel">发布渠道</label>
        <select id="project-channel" value={channel} onChange={(event) => setChannel(event.target.value)} disabled={busy}><option value="douyin">抖音</option><option value="xiaohongshu">小红书</option><option value="wechat">微信公众号</option></select>
        {failure ? <p className="auth-error"><AlertTriangle size={14} />{failure}</p> : null}
        <button className="command-button auth-submit-button" type="submit" disabled={busy}>{busy ? <RefreshCw size={15} className="spin" /> : <Plus size={15} />}{busy ? '正在创建' : '创建并打开项目'}</button>
      </form>
    </section>
  </div>;
}

function StatePill({ state }: { state: ProjectSnapshot['local_state'] }) {
  const labels = { clean: '本地干净', modified: '有本地变更', deleted: '文件已删除', conflict: '存在冲突' };
  return <span className={`state-pill ${state}`}><span />{labels[state]}</span>;
}

function FileIcon({ kind }: { kind: string }) {
  if (kind === 'directory') return <Archive size={15} />;
  if (kind.includes('image')) return <LayoutDashboard size={15} />;
  return <FileText size={15} />;
}

function OfflineState({ message, onRetry, onLogin }: { message: string; onRetry: () => void; onLogin: () => void }) {
  return <section className="offline-panel"><div className="state-view-icon"><WifiOff size={26} /></div><div><div className="eyebrow">Desktop connection</div><h2>本地服务未连接</h2><p>{message}</p></div><div className="connection-guide"><div className="connection-guide-step"><span>1</span><div><strong>登录服务端</strong><small>确认当前团队与账号权限</small></div></div><div className="connection-guide-step"><span>2</span><div><strong>启动本地 Daemon</strong><small>让同步引擎读取已绑定的 Workspace</small></div></div><div className="connection-guide-step"><span>3</span><div><strong>重新读取项目状态</strong><small>连接后会显示 Revision、审批和交付投影</small></div></div></div><div className="offline-actions"><button className="command-button" onClick={onLogin} type="button"><LogIn size={15} />登录服务端</button><button className="secondary-button" onClick={onRetry} type="button"><RefreshCw size={15} />重新连接</button></div></section>;
}

function LoadingState() {
  return <section className="state-view loading"><div className="loading-bar" /><div className="loading-bar short" /><div className="loading-bar" /></section>;
}

function EmptyProjectState() {
  return <section className="state-view"><div className="state-view-icon"><FolderTree size={26} /></div><h2>暂无已绑定项目</h2><p>本地 Daemon 尚未提供项目绑定。</p></section>;
}

function EmptyLine({ label }: { label: string }) {
  return <div className="empty-line"><Archive size={15} /><span>{label}</span></div>;
}

function nextStep(project: ProjectSnapshot): string {
  if (project.local_state === 'conflict') return '处理本地冲突';
  if (project.pending_feedback + project.pending_decision > 0) return '查看审批收件箱';
  if (project.local_state === 'modified') return '同步本地变更';
  return '继续项目工作';
}

function nextStepDetail(project: ProjectSnapshot): string {
  if (project.local_state === 'conflict') return 'stale base 不会被静默覆盖';
  if (project.pending_feedback + project.pending_decision > 0) return '先确认 Revision 和 digest';
  if (project.local_state === 'modified') return 'Daemon 会计算 digest 并排队传输';
  return 'Codex 负责任务期生成，Desktop 保持目录与状态';
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value));
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 * 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`;
  return `${(value / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function isPreviewable(mimeType: string): boolean {
  return mimeType.startsWith('image/') || mimeType.startsWith('video/') || mimeType.startsWith('audio/');
}
