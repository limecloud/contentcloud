import {
  AlertCircle,
  ArrowLeft,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleHelp,
  ClipboardCheck,
  Clock3,
  Download,
  FileCheck2,
  FileInput,
  Film,
  Image as ImageIcon,
  LoaderCircle,
  PackageCheck,
  Play,
  PlayCircle,
  RefreshCw,
  ShieldCheck,
  Upload,
  Video,
  Workflow,
  X
} from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { api, download, post, upload } from '../api';
import { Button, Empty, Field, IconButton } from '../components/ui';
import type {
  ApprovedSnapshot,
  Artifact,
  DeliveryPackage,
  MediaGenerationJob,
  MediaReview,
  StageDefinition,
  StoryboardAsset,
  StoryboardPackage,
  StoryboardShot,
  TaskStageOutput,
  WorkTaskView
} from '../types';
import { normalizeWorkTaskView } from './workOSData';

type ProductionTab = 'content' | 'storyboard' | 'generation' | 'review' | 'delivery';

const productionTabs: {id: ProductionTab; label: string; icon: typeof FileCheck2}[] = [
  {id: 'content', label: '内容与剧本', icon: FileCheck2},
  {id: 'storyboard', label: '分镜素材', icon: ImageIcon},
  {id: 'generation', label: '视频生成', icon: Video},
  {id: 'review', label: '质检与后期', icon: ClipboardCheck},
  {id: 'delivery', label: '交付', icon: PackageCheck}
];

const statusLabels: Record<string, string> = {
  needs_input: '待补输入', ready: '可开始', running: '运行中', paused: '已暂停', waiting_gate: '待决定', blocked: '已阻断', accepted: '已接受', delivered: '已交付', cancelled: '已取消',
  pending: '待处理', approved: '已批准', validated: '已核验', candidate: '候选', changes_requested: '需修改', rejected: '已拒绝', completed: '已完成', succeeded: '已生成', queued: '排队中', submitting: '提交中', submitted: '已提交', generating: '生成中', downloading: '下载中', validating: '校验中', awaiting_cost_approval: '待确认费用', failed: '失败', cancelled_job: '已取消', ready_package: '可交付', complete: '完整', legacy_incomplete: '历史不完整'
};

const outputLabels: Record<string, string> = {
  source_revision: '来源 Revision ID', knowledge_snapshot: '知识快照 ID', submission_revision: '剧本 Revision ID', storyboard_package: 'Storyboard ApprovedSnapshot ID', artifact: 'Artifact ID', generation_job: 'Media Job ID', media_review: 'Media Review ID', delivery_package: 'DeliveryPackage ID'
};

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function list(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function text(value: unknown, fallback = ''): string {
  return typeof value === 'string' || typeof value === 'number' ? String(value) : fallback;
}

function formatDateTime(value?: string): string {
  if (!value) return '未记录';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '未知时间' : new Intl.DateTimeFormat('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit'}).format(date);
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

function formatCost(value: number, currency: string): string {
  return new Intl.NumberFormat('zh-CN', {style: 'currency', currency: currency || 'CNY'}).format(value / 100);
}

function tone(value: string): string {
  if (['approved', 'validated', 'completed', 'succeeded', 'delivered', 'complete', 'ready'].includes(value)) return 'is-success';
  if (['failed', 'rejected', 'blocked', 'output_invalid', 'legacy_incomplete'].includes(value)) return 'is-danger';
  if (['pending', 'waiting_gate', 'changes_requested', 'awaiting_cost_approval', 'candidate'].includes(value)) return 'is-warning';
  if (['running', 'queued', 'submitting', 'submitted', 'generating', 'downloading', 'validating'].includes(value)) return 'is-production';
  return 'is-muted';
}

function tabForStage(stageID: string): ProductionTab {
  if (stageID === 'storyboard') return 'storyboard';
  if (stageID === 'generation') return 'generation';
  if (stageID === 'review' || stageID === 'postproduction') return 'review';
  if (stageID === 'delivery') return 'delivery';
  return 'content';
}

function storyboardFromSnapshot(snapshot?: ApprovedSnapshot): StoryboardPackage | undefined {
  if (!snapshot) return undefined;
  const canonical = record(snapshot.canonical_content);
  const candidates = list(canonical.objects).length > 0 ? list(canonical.objects) : [canonical];
  return candidates.map(record).find(value => value.type === 'storyboard_package') as unknown as StoryboardPackage | undefined;
}

function genericStoryboardShots(snapshot?: ApprovedSnapshot): Record<string, unknown>[] {
  if (!snapshot) return [];
  const canonical = record(snapshot.canonical_content);
  const packageValue = storyboardFromSnapshot(snapshot);
  return packageValue ? packageValue.shots.map(value => value as unknown as Record<string, unknown>) : list(canonical.shots).map(record);
}

export function TaskProductionPage({projectID: explicitProjectID}: {projectID?: string} = {}) {
  const {taskID, projectID: routeProjectID} = useParams();
  const projectID = explicitProjectID ?? routeProjectID;
  const navigate = useNavigate();
  const [view, setView] = useState<WorkTaskView>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState('');
  const [tab, setTab] = useState<ProductionTab>('content');
  const [manualTab, setManualTab] = useState(false);
  const [outputIDs, setOutputIDs] = useState<Record<string, string>>({});
  const [scriptJSON, setScriptJSON] = useState('{\n  "title": "",\n  "scenes": [\n    {\n      "scene": 1,\n      "duration_seconds": 5,\n      "visual": "",\n      "voiceover": ""\n    }\n  ]\n}');
  const [deliveryDestination, setDeliveryDestination] = useState('workspace');
  const [videoErrors, setVideoErrors] = useState<Record<string, boolean>>({});

  const reload = async () => {
    if (!taskID) return;
    const value = normalizeWorkTaskView(await api<WorkTaskView>(`/api/bff/tasks/${encodeURIComponent(taskID)}`));
    setView(value);
    if (!manualTab) setTab(tabForStage(value.task.current_stage_id));
    if (!projectID && value.project.id) navigate(`/projects/${encodeURIComponent(value.project.id)}/tasks/${encodeURIComponent(value.task.id)}`, {replace: true});
  };

  useEffect(() => {
    if (!taskID) return;
    setLoading(true);
    setError('');
    reload().catch(value => setError(value instanceof Error ? value.message : '任务加载失败')).finally(() => setLoading(false));
  }, [taskID]);

  const polling = view?.media_jobs.some(job => ['queued', 'submitting', 'submitted', 'generating', 'downloading', 'validating', 'retry_wait'].includes(job.state));
  useEffect(() => {
    if (!polling) return;
    const timer = window.setInterval(() => void reload().catch(() => undefined), 4000);
    return () => window.clearInterval(timer);
  }, [polling, taskID]);

  const runOperation = async (key: string, operation: () => Promise<void>, success: string) => {
    setBusy(key);
    setNotice('');
    try {
      await operation();
      setNotice(success);
    } catch (value) {
      const stale = value instanceof Error && ((value as Error & {status?: number}).status === 409);
      setNotice(stale ? `${value.message}，请刷新任务后重试。` : value instanceof Error ? value.message : '操作失败');
    } finally {
      setBusy('');
    }
  };

  if (loading) return <div className="workos-page"><div className="workos-loading"><LoaderCircle className="is-spinning" size={17}/>正在读取生产任务...</div></div>;
  if (error || !view) return <div className="workos-page"><Empty title="任务不可用" detail={error || '任务不存在或已被移除。'} action={<Button onClick={() => navigate(projectID ? `/projects/${projectID}/tasks` : '/workspace/tasks')}><ArrowLeft size={15}/>返回任务列表</Button>}/></div>;

  const task = view.task;
  const currentStage = view.sop.stages.find(stage => stage.stage_id === task.current_stage_id);
  const activeRun = view.stage_runs.find(run => run.stage_id === task.current_stage_id);
  const storyboardSnapshot = [...view.approved_snapshots].reverse().find(snapshot => snapshot.submission_type === 'storyboard');
  const storyboard = storyboardFromSnapshot(storyboardSnapshot);
  const selectedReview = [...view.media_reviews].reverse().find(review => review.review_kind === 'content' && review.status === 'approved' && review.selected);
  const finalReview = [...view.media_reviews].reverse().find(review => review.review_kind === 'final');
  const finalArtifact = finalReview ? view.artifacts.find(artifact => artifact.id === finalReview.subject_artifact_id) : view.artifacts.find(artifact => artifact.kind === 'final_render');
  const latestPackage = [...view.delivery_packages].reverse()[0];

  const updateView = (value: WorkTaskView) => setView(normalizeWorkTaskView(value));

  const taskAction = (action: string) => runOperation(`task-${action}`, async () => {
    updateView(await post<WorkTaskView>(`/api/bff/tasks/${encodeURIComponent(task.id)}/actions`, {action}));
  }, action === 'start' ? '当前 Stage 已开始。' : '任务状态已更新。');

  const reportStage = async (outputs: Partial<TaskStageOutput>[], checks?: Record<string, boolean>) => {
    if (!activeRun || !currentStage) throw new Error('当前 StageRun 不可用。');
    const value = await post<WorkTaskView>(`/api/bff/tasks/${encodeURIComponent(task.id)}/stages/${encodeURIComponent(activeRun.stage_id)}/report`, {
      stage_run_id: activeRun.id,
      stage_id: activeRun.stage_id,
      status: 'completed',
      outputs,
      checks: checks || Object.fromEntries(currentStage.checks.map(check => [check, true]))
    });
    updateView(value);
  };

  const reportGenericStage = () => runOperation('report-stage', async () => {
    if (!currentStage) throw new Error('当前 Stage 不可用。');
    const outputs = currentStage.required_output_types.flatMap((requirement, index) => {
      const objectID = (outputIDs[`${requirement.output_type}:${index}`] || '').trim();
      return objectID ? [{output_type: requirement.output_type, object_id: objectID, role: requirement.role || 'primary'}] : [];
    });
    if (outputs.length < currentStage.required_output_types.length) throw new Error('请填写当前 Stage 要求的全部规范对象 ID。');
    await reportStage(outputs);
    setOutputIDs({});
  }, 'Stage 规范输出已由服务端核验并记录。');

  const saveScript = () => runOperation('save-script', async () => {
    let content: unknown;
    try { content = JSON.parse(scriptJSON); } catch { throw new Error('剧本正文必须是有效 JSON。'); }
    const revision = await post<{id: string; revision_no: number}>(`/api/bff/tasks/${encodeURIComponent(task.id)}/revisions`, {
      content_type: task.content_type,
      schema_version: 'contentcloud.marketing_video_script/1.0',
      content,
      knowledge_snapshot_ids: view.knowledge_snapshots.map(snapshot => snapshot.id),
      evidence_summary: {source_revision_ids: view.source_revisions.map(source => source.id)},
      rights_summary: {checked: true}
    });
    await reportStage([{output_type: 'submission_revision', object_id: revision.id, object_version: revision.revision_no, role: 'primary'}]);
    await reload();
  }, '剧本 Revision 已保存并进入审核。');

  const uploadStoryboardAsset = (asset: StoryboardAsset, file?: File) => runOperation(`upload-${asset.id}`, async () => {
    if (!file || !storyboardSnapshot) throw new Error('请选择与锁定分镜一致的素材文件。');
    const form = new FormData();
    form.set('snapshot_id', storyboardSnapshot.id);
    form.set('asset_id', asset.id);
    form.set('file', file);
    await upload(`/api/bff/tasks/${encodeURIComponent(task.id)}/storyboard-artifacts`, form);
    await reload();
  }, `${asset.id} 已通过摘要校验并登记。`);

  const createMediaJob = () => runOperation('create-media-job', async () => {
    if (!activeRun || !storyboardSnapshot) throw new Error('当前没有可用于生成的已批准分镜快照。');
    await post(`/api/bff/tasks/${encodeURIComponent(task.id)}/media-jobs`, {
      stage_run_id: activeRun.id,
      storyboard_snapshot_id: storyboardSnapshot.id,
      provider_id: 'fake',
      profile_version: '1.0.0',
      mode: 'image_to_video',
      aspect_ratio: '9:16',
      duration_seconds: 15
    });
    await reload();
  }, '视频 Job 已创建，Worker 将异步处理。');

  const approveCost = (job: MediaGenerationJob) => runOperation(`cost-${job.id}`, async () => {
    await post(`/api/bff/media-jobs/${encodeURIComponent(job.id)}/approve-cost`, {expected_version: job.row_version});
    await reload();
  }, '生成费用已确认。');

  const cancelJob = (job: MediaGenerationJob) => runOperation(`cancel-${job.id}`, async () => {
    await post(`/api/bff/media-jobs/${encodeURIComponent(job.id)}/cancel`, {expected_version: job.row_version});
    await reload();
  }, '视频 Job 已取消。');

  const finishGeneration = (job: MediaGenerationJob, artifact: Artifact) => runOperation('finish-generation', async () => {
    await reportStage([
      {output_type: 'generation_job', object_id: job.id, object_version: job.row_version, role: 'primary'},
      {output_type: 'artifact', object_id: artifact.id, role: 'preview'}
    ]);
  }, '生成结果已核验，进入成片质检。');

  const decideReview = (review: MediaReview, decision: 'approved' | 'changes_requested' | 'rejected') => runOperation(`review-${review.id}-${decision}`, async () => {
    const checks = review.review_kind === 'final' ? {'media.final': true, 'offer.valid': true, 'rights.references': true} : {'media.content': true};
    updateView(await post<WorkTaskView>(`/api/bff/media-reviews/${encodeURIComponent(review.id)}/decide`, {
      expected_version: review.row_version,
      decision,
      reason: decision === 'approved' ? (review.review_kind === 'final' ? '最终成片批准' : '画面、节奏与剧本一致') : '需要修改后重新提交',
      selected: decision === 'approved',
      checks
    }));
  }, decision === 'approved' ? '审核已批准。' : '修改决定已记录。');

  const finishTakeReview = (review: MediaReview) => runOperation('finish-take-review', async () => {
    await reportStage([{output_type: 'media_review', object_id: review.id, object_version: review.row_version, role: 'selected_take'}]);
  }, '选中 Take 已固定，进入 Gate。');

  const createFinalRender = () => runOperation('create-final-render', async () => {
    if (!activeRun || !selectedReview) throw new Error('没有已批准并选中的 Take。');
    await post(`/api/bff/tasks/${encodeURIComponent(task.id)}/final-render`, {stage_run_id: activeRun.id, selected_review_id: selectedReview.id});
    await reload();
  }, '独立最终成片已生成，等待最终批准。');

  const finishPostproduction = (review: MediaReview, artifact: Artifact) => runOperation('finish-postproduction', async () => {
    await reportStage([
      {output_type: 'artifact', object_id: artifact.id, role: 'final'},
      {output_type: 'media_review', object_id: review.id, object_version: review.row_version, role: 'final'}
    ]);
  }, '最终成片已固定，进入 Gate。');

  const buildDeliveryPackage = () => runOperation('build-package', async () => {
    if (!finalReview || finalReview.status !== 'approved') throw new Error('最终成片尚未批准。');
    await post(`/api/bff/tasks/${encodeURIComponent(task.id)}/delivery-package`, {final_review_id: finalReview.id});
    await reload();
  }, 'DeliveryPackage 已由服务端构建。');

  const finishDeliveryStage = (pkg: DeliveryPackage) => runOperation('finish-delivery-stage', async () => {
    await reportStage([{output_type: 'delivery_package', object_id: pkg.id, role: 'final'}]);
  }, '交付包 Stage 已完成。');

  const deliverTask = () => runOperation('deliver-task', async () => {
    if (!latestPackage) throw new Error('当前没有 ready DeliveryPackage。');
    const revision = [...view.revisions].reverse().find(value => value.status === 'accepted');
    if (!revision) throw new Error('当前没有已批准的剧本 SubmissionRevision，不能正式交付。');
    await post(`/api/bff/tasks/${encodeURIComponent(task.id)}/deliveries`, {revision_id: revision.id, delivery_package_id: latestPackage.id, destination: deliveryDestination, deliver: true});
    await reload();
  }, '营销视频已完成完整性交付。');

  const decideGate = (gateID: string, decision: 'approved' | 'rejected') => runOperation(`gate-${gateID}-${decision}`, async () => {
    updateView(await post<WorkTaskView>(`/api/bff/tasks/${encodeURIComponent(task.id)}/gates/${encodeURIComponent(gateID)}/decide`, {decision, reason: decision === 'approved' ? '工作台审核通过' : '需要修改后重新提交'}));
  }, decision === 'approved' ? 'Gate 已通过。' : 'Gate 已退回。');

  const downloadArtifact = (artifact: Artifact) => runOperation(`download-${artifact.id}`, async () => {
    const result = await download(`/api/bff/artifacts/${encodeURIComponent(artifact.id)}/download`);
    const url = URL.createObjectURL(result.blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = result.fileName || artifact.file_name;
    anchor.click();
    URL.revokeObjectURL(url);
  }, `${artifact.file_name} 已开始下载。`);

  const showTab = (value: ProductionTab) => { setManualTab(true); setTab(value); };
  const pendingGate = view.gates.find(gate => gate.status === 'pending');
  const completedStages = view.stage_runs.filter(run => run.status === 'completed').length;

  return <div className="workos-page task-production-page">
    <button className="back-link" onClick={() => navigate(projectID ? `/projects/${projectID}/tasks` : '/workspace/tasks')}><ArrowLeft size={15}/>返回任务列表</button>
    <header className="task-detail-header production-header">
      <div>
        <span className="eyebrow">{view.project.brand_name} / 营销视频</span>
        <h1>{task.title}</h1>
        <p>{view.sop.name} v{view.sop.version} · 更新于 {formatDateTime(task.updated_at)}</p>
      </div>
      <div className="task-header-actions"><StatusText value={task.status}/>{['ready', 'needs_input'].includes(task.status) && <Button disabled={Boolean(busy)} onClick={() => void taskAction('start')}><PlayCircle size={15}/>开始当前 Stage</Button>}{task.status === 'paused' && <Button disabled={Boolean(busy)} onClick={() => void taskAction('resume')}><PlayCircle size={15}/>恢复</Button>}{task.status === 'blocked' && <Button disabled={Boolean(busy)} onClick={() => void taskAction('retry')}><RefreshCw size={15}/>重试</Button>}{task.status === 'waiting_gate' && <Button onClick={() => document.getElementById('production-gate')?.scrollIntoView({behavior: 'smooth'})}><ClipboardCheck size={15}/>处理 Gate</Button>}{task.status === 'accepted' && <Button onClick={() => showTab('delivery')}><PackageCheck size={15}/>完成交付</Button>}</div>
    </header>

    {notice && <div className={`workos-notice ${notice.includes('失败') || notice.includes('不一致') ? 'is-error' : 'is-info'}`}><CircleHelp size={16}/><span>{notice}</span><IconButton label="关闭提示" onClick={() => setNotice('')}><X size={15}/></IconButton></div>}

    <section className="production-progress" aria-label="生产进度">
      <div className="production-progress-heading"><div><span>当前 Stage</span><strong>{currentStage?.name || '流程已完成'}</strong></div><b>{completedStages} / {view.sop.stages.length}</b></div>
      <div className="stage-timeline production-timeline">{view.sop.stages.map((stage, index) => {const run = view.stage_runs.find(value => value.stage_id === stage.stage_id); const done = run?.status === 'completed'; const current = stage.stage_id === task.current_stage_id; return <button key={stage.stage_id} className={`timeline-step ${done ? 'is-done' : current ? 'is-current' : ''}`} onClick={() => showTab(tabForStage(stage.stage_id))}><span>{done ? <Check size={13}/> : index + 1}</span><div><strong>{stage.name}</strong><small>{done ? '已完成' : current ? (statusLabels[run?.status || ''] || run?.status || '当前') : '未开始'}</small></div></button>;})}</div>
      <div className="production-next-action"><Workflow size={15}/><span>{task.next_action}</span>{task.status === 'running' && <Button variant="secondary" disabled={Boolean(busy)} onClick={() => void taskAction('pause')}>暂停</Button>}</div>
    </section>

    <nav className="production-tabs" role="tablist" aria-label="任务生产视图">{productionTabs.map(item => <button key={item.id} role="tab" aria-selected={tab === item.id} className={tab === item.id ? 'is-active' : ''} onClick={() => showTab(item.id)}><item.icon size={15}/><span>{item.label}</span><TabCount tab={item.id} view={view}/></button>)}</nav>

    <div className="task-detail-layout production-layout">
      <main>
        {tab === 'content' && <ContentPanel view={view} scriptJSON={scriptJSON} setScriptJSON={setScriptJSON} onSaveScript={() => void saveScript()} busy={busy} currentStage={currentStage} outputIDs={outputIDs} setOutputIDs={setOutputIDs} onReport={() => void reportGenericStage()} onNavigateKnowledge={() => navigate(`/projects/${encodeURIComponent(view.project.id)}/knowledge`)}/>}
        {tab === 'storyboard' && <StoryboardPanel snapshot={storyboardSnapshot} storyboard={storyboard} artifacts={view.artifacts} busy={busy} onUpload={uploadStoryboardAsset} currentStage={currentStage} outputIDs={outputIDs} setOutputIDs={setOutputIDs} onReport={() => void reportGenericStage()}/>}
        {tab === 'generation' && <GenerationPanel jobs={view.media_jobs} attempts={view.provider_attempts} artifacts={view.artifacts} busy={busy} videoErrors={videoErrors} setVideoErrors={setVideoErrors} canCreate={task.status === 'running' && task.current_stage_id === 'generation'} onCreate={() => void createMediaJob()} onApproveCost={approveCost} onCancel={cancelJob} onFinish={finishGeneration} onDownload={downloadArtifact} onReload={() => void reload()}/>}
        {tab === 'review' && <ReviewPanel reviews={view.media_reviews} artifacts={view.artifacts} selectedReview={selectedReview} finalReview={finalReview} finalArtifact={finalArtifact} taskStage={task.current_stage_id} taskStatus={task.status} busy={busy} videoErrors={videoErrors} setVideoErrors={setVideoErrors} onDecide={decideReview} onFinishTake={finishTakeReview} onCreateFinal={() => void createFinalRender()} onFinishFinal={finishPostproduction} onDownload={downloadArtifact}/>}
        {tab === 'delivery' && <DeliveryPanel packages={view.delivery_packages} deliveries={view.deliveries} finalArtifact={finalArtifact} taskStatus={task.status} taskStage={task.current_stage_id} busy={busy} destination={deliveryDestination} setDestination={setDeliveryDestination} onBuild={() => void buildDeliveryPackage()} onFinishStage={finishDeliveryStage} onDeliver={() => void deliverTask()} onDownload={downloadArtifact}/>}

        {task.status === 'running' && currentStage && !['script', 'generation', 'review', 'postproduction', 'delivery', 'storyboard'].includes(currentStage.stage_id) && <TypedStageEditor stage={currentStage} outputIDs={outputIDs} setOutputIDs={setOutputIDs} busy={busy} onReport={() => void reportGenericStage()}/>}

        <section className="workos-section production-gate" id="production-gate">
          <SectionHeader kicker="Gate" title="审核决定" action={<span className="section-count">{view.gates.length} 条记录</span>}/>
          {view.gates.length === 0 ? <Empty title="尚无 Gate 记录" detail="当前 Stage 完成后，SOP 会创建需要的审核决定。"/> : <div className="production-record-list">{view.gates.map(gate => <article key={gate.id}><span className="object-mark is-review"><ClipboardCheck size={15}/></span><div><strong>{gate.gate_id}</strong><small>{gate.gate_mode} · {gate.reason || '等待决定'} · {gate.input_refs.length} 个规范输入</small></div><StatusText value={gate.status}/>{gate.status === 'pending' && <div className="record-actions"><Button variant="secondary" disabled={Boolean(busy)} onClick={() => void decideGate(gate.id, 'rejected')}>退回</Button><Button disabled={Boolean(busy)} onClick={() => void decideGate(gate.id, 'approved')}><Check size={14}/>通过</Button></div>}</article>)}</div>}
        </section>
      </main>

      <aside className="task-side-panel production-facts">
        <SideFact label="任务状态" value={statusLabels[task.status] || task.status}/>
        <SideFact label="当前阶段" value={currentStage?.name || '已完成'} icon={Workflow}/>
        <SideFact label="来源" value={`${view.source_revisions.length} 个 Revision`} icon={FileInput}/>
        <SideFact label="知识快照" value={`${view.knowledge_snapshots.length} 个`} icon={ShieldCheck}/>
        <SideFact label="剧本" value={`${view.revisions.length} 个 Revision`} icon={FileCheck2}/>
        <SideFact label="分镜" value={`${storyboard?.shots.length || genericStoryboardShots(storyboardSnapshot).length} 个镜头`} icon={ImageIcon}/>
        <SideFact label="生成 Job" value={`${view.media_jobs.length} 个`} icon={Video}/>
        <SideFact label="媒体审核" value={`${view.media_reviews.length} 条`} icon={ClipboardCheck}/>
        <SideFact label="交付文件" value={`${latestPackage?.manifest.length || 0} 个`} icon={PackageCheck}/>
        <div className="side-divider"></div>
        <div className="side-block"><span>SOP digest</span><strong>v{task.sop_version}</strong><small>{task.sop_digest}</small></div>
        <div className="side-block"><span>当前输出契约</span><strong>{currentStage?.completion_policy || '兼容模式'}</strong><small>{currentStage?.required_output_types.map(value => `${value.output_type}:${value.role || 'primary'}`).join(' · ') || '无类型化要求'}</small></div>
        {pendingGate && <div className="side-block is-warning"><span>待处理 Gate</span><strong>{pendingGate.gate_id}</strong><small>{pendingGate.gate_mode}</small></div>}
      </aside>
    </div>
  </div>;
}

function ContentPanel({view, scriptJSON, setScriptJSON, onSaveScript, busy, currentStage, outputIDs, setOutputIDs, onReport, onNavigateKnowledge}: {view: WorkTaskView; scriptJSON: string; setScriptJSON: (value: string) => void; onSaveScript: () => void; busy: string; currentStage?: StageDefinition; outputIDs: Record<string,string>; setOutputIDs: (value: Record<string,string>) => void; onReport: () => void; onNavigateKnowledge: () => void}) {
  const revisions = [...view.revisions].sort((a, b) => b.revision_no - a.revision_no);
  return <>
    <section className="workos-section production-content-summary">
      <SectionHeader kicker="来源与知识" title="服务端已固定输入" action={<Button variant="secondary" onClick={onNavigateKnowledge}><FileInput size={14}/>打开知识库</Button>}/>
      <div className="production-input-grid"><div><span>来源 Revision</span><strong>{view.source_revisions.length}</strong><small>{view.source_revisions.map(value => value.file_name).join('、') || '尚未绑定'}</small></div><div><span>知识对象</span><strong>{view.knowledge_snapshots.reduce((sum, value) => sum + value.objects.length, 0)}</strong><small>{view.knowledge_snapshots.map(value => `Pack v${value.pack_version}`).join('、') || '尚未绑定'}</small></div><div><span>权利检查</span><strong>{view.knowledge_snapshots.length ? '已快照' : '待处理'}</strong><small>{view.stage_outputs.filter(value => value.output_type === 'knowledge_snapshot').map(value => value.object_digest.slice(0, 18)).join('、')}</small></div></div>
      {view.knowledge_snapshots.flatMap(snapshot => snapshot.objects).length > 0 && <div className="knowledge-fact-strip">{view.knowledge_snapshots.flatMap(snapshot => snapshot.objects).slice(0, 6).map(value => <span key={`${value.id}-${value.version}`}><b>{value.title}</b>{value.statement}</span>)}</div>}
    </section>

    <section className="workos-section script-workbench">
      <SectionHeader kicker="短视频剧本" title={revisions[0] ? `Revision ${revisions[0].revision_no}` : '尚未生成剧本'} action={<span className="section-count">{revisions.length} 个版本</span>}/>
      {revisions.length === 0 ? <Empty title="没有剧本正文" detail="在剧本 Stage 保存第一个 Revision 后，正文和场景会显示在这里。"/> : <div className="script-revisions">{revisions.map(revision => <ScriptRevision key={revision.id} revision={revision}/>)}</div>}
    </section>

    {view.task.status === 'running' && currentStage?.stage_id === 'script' && <section className="workos-section script-editor"><SectionHeader kicker="新 Revision" title="编辑结构化剧本"/><Field label="剧本 JSON"><textarea rows={14} value={scriptJSON} onChange={event => setScriptJSON(event.target.value)} spellCheck={false}/></Field><div className="production-editor-actions"><Button disabled={Boolean(busy)} onClick={onSaveScript}><FileCheck2 size={15}/>{busy === 'save-script' ? '保存中...' : '保存并上报剧本'}</Button></div></section>}

    {view.task.status === 'running' && currentStage && ['sources', 'knowledge'].includes(currentStage.stage_id) && <TypedStageEditor stage={currentStage} outputIDs={outputIDs} setOutputIDs={setOutputIDs} busy={busy} onReport={onReport}/>}
  </>;
}

function ScriptRevision({revision}: {revision: WorkTaskView['revisions'][number]}) {
  const content = record(revision.content);
  const scenes = list(content.scenes).map(record);
  return <article className="script-revision"><header><div><strong>{text(content.title, `Revision ${revision.revision_no}`)}</strong><small>{revision.schema_version} · {formatDateTime(revision.created_at)}</small></div><StatusText value={revision.status}/></header>{scenes.length > 0 ? <ol>{scenes.map((scene, index) => <li key={text(scene.id, String(index))}><span>{String(index + 1).padStart(2, '0')}</span><div><strong>{text(scene.visual, text(scene.scene, `场景 ${index + 1}`))}</strong><p>{text(scene.voiceover, text(scene.narration, '无旁白'))}</p><small>{text(scene.duration_seconds, '?')} 秒 · {text(scene.on_screen_text, '无屏幕文字')}</small></div></li>)}</ol> : <pre>{JSON.stringify(content, null, 2)}</pre>}<footer><code>{revision.content_hash}</code><span>知识快照 {revision.knowledge_snapshot_ids.length}</span></footer></article>;
}

function StoryboardPanel({snapshot, storyboard, artifacts, busy, onUpload, currentStage, outputIDs, setOutputIDs, onReport}: {snapshot?: ApprovedSnapshot; storyboard?: StoryboardPackage; artifacts: Artifact[]; busy: string; onUpload: (asset: StoryboardAsset, file?: File) => void; currentStage?: StageDefinition; outputIDs: Record<string,string>; setOutputIDs: (value: Record<string,string>) => void; onReport: () => void}) {
  const shots = storyboard?.shots || genericStoryboardShots(snapshot) as unknown as StoryboardShot[];
  const artifactForAsset = (assetID: string) => artifacts.find(artifact => artifact.metadata.storyboard_asset_id === assetID);
  return <>
    <section className="workos-section storyboard-workbench">
      <SectionHeader kicker="Storyboard" title={storyboard?.id || text(record(snapshot?.canonical_content).title, '尚未锁定分镜')} action={snapshot ? <StatusText value={storyboard?.status || 'approved'}/> : undefined}/>
      {!snapshot ? <Empty title="没有已批准分镜" detail="本地 StoryboardPackage 通过审核后，在当前 Stage 上报 ApprovedSnapshot ID。"/> : <><div className="storyboard-lock"><span><ShieldCheck size={15}/>Locked digest</span><code>{storyboard?.locked_digest || snapshot.subject_hash}</code><small>{shots.length} 个镜头 · {storyboard?.assets.length || snapshot.artifacts.length} 个素材</small></div>{shots.length === 0 ? <Empty title="分镜快照没有镜头正文" detail="当前 ApprovedSnapshot 只包含摘要。"/> : <div className="shot-grid">{shots.map((shot, index) => {const firstID = shot.first_frame_artifact_id || ''; const asset = storyboard?.assets.find(value => value.id === firstID); const artifact = artifactForAsset(firstID); return <article className="shot-card" key={shot.shot_id || index}><div className="shot-media">{artifact ? <img src={`/api/bff/artifacts/${encodeURIComponent(artifact.id)}/download`} alt={`${shot.shot_id || `镜头 ${index + 1}`} 首帧`} loading="lazy"/> : <div><ImageIcon size={24}/><span>{asset ? '素材字节待登记' : '无首帧 Artifact'}</span></div>}</div><header><span>{shot.shot_id || `SHOT-${index + 1}`}</span><b>{Math.max(0, ((shot.end_ms || 0) - (shot.start_ms || 0)) / 1000).toFixed(1)}s</b></header><strong>{shot.scene || shot.subject || text((shot as unknown as Record<string,unknown>).visual, '未命名画面')}</strong><p>{shot.action || shot.image_prompt_zh || '未提供画面动作'}</p><small>{shot.camera || shot.composition || '机位待定'}</small>{asset && !artifact && <label className={`button secondary storyboard-upload ${busy === `upload-${asset.id}` ? 'is-disabled' : ''}`}><Upload size={14}/>{busy === `upload-${asset.id}` ? '校验中...' : '上传锁定素材'}<input type="file" accept={asset.media_type} disabled={Boolean(busy)} onChange={event => {const file = event.target.files?.[0]; onUpload(asset, file); event.target.value = '';}}/></label>}</article>;})}</div>}</>}
    </section>
    {storyboard?.assets.length ? <section className="workos-section"><SectionHeader kicker="素材清单" title="锁定图片、音视频与权利引用"/><div className="artifact-table">{storyboard.assets.map(asset => {const artifact = artifactForAsset(asset.id); return <div key={asset.id}><span className="object-mark is-production">{asset.media_type.startsWith('image/') ? <ImageIcon size={15}/> : <Film size={15}/>}</span><div><strong>{asset.path}</strong><small>{asset.role} · {asset.shot_id || '公共素材'} · {formatBytes(asset.byte_size)}</small></div><StatusText value={artifact ? 'validated' : 'pending'}/><code>{asset.sha256.slice(0, 14)}...</code></div>;})}</div></section> : null}
    {currentStage?.stage_id === 'storyboard' && <TypedStageEditor stage={currentStage} outputIDs={outputIDs} setOutputIDs={setOutputIDs} busy={busy} onReport={onReport}/>}
  </>;
}

function GenerationPanel({jobs, attempts, artifacts, busy, videoErrors, setVideoErrors, canCreate, onCreate, onApproveCost, onCancel, onFinish, onDownload, onReload}: {jobs: MediaGenerationJob[]; attempts: WorkTaskView['provider_attempts']; artifacts: Artifact[]; busy: string; videoErrors: Record<string,boolean>; setVideoErrors: (value: Record<string,boolean>) => void; canCreate: boolean; onCreate: () => void; onApproveCost: (job: MediaGenerationJob) => void; onCancel: (job: MediaGenerationJob) => void; onFinish: (job: MediaGenerationJob, artifact: Artifact) => void; onDownload: (artifact: Artifact) => void; onReload: () => void}) {
  return <section className="workos-section generation-workbench"><SectionHeader kicker="Media Worker" title="视频生成" action={<div className="section-actions"><IconButton label="刷新 Job 状态" onClick={onReload}><RefreshCw size={15}/></IconButton>{canCreate && <Button onClick={onCreate} disabled={Boolean(busy)}><Play size={14}/>{busy === 'create-media-job' ? '创建中...' : '创建生成 Job'}</Button>}</div>}/>{jobs.length === 0 ? <Empty title="还没有视频 Job" detail="分镜素材完整后，可以创建第一个生成 Job。"/> : <div className="take-grid">{[...jobs].reverse().map(job => {const attempt = [...attempts].reverse().find(value => value.generation_job_id === job.id); const artifact = artifacts.find(value => value.metadata.generation_job_id === job.id && value.kind === 'generated_video'); const cancellable = ['awaiting_cost_approval', 'queued', 'submitting', 'submitted', 'generating', 'retry_wait', 'retryable_failed', 'output_invalid'].includes(job.state); return <article className="take-card" key={job.id}><MediaPreview artifact={artifact} failed={Boolean(artifact && videoErrors[artifact.id])} onError={() => artifact && setVideoErrors({...videoErrors, [artifact.id]: true})}/><header><div><span>Take {job.id.slice(0, 8)}</span><strong>{job.model}</strong></div><StatusText value={job.state}/></header><dl><div><dt>Provider</dt><dd>{job.provider_id} / {job.profile_version}</dd></div><div><dt>规格</dt><dd>{job.aspect_ratio} · {job.duration_seconds}s</dd></div><div><dt>费用</dt><dd>{formatCost(job.actual_cost_minor || job.estimated_cost_minor, job.currency)}</dd></div><div><dt>Attempt</dt><dd>{job.attempt_count} / {job.max_attempts}</dd></div></dl>{attempt && <div className="attempt-line"><Clock3 size={13}/><span>{attempt.provider_state} · {attempt.provider_request_id || '未返回 request id'}</span></div>}{job.error_code && <div className="media-error"><AlertCircle size={14}/><span>{job.error_code} · {job.error_detail_safe}</span></div>}<footer>{job.state === 'awaiting_cost_approval' && <Button disabled={Boolean(busy)} onClick={() => onApproveCost(job)}><Check size={14}/>确认 {formatCost(job.estimated_cost_minor, job.currency)}</Button>}{artifact && <Button variant="secondary" disabled={Boolean(busy)} onClick={() => onDownload(artifact)}><Download size={14}/>下载</Button>}{job.state === 'succeeded' && artifact && <Button disabled={Boolean(busy)} onClick={() => onFinish(job, artifact)}><CheckCircle2 size={14}/>完成生成阶段</Button>}{cancellable && <Button variant="ghost" disabled={Boolean(busy)} onClick={() => onCancel(job)}><X size={14}/>取消</Button>}</footer></article>;})}</div>}</section>;
}

function ReviewPanel({reviews, artifacts, selectedReview, finalReview, finalArtifact, taskStage, taskStatus, busy, videoErrors, setVideoErrors, onDecide, onFinishTake, onCreateFinal, onFinishFinal, onDownload}: {reviews: MediaReview[]; artifacts: Artifact[]; selectedReview?: MediaReview; finalReview?: MediaReview; finalArtifact?: Artifact; taskStage: string; taskStatus: string; busy: string; videoErrors: Record<string,boolean>; setVideoErrors: (value: Record<string,boolean>) => void; onDecide: (review: MediaReview, decision: 'approved'|'changes_requested'|'rejected') => void; onFinishTake: (review: MediaReview) => void; onCreateFinal: () => void; onFinishFinal: (review: MediaReview, artifact: Artifact) => void; onDownload: (artifact: Artifact) => void}) {
  const contentReviews = reviews.filter(value => value.review_kind === 'content');
  const technical = reviews.filter(value => value.review_kind === 'technical');
  return <>
    <section className="workos-section review-workbench"><SectionHeader kicker="Take Review" title="成片质检与选择" action={<span className="section-count">技术检查 {technical.filter(value => value.status === 'approved').length}/{technical.length}</span>}/>{contentReviews.length === 0 ? <Empty title="没有待审 Take" detail="Media Worker 生成并校验视频后，会创建内容审核。"/> : <div className="take-grid">{contentReviews.map(review => {const artifact = artifacts.find(value => value.id === review.subject_artifact_id); const technicalReview = reviews.find(value => value.review_kind === 'technical' && value.subject_artifact_id === review.subject_artifact_id); return <article className={`take-card ${review.selected ? 'is-selected' : ''}`} key={review.id}><MediaPreview artifact={artifact} failed={Boolean(artifact && videoErrors[artifact.id])} onError={() => artifact && setVideoErrors({...videoErrors, [artifact.id]: true})}/><header><div><span>{review.selected ? 'Selected Take' : 'Candidate Take'}</span><strong>{artifact?.file_name || review.subject_artifact_id}</strong></div><StatusText value={review.status}/></header><div className="review-checks"><span><ShieldCheck size={14}/>技术检查 <b>{technicalReview?.status === 'approved' ? '通过' : '待处理'}</b></span><span><FileCheck2 size={14}/>内容检查 <b>{review.status === 'approved' ? '通过' : '待决定'}</b></span></div>{review.decision_reason && <p className="decision-reason">{review.decision_reason}</p>}<footer>{review.status === 'pending' && <><Button variant="secondary" disabled={Boolean(busy)} onClick={() => onDecide(review, 'changes_requested')}>要求修改</Button><Button disabled={Boolean(busy)} onClick={() => onDecide(review, 'approved')}><Check size={14}/>批准并选中</Button></>}{taskStage === 'review' && taskStatus === 'running' && review.status === 'approved' && review.selected && <Button disabled={Boolean(busy)} onClick={() => onFinishTake(review)}><CheckCircle2 size={14}/>完成 Take 选择</Button>}{artifact && <Button variant="ghost" onClick={() => onDownload(artifact)}><Download size={14}/>下载</Button>}</footer></article>;})}</div>}</section>

    <section className="workos-section final-render-workbench"><SectionHeader kicker="Post Production" title="最终成片" action={taskStage === 'postproduction' && taskStatus === 'running' && selectedReview && !finalArtifact ? <Button disabled={Boolean(busy)} onClick={onCreateFinal}><Film size={14}/>{busy === 'create-final-render' ? '渲染中...' : '生成最终成片'}</Button> : undefined}/>{!finalArtifact ? <Empty title="尚未生成最终成片" detail={selectedReview ? '已选 Take 可进入独立最终渲染。' : '先批准并选中一个 Take。'}/> : <div className="final-render-layout"><MediaPreview artifact={finalArtifact} failed={Boolean(videoErrors[finalArtifact.id])} onError={() => setVideoErrors({...videoErrors, [finalArtifact.id]: true})}/><div className="final-render-facts"><span>Artifact</span><strong>{finalArtifact.file_name}</strong><code>{finalArtifact.sha256}</code><dl><div><dt>渲染器</dt><dd>{text(finalArtifact.metadata.renderer, 'deterministic')}</dd></div><div><dt>大小</dt><dd>{formatBytes(finalArtifact.byte_size)}</dd></div><div><dt>最终审核</dt><dd><StatusText value={finalReview?.status || 'pending'}/></dd></div></dl><div className="final-actions">{finalReview?.status === 'pending' && <><Button variant="secondary" disabled={Boolean(busy)} onClick={() => onDecide(finalReview, 'changes_requested')}>要求修改</Button><Button disabled={Boolean(busy)} onClick={() => onDecide(finalReview, 'approved')}><Check size={14}/>批准最终成片</Button></>}{taskStage === 'postproduction' && taskStatus === 'running' && finalReview?.status === 'approved' && <Button disabled={Boolean(busy)} onClick={() => onFinishFinal(finalReview, finalArtifact)}><CheckCircle2 size={14}/>完成后期阶段</Button>}<Button variant="secondary" onClick={() => onDownload(finalArtifact)}><Download size={14}/>下载成片</Button></div></div></div>}</section>
  </>;
}

function DeliveryPanel({packages, deliveries, finalArtifact, taskStatus, taskStage, busy, destination, setDestination, onBuild, onFinishStage, onDeliver, onDownload}: {packages: DeliveryPackage[]; deliveries: WorkTaskView['deliveries']; finalArtifact?: Artifact; taskStatus: string; taskStage: string; busy: string; destination: string; setDestination: (value:string) => void; onBuild: () => void; onFinishStage: (pkg:DeliveryPackage) => void; onDeliver: () => void; onDownload: (artifact:Artifact) => void}) {
  const latestPackage = [...packages].reverse()[0];
  return <>
    <section className="workos-section delivery-workbench"><SectionHeader kicker="DeliveryPackage" title="服务端交付清单" action={taskStage === 'delivery' && taskStatus === 'running' && !latestPackage ? <Button disabled={Boolean(busy)} onClick={onBuild}><PackageCheck size={14}/>构建交付包</Button> : undefined}/>{!latestPackage ? <Empty title="还没有 DeliveryPackage" detail="最终成片批准后，服务端会固定 manifest。"/> : <><div className="package-heading"><div><span>Package ID</span><strong>{latestPackage.id}</strong><small>{formatDateTime(latestPackage.created_at)} · {latestPackage.status}</small></div><StatusText value={latestPackage.status}/></div><div className="delivery-manifest">{latestPackage.manifest.map(artifact => <article key={artifact.id}><span className="object-mark is-production"><Film size={15}/></span><div><strong>{artifact.file_name}</strong><small>{artifact.media_type} · {formatBytes(artifact.byte_size)}</small><code>{artifact.sha256}</code></div><StatusText value={artifact.kind === 'final_render' ? 'complete' : artifact.kind}/><IconButton label={`下载 ${artifact.file_name}`} onClick={() => onDownload(artifact)}><Download size={15}/></IconButton></article>)}</div>{taskStage === 'delivery' && taskStatus === 'running' && <div className="delivery-stage-action"><Button disabled={Boolean(busy)} onClick={() => onFinishStage(latestPackage)}><CheckCircle2 size={14}/>完成交付包 Stage</Button></div>}</>}</section>
    <section className="workos-section"><SectionHeader kicker="Task Delivery" title="正式交付" action={taskStatus === 'delivered' ? <StatusText value="delivered"/> : undefined}/>{deliveries.length > 0 ? <div className="production-record-list">{deliveries.map(delivery => <article key={delivery.id}><span className="object-mark is-success"><PackageCheck size={15}/></span><div><strong>{delivery.destination}</strong><small>{delivery.manifest.length} 个文件 · {delivery.delivery_package_id || '无 Package'}</small></div><StatusText value={delivery.integrity_status}/><StatusText value={delivery.status}/></article>)}</div> : <Empty title="尚未正式交付" detail="所有 Stage 完成并接受任务后，可将完整包交付到指定目的地。"/>}{taskStatus === 'accepted' && latestPackage && finalArtifact && <div className="delivery-form"><Field label="交付目的地"><input value={destination} onChange={event => setDestination(event.target.value)}/></Field><Button disabled={Boolean(busy)} onClick={onDeliver}><PackageCheck size={15}/>{busy === 'deliver-task' ? '交付中...' : '确认完整性交付'}</Button></div>}</section>
  </>;
}

function TypedStageEditor({stage, outputIDs, setOutputIDs, busy, onReport}: {stage: StageDefinition; outputIDs: Record<string,string>; setOutputIDs: (value: Record<string,string>) => void; busy: string; onReport: () => void}) {
  return <section className="workos-section typed-stage-editor"><SectionHeader kicker="类型化输出" title={`完成 ${stage.name}`}/><div className="typed-output-grid">{stage.required_output_types.map((requirement, index) => {const key = `${requirement.output_type}:${index}`; return <Field key={key} label={outputLabels[requirement.output_type] || requirement.output_type} hint={`${requirement.role || 'primary'} · 最低状态 ${requirement.min_status || 'validated'}`}><input value={outputIDs[key] || ''} onChange={event => setOutputIDs({...outputIDs, [key]: event.target.value})} placeholder="服务端规范对象 ID"/></Field>;})}</div><div className="stage-check-list">{stage.checks.map(check => <span key={check}><Check size={13}/>{check}</span>)}</div><div className="production-editor-actions"><Button disabled={Boolean(busy) || stage.required_output_types.some((requirement, index) => !(outputIDs[`${requirement.output_type}:${index}`] || '').trim())} onClick={onReport}><CheckCircle2 size={14}/>{busy === 'report-stage' ? '核验中...' : '核验并完成 Stage'}</Button></div></section>;
}

function MediaPreview({artifact, failed, onError}: {artifact?: Artifact; failed:boolean; onError: () => void}) {
  if (!artifact) return <div className="media-preview is-empty"><Video size={25}/><span>媒体 Artifact 待生成</span></div>;
  if (failed) return <div className="media-preview is-error"><AlertCircle size={24}/><span>当前 Fixture 无浏览器可解码轨道</span><code>{artifact.sha256.slice(0, 20)}...</code></div>;
  return <div className="media-preview"><video controls preload="metadata" playsInline onError={onError}><source src={`/api/bff/artifacts/${encodeURIComponent(artifact.id)}/download`} type={artifact.media_type}/></video><span className="media-kind">{artifact.kind === 'final_render' ? 'FINAL' : 'TAKE'}</span></div>;
}

function TabCount({tab, view}: {tab:ProductionTab;view:WorkTaskView}) {
  const value = tab === 'content' ? view.revisions.length : tab === 'storyboard' ? view.approved_snapshots.filter(snapshot => snapshot.submission_type === 'storyboard').length : tab === 'generation' ? view.media_jobs.length : tab === 'review' ? view.media_reviews.length : view.delivery_packages.length;
  return <b>{value}</b>;
}

function SectionHeader({kicker, title, action}: {kicker:string;title:string;action?:React.ReactNode}) {
  return <header className="workos-section-header"><div><span>{kicker}</span><h2>{title}</h2></div>{action}</header>;
}

function StatusText({value}: {value:string}) {
  return <span className={`status-text ${tone(value)}`}><i></i>{statusLabels[value] || value}</span>;
}

function SideFact({label, value, icon: Icon}: {label:string;value:string;icon?:typeof Workflow}) {
  return <div className="side-fact"><span>{label}</span><strong>{Icon && <Icon size={14}/>} {value}</strong></div>;
}
