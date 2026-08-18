import { useEffect, useMemo, useState } from 'react';
import { QueryClient, QueryClientProvider, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Activity,
  AlertTriangle,
  Archive,
  CheckCircle2,
  ChevronRight,
  CircleDashed,
  ClipboardCheck,
  Cloud,
  CloudOff,
  FileText,
  FolderTree,
  LayoutDashboard,
  PackageCheck,
  RefreshCw,
  Send,
  UploadCloud,
  WifiOff,
} from 'lucide-react';

import type { DesktopCommandResult, DesktopReviewAction, DesktopReviewRevisionDetail, DesktopSnapshot, DesktopSnapshotResult, ProjectSnapshot } from '../shared/contracts';

type View = 'overview' | 'review' | 'transfers' | 'runs' | 'delivery';

const viewItems: Array<{ id: View; label: string; icon: typeof LayoutDashboard }> = [
  { id: 'overview', label: '内容目录', icon: FolderTree },
  { id: 'transfers', label: '同步与上传', icon: UploadCloud },
  { id: 'review', label: '审批收件箱', icon: ClipboardCheck },
  { id: 'runs', label: '任务运行', icon: Activity },
  { id: 'delivery', label: '交付状态', icon: PackageCheck },
];

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });

export function DesktopApp() {
  return <QueryClientProvider client={queryClient}><DesktopWorkspace /></QueryClientProvider>;
}

function DesktopWorkspace() {
	const [selectedProjectID, setSelectedProjectID] = useState<string>();
	const [view, setView] = useState<View>('overview');
	const cache = useQueryClient();
  const query = useQuery({
    queryKey: ['desktop-snapshot'],
    queryFn: () => window.contentcloudDesktop.getSnapshot(),
    refetchInterval: false,
  });
	const publish = useMutation({
		mutationFn: (target: ProjectSnapshot) => window.contentcloudDesktop.publishWorkspace({
			workspace_id: target.workspace_id,
			project_id: target.project_id,
			base_revision: target.cloud_revision,
			observed_digest: target.observed_digest ?? '',
		}),
	});

	useEffect(() => window.contentcloudDesktop.onSnapshotChanged((result) => {
		cache.setQueryData(['desktop-snapshot'], result);
		if (result.status === 'ready' && !selectedProjectID) setSelectedProjectID(result.snapshot.projects[0]?.project_id);
	}), [cache, selectedProjectID]);

  const state = query.data;
  const snapshot = state?.status === 'ready' ? state.snapshot : undefined;
  const project = useMemo(() => selectProject(snapshot, selectedProjectID), [snapshot, selectedProjectID]);
  const refresh = () => query.refetch().catch(() => undefined);
	const publishProject = () => {
		if (project?.observed_digest) publish.mutate(project);
	};

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
          <ConnectionBadge result={state} />
        </div>
      </aside>

      <main className="workspace-surface">
        <header className="topbar">
          <div>
            <div className="eyebrow">持续项目工作面</div>
            <h1>{project?.name ?? 'Content Work OS Desktop'}</h1>
          </div>
          <div className="topbar-actions">
            <span className="version-label">{snapshot?.daemon.version ? `Daemon ${snapshot.daemon.version}` : 'Daemon 未连接'}</span>
            <button className="icon-button" onClick={refresh} type="button" aria-label="刷新项目状态" title="刷新项目状态">
              <RefreshCw size={17} className={query.isFetching ? 'spin' : undefined} />
            </button>
          </div>
        </header>

        {state?.status === 'offline' ? <OfflineState message={state.message} onRetry={refresh} /> : null}
        {state?.status === 'ready' && snapshot ? <>
          <ProjectPicker projects={snapshot.projects} selectedProjectID={selectedProjectID} onChange={setSelectedProjectID} />
          {!project ? <EmptyProjectState /> : <ViewContent project={project} view={view} onPublish={publishProject} publishing={publish.isPending} publishResult={publish.data} />}
        </> : null}
        {!state && query.isLoading ? <LoadingState /> : null}
      </main>
    </div>
  );
}

function selectProject(snapshot: DesktopSnapshot | undefined, selectedProjectID: string | undefined): ProjectSnapshot | undefined {
  if (!snapshot?.projects.length) return undefined;
  return snapshot.projects.find((item) => item.project_id === selectedProjectID) ?? snapshot.projects[0];
}

function ProjectPicker({ projects, selectedProjectID, onChange }: { projects: ProjectSnapshot[]; selectedProjectID?: string; onChange: (value: string) => void }) {
  return <div className="project-picker">
    <div className="project-picker-leading"><FolderTree size={17} /><span>已绑定项目</span></div>
    <select value={selectedProjectID ?? projects[0]?.project_id ?? ''} onChange={(event) => onChange(event.target.value)} aria-label="选择项目">
      {projects.map((project) => <option key={project.project_id} value={project.project_id}>{project.name} · {project.project_id}</option>)}
    </select>
    <span className="picker-note">{projects.length} 个本地 Workspace</span>
  </div>;
}

function ViewContent({ project, view, onPublish, publishing, publishResult }: { project: ProjectSnapshot; view: View; onPublish: () => void; publishing: boolean; publishResult?: DesktopCommandResult }) {
  switch (view) {
    case 'transfers': return <TransferView project={project} onPublish={onPublish} publishing={publishing} result={publishResult} />;
    case 'review': return <ReviewView project={project} />;
    case 'runs': return <RuntimeView project={project} />;
    case 'delivery': return <DeliveryView project={project} />;
    default: return <ContentDirectory project={project} />;
  }
}

function TransferView({ project, onPublish, publishing, result }: { project: ProjectSnapshot; onPublish: () => void; publishing: boolean; result?: DesktopCommandResult }) {
  const allowed = project.allowed_actions.includes('workspace.publish') && Boolean(project.observed_digest);
  return <section className="status-view">
    <div className="status-icon"><UploadCloud size={20} /></div>
    <div>
      <div className="eyebrow">项目工作面</div>
      <h2>同步与上传</h2>
      <div className="status-value">{project.transfer_state}</div>
      <p>{project.last_synced_at ? `最近同步 ${formatDate(project.last_synced_at)}` : `本地 Revision ${project.local_revision} · Cloud Revision ${project.cloud_revision}`}</p>
      <button className="command-button" type="button" onClick={onPublish} disabled={!allowed || publishing}>
        <UploadCloud size={15} />{publishing ? '正在排队' : project.transfer_state === 'queued' ? '已进入同步队列' : '发布本地 Revision'}
      </button>
      {result ? <OperationResult result={result} /> : null}
    </div>
  </section>;
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
      <div className="section-heading"><div><div className="eyebrow">项目内容</div><h2>内容目录</h2></div><StatePill state={project.local_state} /></div>
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

function StatusView({ icon, title, value, detail }: { icon: React.ReactNode; title: string; value: string; detail: string }) {
  return <section className="status-view"><div className="status-icon">{icon}</div><div><div className="eyebrow">项目工作面</div><h2>{title}</h2><div className="status-value">{value}</div><p>{detail}</p></div></section>;
}

function RuntimeView({ project }: { project: ProjectSnapshot }) {
  return <section className="surface-view"><div className="surface-view-heading"><div><div className="eyebrow">Cloud Runtime projection</div><h2>任务运行</h2></div><StatePill state={project.local_state} /></div><div className="runtime-status"><Activity size={22} /><strong>{project.runtime_state}</strong></div><div className="projection-grid"><Metric label="本地 Revision" value={String(project.local_revision)} /><Metric label="Cloud Revision" value={project.cloud_revision} /><Metric label="事件游标" value={String(project.cloud_event_cursor)} /></div><p className="surface-note">Codex 负责任务期推理、生成与工具执行；Desktop 只展示 Daemon 与 Cloud Runtime 的可恢复投影。</p></section>;
}

function DeliveryView({ project }: { project: ProjectSnapshot }) {
  return <section className="surface-view"><div className="surface-view-heading"><div><div className="eyebrow">Approved snapshot delivery</div><h2>交付状态</h2></div><span className={`state-pill ${project.lifecycle_state === 'delivered' ? 'clean' : 'modified'}`}><span />{project.lifecycle_state}</span></div><div className="delivery-track"><div className="delivery-step done"><span>1</span><div><strong>内容 Revision</strong><small>{project.cloud_revision}</small></div></div><div className="delivery-step"><span>2</span><div><strong>审批结论</strong><small>{project.review_state}</small></div></div><div className={project.lifecycle_state === 'delivered' ? 'delivery-step done' : 'delivery-step'}><span>3</span><div><strong>交付包</strong><small>{project.lifecycle_state === 'delivered' ? '已生成并可追踪' : '等待批准快照'}</small></div></div></div><p className="surface-note">交付只能引用已批准快照和内容摘要，Desktop 不渲染 Codex 对话内容。</p></section>;
}

function ContextPanel({ title, icon, children }: { title: string; icon: React.ReactNode; children: React.ReactNode }) {
  return <section className="context-panel"><div className="context-panel-heading">{icon}<h3>{title}</h3></div>{children}</section>;
}

function Metric({ label, value, tone = 'neutral' }: { label: string; value: string; tone?: 'neutral' | 'success' | 'warning' }) {
  return <div className="metric-row"><span>{label}</span><strong className={`metric-value ${tone}`}>{value}</strong></div>;
}

function ConnectionBadge({ result }: { result: DesktopSnapshotResult | undefined }) {
  if (result?.status === 'ready') return <div className="connection-badge ready"><Cloud size={14} /><span>Daemon 已连接</span></div>;
  return <div className="connection-badge offline"><CloudOff size={14} /><span>Daemon 离线</span></div>;
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

function OfflineState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return <section className="state-view"><div className="state-view-icon"><WifiOff size={26} /></div><h2>本地服务未连接</h2><p>{message}</p><button className="command-button" onClick={onRetry} type="button"><RefreshCw size={15} />重新连接</button></section>;
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
