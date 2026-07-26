import {
  CheckCircle2,
  ChevronRight,
  Clock3,
  Download,
  ExternalLink,
  FileCode2,
  Film,
  FlaskConical,
  GitBranch,
  History,
  Link2,
  MessageSquare,
  Play,
  RefreshCw,
  RotateCcw,
  Send,
  ShieldAlert,
  Sparkles,
  Undo2,
  UserRoundCheck,
  XCircle,
} from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { api, download, post } from '../api';
import type {
  Artifact,
  ArtifactOpenResult,
  ArtifactPresentation,
  Brief,
  Member,
  Project,
  ReviewComment,
  ReviewCycle,
  ReviewGrant,
  Run,
  Script,
  Shot,
} from '../types';
import { Banner, Button, Empty, Field, Modal, Status } from '../components/ui';
import { ProjectPage } from './OverviewView';

type CommentTarget = { shotID: string; shotIndex: number };
type ReviewDecision = 'approve_internal' | 'return';
type ChangeType = 'revision' | 'variant';

const changeFields = [
  { value: '/title', label: '剧本标题' },
  { value: '/shots/0', label: '开场镜头' },
  { value: '/shots/0/voiceover', label: '开场口播' },
  { value: '/creative_strategy/cta', label: 'CTA 文案' },
  { value: '/production_bible/visual_style_lock', label: '视觉风格锁' },
];

const defaultInvariants = [
  '/creative_strategy/primary_selling_point',
  '/creative_strategy/cta',
  '/production_bible/scene_lock',
  '/target_duration_seconds',
  '/aspect_ratio',
];

export function ScriptsView({ project }: { project: Project }) {
  const [scripts, setScripts] = useState<Script[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);
  const [briefs, setBriefs] = useState<Brief[]>([]);
  const [members, setMembers] = useState<Member[]>([]);
  const [selected, setSelected] = useState<string>();
  const [comments, setComments] = useState<ReviewComment[]>([]);
  const [cycles, setCycles] = useState<ReviewCycle[]>([]);
  const [grants, setGrants] = useState<ReviewGrant[]>([]);
  const [artifacts, setArtifacts] = useState<ArtifactPresentation[]>([]);
  const [openNotice, setOpenNotice] = useState('');
  const [commentTarget, setCommentTarget] = useState<CommentTarget>();
  const [reviewing, setReviewing] = useState<ReviewDecision>();
  const [changing, setChanging] = useState<ChangeType>();
  const [showGrants, setShowGrants] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const load = async () => {
    try {
      const [nextScripts, nextRuns, nextBriefs] = await Promise.all([
        api<Script[]>(`/api/bff/projects/${project.id}/scripts`),
        api<Run[]>(`/api/bff/projects/${project.id}/runs`),
        api<Brief[]>(`/api/bff/projects/${project.id}/briefs`),
      ]);
      setScripts(nextScripts);
      setRuns(nextRuns);
      setBriefs(nextBriefs);
      setSelected(previous => nextScripts.some(item => item.id === previous) ? previous : nextScripts[0]?.id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '剧本加载失败');
    }
  };

  useEffect(() => { load(); }, [project.id]);
  useEffect(() => {
    api<Member[]>('/api/bff/team/members').then(setMembers).catch(() => setMembers([]));
  }, [project.id]);

  const approvedBriefs = briefs.filter(item => item.status === 'approved');
  const approvedBrief = approvedBriefs[0];
  const active = scripts.find(item => item.id === selected) || scripts[0];
  const pending = runs.find(item => item.state === 'queued' || item.state === 'leased' || item.state === 'running');
  const versionByID = useMemo(() => new Map(scripts.map(item => [item.id, item.version])), [scripts]);

  const loadDetail = async (scriptID: string) => {
    try {
      const [nextComments, nextCycles, nextGrants, nextArtifacts] = await Promise.all([
        api<ReviewComment[]>(`/api/bff/scripts/${scriptID}/comments`),
        api<ReviewCycle[]>(`/api/bff/scripts/${scriptID}/review-cycles`),
        api<ReviewGrant[]>(`/api/bff/scripts/${scriptID}/review-grants`),
        api<ArtifactPresentation[]>(`/api/bff/scripts/${scriptID}/artifacts`),
      ]);
      setComments(nextComments);
      setCycles(nextCycles);
      setGrants(nextGrants);
      setArtifacts(nextArtifacts);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '剧本审核信息加载失败');
    }
  };

  useEffect(() => {
    if (!active) return;
    setComments([]);
    setCycles([]);
    setGrants([]);
    setArtifacts([]);
    loadDetail(active.id);
  }, [active?.id]);

  const generate = async () => {
    if (!approvedBrief) return;
    setBusy(true);
    setError('');
    try {
      await post(`/api/bff/briefs/${approvedBrief.id}/runs`, { idempotency_key: crypto.randomUUID() });
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '创建任务失败');
    } finally {
      setBusy(false);
    }
  };

  const review = async (decision: 'submit' | ReviewDecision, conclusion = '', assigneeUserID = '') => {
    if (!active) return;
    setBusy(true);
    setError('');
    try {
      await post(`/api/bff/scripts/${active.id}/review`, {
        decision,
        conclusion,
        assignee_user_id: assigneeUserID,
      });
      setReviewing(undefined);
      await load();
      await loadDetail(active.id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '审核失败');
    } finally {
      setBusy(false);
    }
  };

  const resolveComment = async (commentID: string) => {
    setError('');
    try {
      const resolved = await post<ReviewComment>(`/api/bff/comments/${commentID}/resolve`);
      setComments(current => current.map(item => item.id === resolved.id ? resolved : item));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '批注解决失败');
    }
  };

  const exportScript = async (format: string) => {
    if (!active) return;
    setBusy(true);
    setError('');
    try {
      const artifact = await post<Artifact>(`/api/bff/scripts/${active.id}/exports`, { format });
      const result = await download(`/api/bff/artifacts/${artifact.id}/download`);
      const href = URL.createObjectURL(result.blob);
      const link = document.createElement('a');
      link.href = href;
      link.download = result.fileName;
      link.click();
      URL.revokeObjectURL(href);
      await loadDetail(active.id);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '导出失败');
    } finally {
      setBusy(false);
    }
  };

  const openArtifact = async (presentation: ArtifactPresentation) => {
    if (!presentation.source_device) return;
    setBusy(true);
    setError('');
    setOpenNotice('');
    try {
      const result = await post<ArtifactOpenResult>(`/api/bff/artifacts/${presentation.artifact.id}/local-open`, { device_id: presentation.source_device.id });
      setOpenNotice(result.request ? `已发送到 ${presentation.source_device.display_name}，请求将在 60 秒内完成` : '本机打开请求已校验');
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '本机打开请求失败');
    } finally {
      setBusy(false);
    }
  };

  return <ProjectPage
    project={project}
    kicker="AI 视频剧本"
    title="结构化剧本与镜头审核"
    actions={<div className="heading-actions">
      <Button variant="secondary" title="刷新" aria-label="刷新" onClick={load}><RefreshCw size={16} /></Button>
      <Button disabled={busy || !approvedBrief || Boolean(pending)} onClick={generate}>
        <Sparkles size={16} />{pending ? '等待客户端' : busy ? '创建中...' : '生成剧本'}
      </Button>
    </div>}
  >
    {error && <Banner kind="error" onClose={() => setError('')}>{error}</Banner>}
    {openNotice && <Banner kind="success" onClose={() => setOpenNotice('')}>{openNotice}</Banner>}
    {pending && <Banner kind="info"><Clock3 size={16} />任务 {pending.state === 'queued' ? '正在等待已连接客户端领取' : '正在客户电脑执行'} <Status value={pending.state} /></Banner>}
    {!approvedBrief && <Banner kind="warning"><ShieldAlert size={16} />当前项目没有已批准 Brief</Banner>}
    {scripts.length === 0
      ? <section className="section"><Empty title="暂无剧本版本" detail="批准 Brief 后创建本地生成任务" action={<Button disabled={!approvedBrief || Boolean(pending)} onClick={generate}><Play size={16} />生成剧本</Button>} /></section>
      : <div className="script-workspace">
        <aside className="version-list">
          <header><span>版本</span><strong>{scripts.length}</strong></header>
          {scripts.map(script => <button key={script.id} className={script.id === active?.id ? 'active' : ''} onClick={() => setSelected(script.id)}>
            <div><strong>V{script.version}</strong><span>{changeTypeLabel(script.change_type)} · {formatDate(script.created_at)}</span></div>
            <Status value={script.status} /><ChevronRight size={16} />
          </button>)}
        </aside>
        {active && <ScriptDetail
          script={active}
          baselineVersion={active.baseline_script_version_id ? versionByID.get(active.baseline_script_version_id) : undefined}
          comments={comments}
          cycles={cycles}
          grants={grants}
          artifacts={artifacts}
          pending={Boolean(pending)}
          busy={busy}
          onSubmit={() => review('submit')}
          onReview={setReviewing}
          onComment={(shotID, shotIndex) => setCommentTarget({ shotID, shotIndex })}
          onResolveComment={resolveComment}
          onChange={setChanging}
          onCustomerReview={() => setShowGrants(true)}
          onExport={exportScript}
          onOpenArtifact={openArtifact}
        />}
      </div>}
    {active && commentTarget && <CommentModal
      script={active}
      target={commentTarget}
      onClose={() => setCommentTarget(undefined)}
      onCreated={item => { setComments(current => [...current, item]); setCommentTarget(undefined); }}
    />}
    {active && reviewing && <InternalReviewModal
      script={active}
      decision={reviewing}
      members={members}
      busy={busy}
      onClose={() => setReviewing(undefined)}
      onSubmit={(conclusion, assignee) => review(reviewing, conclusion, assignee)}
    />}
    {active && changing && <ScriptChangeModal
      script={active}
      changeType={changing}
      approvedBriefs={approvedBriefs}
      onClose={() => setChanging(undefined)}
      onCreated={async () => { setChanging(undefined); await load(); }}
    />}
    {active && showGrants && <ReviewGrantModal
      script={active}
      grants={grants}
      onClose={() => setShowGrants(false)}
      onChanged={() => loadDetail(active.id)}
    />}
  </ProjectPage>;
}

function ScriptDetail({
  script,
  baselineVersion,
  comments,
  cycles,
  grants,
  artifacts,
  pending,
  busy,
  onSubmit,
  onReview,
  onComment,
  onResolveComment,
  onChange,
  onCustomerReview,
  onExport,
  onOpenArtifact,
}: {
  script: Script;
  baselineVersion?: number;
  comments: ReviewComment[];
  cycles: ReviewCycle[];
  grants: ReviewGrant[];
  artifacts: ArtifactPresentation[];
  pending: boolean;
  busy: boolean;
  onSubmit: () => void;
  onReview: (decision: ReviewDecision) => void;
  onComment: (shotID: string, shotIndex: number) => void;
  onResolveComment: (commentID: string) => void;
  onChange: (changeType: ChangeType) => void;
  onCustomerReview: () => void;
  onExport: (format: string) => void;
  onOpenArtifact: (artifact: ArtifactPresentation) => void;
}) {
  const strategy = script.package.creative_strategy;
  const unresolved = comments.filter(item => !item.resolved_at);
  const latestCycle = cycles[0];
  const activeGrants = grants.filter(item => !item.revoked_at && !item.decision_at && new Date(item.expires_at) > new Date());
  return <section className="script-detail">
    <header className="script-title">
      <div><span>Script Package 1.1</span><h2>{script.package.title}</h2><p>{strategy.objective}</p></div>
      <div><Status value={script.status} /><span>{script.package.target_duration_seconds}s · 9:16</span></div>
    </header>
    <div className="revision-meta script-lineage">
      <span><GitBranch size={14} />{changeTypeLabel(script.change_type)}</span>
      {baselineVersion && <span>基于 V{baselineVersion}</span>}
      {script.revision_reason && <span>{script.revision_reason}</span>}
      {script.hypothesis && <span><FlaskConical size={13} />{script.hypothesis}</span>}
      {script.changed_fields.length > 0 && <code>{script.changed_fields.map(changeFieldLabel).join('、')}</code>}
    </div>
    {!script.validation.valid && <Banner kind="error">{script.validation.errors.map(item => item.message).join('；')}</Banner>}
    <div className="strategy-strip">
      <div><span>人群</span><strong>{strategy.audience}</strong></div>
      <div><span>需求时刻</span><strong>{strategy.demand_moment}</strong></div>
      <div><span>主卖点</span><strong>{strategy.primary_selling_point}</strong></div>
      <div><span>唯一变量</span><strong>{strategy.primary_test_variable}</strong></div>
    </div>
    <div className="bible-band">
      <div><span>场景锁</span><p>{script.package.production_bible.scene_lock}</p></div>
      <div><span>视觉锁</span><p>{script.package.production_bible.visual_style_lock}</p></div>
      <div><span>主体</span><p>{script.package.production_bible.subjects.map(subject => subject.name).join('、')}</p></div>
    </div>
    <div className="review-summary-band">
      <div><History size={16} /><span>审核周期</span><strong>{latestCycle ? `第 ${latestCycle.cycle_number} 轮 · ${cycleStatusLabel(latestCycle.status)}` : '尚未开始'}</strong></div>
      <div><MessageSquare size={16} /><span>未解决批注</span><strong className={unresolved.length ? 'danger-text' : ''}>{unresolved.length}</strong></div>
      <div><Link2 size={16} /><span>有效客户授权</span><strong>{activeGrants.length}</strong></div>
    </div>
    {latestCycle?.conclusion && <div className="review-conclusion"><UserRoundCheck size={16} /><div><span>整版结论</span><p>{latestCycle.conclusion}</p>{latestCycle.assignee_user_id && <small>修改责任人：{latestCycle.assignee_user_id.slice(0, 8)}</small>}</div></div>}
    {script.package.deliverability === 'blocked'
      ? <div className="blocked-list">{script.package.blocked_reasons.map(item => <div key={item.code}><ShieldAlert size={18} /><div><strong>{item.message}</strong><span>{item.next_action}</span></div></div>)}</div>
      : <div className="shot-list">{script.package.shots.map((shot, index) => <ShotRow
        key={shot.shot_id}
        shot={shot}
        index={index}
        comments={comments.filter(comment => comment.shot_id === shot.shot_id)}
        onComment={() => onComment(shot.shot_id, index)}
        onResolveComment={onResolveComment}
      />)}</div>}
    {cycles.length > 0 && <div className="review-history">
      <header><History size={15} /><strong>审核记录</strong></header>
      {cycles.map(cycle => <div key={cycle.id}><span>第 {cycle.cycle_number} 轮</span><Status value={cycle.status} /><p>{cycle.conclusion || '进行中'}</p><small>{formatDate(cycle.decided_at || cycle.opened_at)}</small></div>)}
    </div>}
    <footer className="review-bar">
      <div><CheckCircle2 size={17} /><span>内容哈希</span><code>{script.content_hash.slice(0, 16)}</code></div>
      <div>
        {script.status === 'review_ready' && <Button disabled={busy} onClick={onSubmit}><Send size={15} />提交内审</Button>}
        {script.status === 'internal_review' && <><Button variant="ghost" onClick={() => onReview('return')}><Undo2 size={15} />退回</Button><Button disabled={unresolved.length > 0} onClick={() => onReview('approve_internal')}><CheckCircle2 size={15} />内审通过</Button></>}
        {script.status === 'revision_requested' && <Button disabled={pending} onClick={() => onChange('revision')}><RotateCcw size={15} />创建修订</Button>}
        {(script.status === 'internally_approved' || script.status === 'client_review') && <Button onClick={onCustomerReview}><Link2 size={15} />客户审批链接</Button>}
        {script.status === 'approved' && <><Button variant="secondary" disabled={pending} onClick={() => onChange('variant')}><FlaskConical size={15} />创建变体</Button><div className="export-actions"><Button variant="secondary" onClick={() => onExport('markdown')}><Download size={15} />Markdown</Button><Button variant="secondary" onClick={() => onExport('xlsx')}>XLSX</Button><Button onClick={() => onExport('json')}>JSON</Button></div></>}
      </div>
    </footer>
    {artifacts.length > 0 && <div className="artifact-strip">{artifacts.map(presentation => {
      const artifact = presentation.artifact;
      return <div className="artifact-item" key={artifact.id}>
        <FileCode2 size={14} />
        <div><strong>{artifact.file_name}</strong><span>{artifactTierLabel(presentation.tier)} · {Math.ceil(artifact.byte_size / 1024)} KB</span></div>
        {presentation.actions.includes('local_open') && <button className="icon-button" title={`在 ${presentation.source_device?.display_name || '来源电脑'} 打开`} disabled={busy} onClick={() => onOpenArtifact(presentation)}><ExternalLink size={15} /></button>}
        {presentation.actions.includes('download') && <a className="icon-button" title="下载" href={`/api/bff/artifacts/${artifact.id}/download`}><Download size={15} /></a>}
      </div>;
    })}</div>}
  </section>;
}

function ShotRow({ shot, index, comments, onComment, onResolveComment }: { shot: Shot; index: number; comments: ReviewComment[]; onComment: () => void; onResolveComment: (commentID: string) => void }) {
  return <article className="shot-row">
    <div className="shot-number"><span>{String(index + 1).padStart(2, '0')}</span><small>{formatTC(shot.start_ms)}-{formatTC(shot.end_ms)}</small></div>
    <div className="shot-body">
      <header><Status value={shot.role} /><strong>{shot.narrative_purpose}</strong><button className="shot-comment-button" title="添加镜头批注" onClick={onComment}><MessageSquare size={15} />{comments.filter(item => !item.resolved_at).length || ''}</button></header>
      <div className="shot-main"><div><span>画面</span><p>{shot.visual_intent}</p></div><div><span>动作与运镜</span><p>{shot.subject_action} · {shot.camera_motion}</p></div><div><span>口播</span><p>{shot.voiceover || '无口播'}</p></div></div>
      {comments.map(comment => <blockquote className={`shot-comment ${comment.resolved_at ? 'resolved' : ''}`} key={comment.id}>
        <div><span>{comment.visibility === 'client' ? '客户可见' : '仅内部'}{comment.carried_from_comment_id ? ' · 跨版本携带' : ''}</span>{!comment.resolved_at && <button onClick={() => onResolveComment(comment.id)}><CheckCircle2 size={13} />解决</button>}</div>
        {comment.body}
      </blockquote>)}
      <details><summary><Film size={15} />生成与连续性</summary><div className="frame-grid"><div><span>首帧</span><p>{shot.first_frame.prompt_zh}</p></div><div><span>动态</span><p>{shot.motion_spec}</p></div><div><span>尾帧</span><p>{shot.end_frame.prompt_zh}</p></div></div></details>
    </div>
  </article>;
}

function CommentModal({ script, target, onClose, onCreated }: { script: Script; target: CommentTarget; onClose: () => void; onCreated: (value: ReviewComment) => void }) {
  const [body, setBody] = useState('');
  const [visibility, setVisibility] = useState('internal');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async () => {
    setBusy(true);
    setError('');
    try {
      onCreated(await post<ReviewComment>(`/api/bff/scripts/${script.id}/comments`, {
        shot_id: target.shotID,
        body,
        visibility,
        json_pointer: `/shots/${target.shotIndex}`,
      }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '批注失败');
    } finally {
      setBusy(false);
    }
  };
  return <Modal title={`镜头 ${target.shotIndex + 1} 批注`} onClose={onClose}>
    <Field label="可见范围"><select value={visibility} onChange={event => setVisibility(event.target.value)}><option value="internal">仅内部</option><option value="client">客户可见</option></select></Field>
    <Field label="批注"><textarea rows={4} value={body} onChange={event => setBody(event.target.value)} /></Field>
    {error && <p className="form-error">{error}</p>}
    <footer className="modal-actions"><Button variant="secondary" onClick={onClose}>取消</Button><Button disabled={busy || !body.trim()} onClick={submit}>{busy ? '保存中...' : '保存批注'}</Button></footer>
  </Modal>;
}

function InternalReviewModal({ script, decision, members, busy, onClose, onSubmit }: { script: Script; decision: ReviewDecision; members: Member[]; busy: boolean; onClose: () => void; onSubmit: (conclusion: string, assigneeUserID: string) => void }) {
  const isReturn = decision === 'return';
  const activeMembers = members.filter(item => item.membership.status === 'active');
  const [conclusion, setConclusion] = useState('');
  const [assignee, setAssignee] = useState(activeMembers.find(item => item.membership.role === 'editor')?.membership.user_id || activeMembers[0]?.membership.user_id || '');
  return <Modal title={`${isReturn ? '退回' : '批准'}剧本 V${script.version}`} onClose={onClose}>
    <p className="modal-copy">整版结论将写入当前审核周期并保留在版本历史中。</p>
    <Field label="整版结论"><textarea rows={4} value={conclusion} onChange={event => setConclusion(event.target.value)} placeholder={isReturn ? '说明需要修改的问题和验收标准' : '说明内容、事实与生成可行性审核结论'} /></Field>
    {isReturn && <Field label="修改责任人" hint={activeMembers.length ? undefined : '当前没有可分配的有效团队成员'}><select value={assignee} onChange={event => setAssignee(event.target.value)} disabled={!activeMembers.length}><option value="">请选择</option>{activeMembers.map(member => <option key={member.membership.user_id} value={member.membership.user_id}>{member.display_name || member.email} · {roleLabel(member.membership.role)}</option>)}</select></Field>}
    <footer className="modal-actions"><Button variant="secondary" onClick={onClose}>取消</Button><Button variant={isReturn ? 'danger' : 'primary'} disabled={busy || !conclusion.trim() || (isReturn && !assignee)} onClick={() => onSubmit(conclusion.trim(), assignee)}>{busy ? '提交中...' : isReturn ? '确认退回' : '确认通过'}</Button></footer>
  </Modal>;
}

function ScriptChangeModal({ script, changeType, approvedBriefs, onClose, onCreated }: { script: Script; changeType: ChangeType; approvedBriefs: Brief[]; onClose: () => void; onCreated: () => void }) {
  const variant = changeType === 'variant';
  const [briefID, setBriefID] = useState(approvedBriefs[0]?.id || '');
  const [selectedFields, setSelectedFields] = useState<string[]>([variant ? '/title' : '/shots/0']);
  const [hypothesis, setHypothesis] = useState('');
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const toggleField = (field: string) => setSelectedFields(current => current.includes(field) ? current.filter(item => item !== field) : [...current, field]);
  const submit = async () => {
    setBusy(true);
    setError('');
    try {
      await post(`/api/bff/scripts/${script.id}/runs`, {
        brief_version_id: briefID,
        change_type: changeType,
        invariant_fields: defaultInvariants.filter(field => !selectedFields.some(changed => changed === field || changed.startsWith(`${field}/`) || field.startsWith(`${changed}/`))),
        changed_fields: selectedFields,
        hypothesis: variant ? hypothesis.trim() : '',
        revision_reason: reason.trim(),
        idempotency_key: crypto.randomUUID(),
      });
      onCreated();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '创建变更任务失败');
    } finally {
      setBusy(false);
    }
  };
  return <Modal title={`${variant ? '创建单变量变体' : '创建修订'} · 基于 V${script.version}`} onClose={onClose}>
    <p className="modal-copy">服务端只冻结基线、变化声明和业务输入；实际 Agent 与模型仍只在已授权客户电脑执行。</p>
    <Field label="使用 Brief"><select value={briefID} onChange={event => setBriefID(event.target.value)}>{approvedBriefs.map(brief => <option key={brief.id} value={brief.id}>Brief V{brief.version} · {brief.objective}</option>)}</select></Field>
    <div className="change-field-picker"><span>{variant ? '唯一变化字段' : '允许变化字段'}</span>{changeFields.map(field => <label key={field.value}><input type={variant ? 'radio' : 'checkbox'} name="change-field" checked={selectedFields.includes(field.value)} onChange={() => variant ? setSelectedFields([field.value]) : toggleField(field.value)} /><div><strong>{field.label}</strong><code>{field.value}</code></div></label>)}</div>
    {variant && <Field label="实验假设"><textarea rows={3} value={hypothesis} onChange={event => setHypothesis(event.target.value)} placeholder="例如：更具体的标题能提高前三秒停留" /></Field>}
    <Field label={variant ? '变体原因' : '修订原因'}><textarea rows={3} value={reason} onChange={event => setReason(event.target.value)} placeholder="说明本次变化来自哪条批注或业务决策" /></Field>
    {error && <p className="form-error">{error}</p>}
    <footer className="modal-actions"><Button variant="secondary" onClick={onClose}>取消</Button><Button disabled={busy || !briefID || selectedFields.length === 0 || !reason.trim() || (variant && !hypothesis.trim())} onClick={submit}>{busy ? '创建中...' : '创建本地执行任务'}</Button></footer>
  </Modal>;
}

function ReviewGrantModal({ script, grants, onClose, onChanged }: { script: Script; grants: ReviewGrant[]; onClose: () => void; onChanged: () => void }) {
  const [email, setEmail] = useState('');
  const [created, setCreated] = useState<ReviewGrant>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const create = async () => {
    setBusy(true);
    setError('');
    try {
      const value = await post<ReviewGrant>(`/api/bff/scripts/${script.id}/review-grants`, { reviewer_email: email });
      setCreated(value);
      onChanged();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '创建失败');
    } finally {
      setBusy(false);
    }
  };
  const revoke = async (grant: ReviewGrant) => {
    if (!window.confirm(`确认撤销发给 ${grant.reviewer_email} 的审批链接？撤销后会立即失效。`)) return;
    setBusy(true);
    setError('');
    try {
      await post(`/api/bff/review-grants/${grant.id}/revoke`);
      onChanged();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : '撤销失败');
    } finally {
      setBusy(false);
    }
  };
  const url = created?.review_token ? `${window.location.origin}/review/${created.review_token}` : '';
  return <Modal title={`客户审批 · 剧本 V${script.version}`} onClose={onClose}>
    {created && url && <div className="grant-result">
      <Banner kind="success">链接与验证码仅显示本次，请通过不同渠道分别发送给客户。</Banner>
      <Field label="审批链接"><input readOnly value={url} /></Field>
      <Field label="六位验证码"><input readOnly value={created.dev_otp || ''} /></Field>
      <div className="row-actions"><Button onClick={() => navigator.clipboard.writeText(url)}>复制审批链接</Button><Button variant="secondary" onClick={() => setCreated(undefined)}>继续管理</Button></div>
    </div>}
    {!created && <>
      <p className="modal-copy">新链接固定绑定当前版本和内容哈希，不会自动切换到后续版本。</p>
      <div className="grant-create-row"><Field label="客户审批邮箱"><input type="email" value={email} onChange={event => setEmail(event.target.value)} placeholder="reviewer@example.com" /></Field><Button disabled={busy || !email.includes('@')} onClick={create}><Link2 size={15} />生成安全链接</Button></div>
      <div className="grant-history"><header><History size={15} /><strong>授权历史</strong><span>{grants.length}</span></header>{grants.length === 0 ? <p>尚未创建审批授权</p> : grants.map(grant => <div key={grant.id}><div><strong>{grant.reviewer_email}</strong><span>{formatDate(grant.created_at)} · {grantStatusLabel(grant)}</span></div>{!grant.revoked_at && !grant.decision_at && new Date(grant.expires_at) > new Date() && <Button variant="danger" disabled={busy} onClick={() => revoke(grant)}><XCircle size={14} />撤销</Button>}</div>)}</div>
    </>}
    {error && <p className="form-error">{error}</p>}
    <footer className="modal-actions"><Button variant="secondary" onClick={onClose}>关闭</Button></footer>
  </Modal>;
}

const formatTC = (ms: number) => `${Math.floor(ms / 1000)}s`;
const formatDate = (value: string) => new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value));
const changeTypeLabel = (value: string) => ({ initial: '初版', revision: '修订', variant: '单变量变体' }[value] || value);
const changeFieldLabel = (value: string) => changeFields.find(field => field.value === value)?.label || value;
const cycleStatusLabel = (value: string) => ({ open: '进行中', approved: '已通过', changes_requested: '已退回' }[value] || value);
const roleLabel = (value: string) => ({ tenant_admin: '管理员', project_manager: '项目负责人', strategist: '策略', editor: '编导', reviewer: '审核', viewer: '查看' }[value] || value);
const grantStatusLabel = (grant: ReviewGrant) => grant.revoked_at ? '已撤销' : grant.decision_at ? '已决定' : new Date(grant.expires_at) <= new Date() ? '已过期' : grant.verified_at ? '已验证' : '待验证';
const artifactTierLabel = (value: ArtifactPresentation['tier']) => ({ cloud_native: '原生剧本', hosted_preview: '托管演示', safe_rendition: '安全预览件', local_open: '来源电脑', metadata_only: '仅元数据' }[value]);
