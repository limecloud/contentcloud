import { useEffect, useMemo, useState } from 'react';
import { Activity, AlertTriangle, Ban, Bot, CheckCircle2, ChevronRight, CircleDollarSign, Clock3, GitFork, History, Pause, Play, RefreshCw, ScanSearch, ServerCog, ShieldAlert, ShieldCheck, Workflow } from 'lucide-react';
import { useNavigate, useParams } from 'react-router-dom';
import { api, post } from '../../api';
import type { RuntimeAttemptView, RuntimeEffectView, RuntimeGateView, RuntimeJobDetail, RuntimeJobList, RuntimeJobSummary, RuntimeReplayResult, RuntimeStateCollectionView } from '../../types';
import { adminJobPath } from '../routes';

const stateLabel: Record<string, string> = { created: '已创建', admitted: '已接纳', running: '进行中', waiting_human: '等待确认', paused: '已暂停', completed: '已完成', failed: '失败', cancelled: '已取消', rejected: '已拒绝' };
const taskStatusLabel: Record<string, string> = { needs_input: '等待补充资料', ready: '准备开始', running: '处理中', waiting_gate: '等待确认', paused: '已暂停', blocked: '暂时无法继续', accepted: '待交付', delivered: '已交付', cancelled: '已取消' };
const stateClass = (state: string) => state === 'completed' ? 'is-success' : state === 'failed' || state === 'cancelled' || state === 'rejected' ? 'is-danger' : state === 'waiting_human' || state === 'paused' ? 'is-warning' : state === 'running' || state === 'admitted' ? 'is-working' : 'is-neutral';
const nodeStateLabel: Record<string, string> = { pending: '未开始', ready: '待执行', leased: '已分配', running: '处理中', waiting_resource: '等待资源', waiting_external: '等待外部结果', waiting_human: '等待确认', succeeded: '已完成', retryable_failed: '失败，可再试', failed: '失败', blocked: '暂时无法继续', skipped: '已跳过', cancelled: '已取消', lease_expired: '处理已超时' };
const agentStateLabel: Record<string, string> = { created: '已创建', runnable: '可运行', active: '处理中', waiting_children: '等待后续步骤', waiting_gate: '等待确认', waiting_effect: '等待外部操作', completed: '已完成', failed: '失败', canceling: '取消中', cancelled: '已取消' };
const attemptStateLabel: Record<string, string> = { prepared: '已准备', running: '处理中', succeeded: '已完成', retryable_failed: '失败，可再试', failed: '失败', cancelled: '已取消', expired: '已过期' };
const gateStateLabel: Record<string, string> = { pending: '待确认', approved: '已通过', rejected: '已拒绝', changes_requested: '要求修改', expired: '已过期' };
const effectStateLabel: Record<string, string> = { registered: '已登记', submitted: '已提交', acknowledged: '已确认', succeeded: '已成功', failed: '失败', unknown: '结果待核对', reconciling: '正在核对', manual_action: '等待人工处理' };
const stateTone = (state: string) => state === 'succeeded' || state === 'completed' || state === 'approved' ? 'is-success' : state === 'failed' || state === 'cancelled' || state === 'rejected' ? 'is-danger' : state.startsWith('waiting') || state === 'paused' || state === 'unknown' || state === 'reconciling' || state === 'pending' || state === 'changes_requested' ? 'is-warning' : state === 'running' || state === 'active' || state === 'admitted' ? 'is-working' : 'is-neutral';
const dateTime = (value: string) => new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value));
const shortDigest = (value: string) => value.length > 24 ? `${value.slice(0, 21)}…` : value;
const durationSince = (value: string) => {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 60) return `${seconds} 秒`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} 分钟`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时`;
  return `${Math.floor(hours / 24)} 天`;
};
const gateModeLabel: Record<string, string> = { required_check: '必需检查', internal_review: '内部审核', client_decision: '客户确认' };
const consistencyLabel: Record<string, string> = { single_writer: '单人维护', append_only: '只保留新增记录', cas_map: '按版本保存', reducer_owned: '由系统统一整理' };
const eventLabel = (value: string) => ({ 'job.created': '创建任务', 'job.admitted': '任务开始排队', 'job.running': '开始处理', 'job.waiting_human': '等待人工确认', 'job.paused': '暂停后续处理', 'job.resumed': '恢复处理', 'job.cancelled': '取消任务', 'job.completed': '任务完成', 'job.failed': '任务失败', 'job.forked': '从已有进度继续' } as Record<string, string>)[value] || value;
const processingTypeLabel = (value: string) => ({ codex: '工作电脑处理', fake: '演示处理', worker: '后台处理', agent: '自动处理' } as Record<string, string>)[value] || '自动处理';
const actorLabel = (value: string) => ({ user: '用户', worker: '后台服务', harness: '处理服务', system: '系统' } as Record<string, string>)[value] || '系统';
  const effectLabel = (value: string) => ({ 'media.generate': '生成视频', 'media.render': '生成成片', 'knowledge.query': '查询知识', 'content.generate': '生成内容' } as Record<string, string>)[value] || '外部服务处理';

type RuntimeMutation = 'pause' | 'resume' | 'cancel' | 'refresh';
type RuntimeTab = 'overview' | 'steps' | 'requests' | 'gates' | 'cost' | 'events' | 'technical';

export function AdminRuntimePage() {
  const { jobID = '' } = useParams();
  const navigate = useNavigate();
  const [list, setList] = useState<RuntimeJobList>();
  const [selectedID, setSelectedID] = useState(jobID);
  const [detail, setDetail] = useState<RuntimeJobDetail>();
  const [state, setState] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [replay, setReplay] = useState<RuntimeReplayResult>();

  const loadDetail = async (id: string) => {
    try {
      setDetail(await api<RuntimeJobDetail>(`/api/bff/runtime/jobs/${encodeURIComponent(id)}`));
    } catch (value) {
      setError(value instanceof Error ? value.message : '任务详情读取失败');
    }
  };
  const loadList = async (preferredID = jobID || selectedID) => {
    setLoading(true);
    setError('');
    try {
      const query = state ? `?state=${encodeURIComponent(state)}` : '';
      const next = await api<RuntimeJobList>(`/api/bff/runtime/jobs${query}`);
      setList(next);
      const nextID = preferredID && next.items.some(item => item.id === preferredID) ? preferredID : next.items[0]?.id || '';
      setSelectedID(nextID);
      if (nextID) {
        if (jobID !== nextID) navigate(adminJobPath(nextID), { replace: true });
        await loadDetail(nextID);
      } else {
        setDetail(undefined);
      }
    } catch (value) {
      setError(value instanceof Error ? value.message : '任务运行数据读取失败');
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => { void loadList(jobID); }, [state]);
  useEffect(() => {
    if (jobID && jobID !== selectedID) {
      setSelectedID(jobID);
      setReplay(undefined);
      void loadDetail(jobID);
    }
  }, [jobID]);
  const selected = useMemo(() => list?.items.find(item => item.id === selectedID), [list, selectedID]);

  const action = async (kind: RuntimeMutation) => {
    if (!selectedID) return;
    if (kind === 'cancel' && !window.confirm('确认取消这个任务运行？已经完成的结果会保留，未开始的步骤不会继续执行。')) return;
    setBusy(kind);
    setError('');
    setNotice('');
    try {
      const next = await post<RuntimeJobDetail>(`/api/bff/runtime/jobs/${encodeURIComponent(selectedID)}/${kind}`);
      setDetail(next);
      await loadList(selectedID);
      setNotice(kind === 'pause' ? '已暂停后续处理，当前已完成结果保持不变。' : kind === 'resume' ? '已恢复任务处理。' : kind === 'cancel' ? '已取消未开始步骤，历史结果保持不变。' : '任务记录已刷新。');
    } catch (value) {
      setError(value instanceof Error ? value.message : '任务操作失败');
    } finally {
      setBusy('');
    }
  };

  const replayJob = async () => {
    if (!selectedID) return;
    setBusy('replay');
    setError('');
    try {
      setReplay(await post<RuntimeReplayResult>(`/api/bff/runtime/jobs/${encodeURIComponent(selectedID)}/replay`));
    } catch (value) {
      setError(value instanceof Error ? value.message : '任务状态整理失败');
    } finally {
      setBusy('');
    }
  };

  const forkCheckpoint = async (checkpointID: string) => {
    if (!selectedID || !window.confirm('确认从这个安全检查点创建新的执行分支？源任务和已完成结果不会被改写。')) return;
    setBusy(`fork:${checkpointID}`);
    setError('');
    try {
      const next = await post<RuntimeJobDetail>(`/api/bff/runtime/checkpoints/${encodeURIComponent(checkpointID)}/fork`, { idempotency_key: `runtime-fork:${checkpointID}:${Date.now()}` });
      setReplay(undefined);
      setNotice(`已创建新的执行分支 ${next.summary.id.slice(0, 8)}。`);
      setDetail(next);
      navigate(adminJobPath(next.summary.id));
      await loadList(next.summary.id);
    } catch (value) {
      setError(value instanceof Error ? value.message : '创建执行分支失败');
    } finally {
      setBusy('');
    }
  };

  const reconcileEffect = async (effect: RuntimeEffectView) => {
    setBusy(`reconcile:${effect.id}`);
    setError('');
    try {
      const next = await post<RuntimeJobDetail>(`/api/bff/runtime/effects/${encodeURIComponent(effect.id)}/reconcile`, { expected_version: effect.version });
      setDetail(next);
      setNotice('结果待核对的外部请求已进入核对状态，没有重新提交相同请求。');
      await loadList(selectedID);
    } catch (value) {
      setError(value instanceof Error ? value.message : '外部结果核对失败');
    } finally {
      setBusy('');
    }
  };

  return <div className="admin-runtime-page">
    <div className="admin-heading"><div><span className="eyebrow">任务跟进 / 任务进度</span><h1>任务进度</h1><p>查看客户任务停在哪一步，并在需要时暂停、恢复或核对结果。</p></div><div className="admin-runtime-actions"><label className="admin-runtime-filter"><span>筛选</span><select value={state} onChange={event => setState(event.target.value)}><option value="">全部任务</option><option value="running">进行中</option><option value="waiting_human">等待确认</option><option value="failed">需要处理</option><option value="completed">已完成</option><option value="cancelled">已取消</option></select></label><button className="icon-button" aria-label="刷新任务进度" title="刷新任务进度" disabled={loading} onClick={() => void loadList()}><RefreshCw size={16} className={loading ? 'is-spinning' : ''} /></button></div></div>
    {error && <div className="admin-runtime-error" role="alert"><AlertTriangle size={16} /><span>{error}</span></div>}
    {notice && <div className="admin-runtime-notice" role="status"><CheckCircle2 size={16} /><span>{notice}</span></div>}
    <div className="admin-runtime-layout">
      <section className="section admin-runtime-list"><header className="section-header"><div><span className="section-kicker">任务进展</span><h2>需要留意的任务</h2></div><span className="admin-muted">{list?.items.length || 0} 条</span></header>{loading ? <div className="admin-runtime-empty">正在读取任务进度…</div> : !list?.items.length ? <div className="admin-runtime-empty"><ServerCog size={22} /><span>当前没有匹配的任务</span></div> : <div className="admin-runtime-job-list">{list.items.map(item => <RuntimeJobRow key={item.id} item={item} selected={item.id === selectedID} onClick={() => { setSelectedID(item.id); setReplay(undefined); setNotice(''); navigate(adminJobPath(item.id)); void loadDetail(item.id); }} />)}</div>}</section>
      <section className="section admin-runtime-detail">{!detail ? <div className="admin-runtime-empty"><Workflow size={24} /><span>选择一个任务查看详情</span></div> : <RuntimeDetail detail={detail} busy={busy} replay={replay} onAction={action} onReplay={replayJob} onForkCheckpoint={forkCheckpoint} onReconcileEffect={reconcileEffect} />}</section>
    </div>
  </div>;
}

function RuntimeJobRow({ item, selected, onClick }: { item: RuntimeJobSummary; selected: boolean; onClick: () => void }) {
  const step = item.total_steps > 0 ? `${item.completed_steps}/${item.total_steps} 步` : '步骤未登记';
  return <button className={`admin-runtime-job-row ${selected ? 'is-selected' : ''}`} onClick={onClick}><span className="admin-runtime-job-icon"><Activity size={16} /></span><span className="admin-runtime-job-copy"><strong>{item.task_title || '未命名任务'}</strong><small>{item.customer_name || item.project_name || '未命名客户'} · {item.project_name || '未命名项目'}</small><small>{item.product_name ? `${item.product_name} v${item.product_version} · ${step}` : step} · 停留 {durationSince(item.status_since)}</small></span><span className={`runtime-state ${stateClass(item.state)}`}>{stateLabel[item.state] || item.state}</span><ChevronRight size={15} /></button>;
}

interface RuntimeDetailProps {
  detail: RuntimeJobDetail;
  busy: string;
  replay?: RuntimeReplayResult;
  initialTab?: RuntimeTab;
  onAction: (kind: RuntimeMutation) => Promise<void>;
  onReplay: () => Promise<void>;
  onForkCheckpoint: (checkpointID: string) => Promise<void>;
  onReconcileEffect: (effect: RuntimeEffectView) => Promise<void>;
}

export function RuntimeDetail({ detail, busy, replay, initialTab = 'overview', onAction, onReplay, onForkCheckpoint, onReconcileEffect }: RuntimeDetailProps) {
  const { summary } = detail;
  const [tab, setTab] = useState<RuntimeTab>(initialTab);
  const can = (action: string) => summary.allowed_actions.includes(action);
  const tabs: Array<[RuntimeTab, string, number?]> = [['overview', '任务概况'], ['steps', '处理步骤', detail.nodes.length], ['requests', '外部服务', detail.effects.length], ['gates', '客户确认', detail.gates.length], ['cost', '费用', summary.cost.effect_count], ['events', '操作记录', detail.events.length], ['technical', '详细信息']];
  return <div className="admin-runtime-detail-inner"><header className="admin-runtime-detail-header"><div><span className="eyebrow">{summary.customer_name || summary.project_name}</span><h2>{summary.task_title || '未命名任务'}</h2><p>{summary.project_name || '未命名项目'}{summary.product_name ? ` · ${summary.product_name} v${summary.product_version}` : ''} · 更新于 {dateTime(summary.updated_at)}</p>{summary.source_job_run_id && <p className="admin-runtime-lineage"><GitFork size={13} />从安全检查点创建，源任务 {summary.source_job_run_id.slice(0, 8)}</p>}</div><div className="admin-runtime-detail-actions">{can('pause') && <button className="button button-secondary" disabled={Boolean(busy)} onClick={() => void onAction('pause')}><Pause size={14} />{busy === 'pause' ? '暂停中…' : '暂停后续处理'}</button>}{can('resume') && <button className="button button-primary" disabled={Boolean(busy)} onClick={() => void onAction('resume')}><Play size={14} />{busy === 'resume' ? '恢复处理' : '恢复处理'}</button>}{can('cancel') && <button className="button button-danger" disabled={Boolean(busy)} onClick={() => void onAction('cancel')}><Ban size={14} />{busy === 'cancel' ? '取消中…' : '取消未开始步骤'}</button>}{can('refresh') && <button className="icon-button" aria-label="刷新当前任务" title="刷新当前任务" disabled={Boolean(busy)} onClick={() => void onAction('refresh')}><RefreshCw size={15} /></button>}</div></header>
    {replay && <div className="admin-runtime-replay-result"><ScanSearch size={16} /><span>{replay.projection_rebuilt ? '任务状态已重新整理' : '任务状态未重新整理'}；完整性{replay.integrity_status === 'verified' ? '已验证' : '未验证'}，已核对 {replay.event_count} 条记录，外部服务调用 {replay.external_calls} 次，最新记录编号 {replay.last_sequence}。</span></div>}
    <nav className="admin-runtime-tabs" aria-label="任务详情分区">{tabs.map(([id, label, count]) => <button key={id} className={tab === id ? 'is-active' : ''} onClick={() => setTab(id)}>{label}{count !== undefined && <small>{count}</small>}</button>)}</nav>
    {tab === 'overview' && <RuntimeOverview detail={detail} busy={busy} onForkCheckpoint={onForkCheckpoint} />}
    {tab === 'steps' && <RuntimeSteps summary={summary} nodes={detail.nodes} />}
    {tab === 'requests' && <RuntimeEffects effects={detail.effects} busy={busy} onReconcileEffect={onReconcileEffect} />}
    {tab === 'gates' && <RuntimeGates gates={detail.gates} />}
    {tab === 'cost' && <RuntimeCost summary={summary} effects={detail.effects} />}
    {tab === 'events' && <RuntimeEvents events={detail.events} />}
    {tab === 'technical' && <RuntimeTechnical detail={detail} busy={busy} onReplay={onReplay} />}
  </div>;
}

function RuntimeOverview({ detail, busy, onForkCheckpoint }: { detail: RuntimeJobDetail; busy: string; onForkCheckpoint: (checkpointID: string) => Promise<void> }) {
  const { summary } = detail;
  const progress = summary.total_steps > 0 ? `${summary.completed_steps}/${summary.total_steps}` : '未登记';
  return <div className="admin-runtime-overview"><div className="admin-runtime-facts"><span><small>任务状态</small><strong className={`runtime-state ${stateClass(summary.state)}`}>{stateLabel[summary.state] || summary.state}</strong></span><span><small>当前步骤</small><strong>{summary.current_step_name || '尚未开始'}</strong></span><span><small>已完成步骤</small><strong>{progress} 步</strong></span><span><small>持续时间</small><strong>{durationSince(summary.status_since)}</strong></span></div><section className="admin-runtime-guidance"><div><span className="section-kicker">现在发生了什么</span><h3>{summary.blocking_reason || '任务状态已更新'}</h3><p>{summary.task_next_action || '任务详情会在这里汇总。'}</p></div><div className="admin-runtime-guidance-next"><span>建议下一步</span><strong>{summary.recommended_action || '继续观察任务'}</strong></div></section><section className="admin-runtime-business-meta"><div><span>客户</span><strong>{summary.customer_name || '未登记'}</strong></div><div><span>项目</span><strong>{summary.project_name || '未登记'}</strong></div><div><span>创作流程</span><strong>{summary.product_name ? `${summary.product_name} v${summary.product_version}` : '未登记'}</strong></div><div><span>任务编号</span><code>{summary.id}</code></div></section>{summary.checkpoint_count > 0 && <section className="admin-runtime-recovery"><header><div><span className="section-kicker">可以继续的地方</span><h3>已有可复用进度</h3></div><span>{summary.checkpoint_count} 个</span></header>{detail.checkpoints.map(item => <div className="admin-runtime-checkpoint" key={item.id}><CheckCircle2 size={15} /><div><strong>{item.node_key}</strong><small>{item.completed_nodes.length} 个步骤已完成{item.blocked_reason ? ` · ${item.blocked_reason}` : ''}</small></div><time>{dateTime(item.created_at)}</time>{item.allowed_actions.includes('fork') && <button className="button button-secondary" title="从这里继续，不影响原任务" disabled={Boolean(busy)} onClick={() => void onForkCheckpoint(item.id)}><GitFork size={14} />从这里继续</button>}</div>)}</section>}</div>;
}

function RuntimeSteps({ summary, nodes }: { summary: RuntimeJobSummary; nodes: RuntimeJobDetail['nodes'] }) { return <section className="admin-runtime-subsection"><header><div><span className="section-kicker">任务进展</span><strong>处理步骤</strong></div><span>{summary.completed_steps}/{summary.total_steps || nodes.length} 步已完成</span></header><div className="admin-runtime-node-table"><div className="admin-runtime-node-head"><span>步骤</span><span>状态</span><span>结果</span><span>更新时间</span></div>{nodes.length === 0 ? <div className="admin-runtime-empty compact">还没有开始处理步骤</div> : nodes.map(node => <div className="admin-runtime-node-row" key={node.id}><div><strong>{node.name || node.node_key}</strong><small>{node.customer_step_id || '平台步骤'}</small></div><span className={`runtime-state ${stateTone(node.state)}`}>{nodeStateLabel[node.state] || node.state}</span><span>{node.output_digest ? '已保存' : node.state === 'succeeded' || node.state === 'skipped' ? '已完成' : '还没有结果'}</span><time>{dateTime(node.updated_at)}</time></div>)}</div></section>; }

function RuntimeGates({ gates }: { gates: RuntimeGateView[] }) { return <section className="admin-runtime-subsection"><header><div><span className="section-kicker">需要人工确认</span><strong>客户确认</strong></div><span>{gates.length} 个确认点</span></header>{gates.length === 0 ? <div className="admin-runtime-empty compact"><ShieldCheck size={18} />当前任务没有人工确认点</div> : <div className="admin-runtime-event-list">{gates.map(gate => <article key={gate.id}><ShieldCheck size={14} /><div><strong>{gate.name}</strong><small>{gateModeLabel[gate.mode] || gate.mode} · <span className={`runtime-state ${stateTone(gate.state)}`}>{gateStateLabel[gate.state] || gate.state}</span>{gate.reason ? ` · ${gate.reason}` : ''}</small></div><time>{dateTime(gate.updated_at)}</time></article>)}</div>}</section>; }

function RuntimeEffects({ effects, busy, onReconcileEffect }: { effects: RuntimeJobDetail['effects']; busy: string; onReconcileEffect: (effect: RuntimeEffectView) => Promise<void> }) { return <section className="admin-runtime-subsection"><header><div><span className="section-kicker">外部服务</span><strong>外部服务结果</strong></div><span>{effects.length} 条记录</span></header>{effects.length === 0 ? <div className="admin-runtime-empty compact"><ShieldAlert size={18} />还没有外部服务记录</div> : <div className="admin-runtime-event-list">{effects.map(effect => <article className="admin-runtime-effect-row" key={effect.id}><ShieldAlert size={14} /><div><strong>{effectLabel(effect.kind)}</strong><small><span className={`runtime-state ${stateTone(effect.state)}`}>{effectStateLabel[effect.state] || effect.state}</span>{effect.error_code ? ' · 需要查看详细原因' : ' · 详细请求已隐藏'}</small></div><time>{effect.cost_minor ? `${effect.cost_minor} ${effect.currency}` : '费用未登记'}</time>{effect.allowed_actions.includes('reconcile') && <button className="button button-secondary" disabled={Boolean(busy)} onClick={() => void onReconcileEffect(effect)}><ShieldAlert size={13} />{busy === `reconcile:${effect.id}` ? '核对中…' : '核对结果'}</button>}</article>)}</div>}</section>; }

function RuntimeCost({ summary, effects }: { summary: RuntimeJobSummary; effects: RuntimeJobDetail['effects'] }) { const cost = summary.cost; const label = cost.status === 'recorded' ? `${cost.amount_minor} ${cost.currency}` : cost.status === 'mixed_currency' ? '多币种，需分别核对' : '暂时没有费用记录'; return <section className="admin-runtime-subsection"><header><div><span className="section-kicker">费用记录</span><strong>费用</strong></div><span>{cost.effect_count} 条外部处理有费用记录</span></header><div className="admin-runtime-cost-summary"><CircleDollarSign size={20} /><div><strong>{label}</strong><span>{cost.status === 'recorded' ? '这里只显示已经登记的金额，不代表最终结算。' : '当前没有可确认的费用，页面不会估算。'}</span></div></div><div className="admin-runtime-event-list">{effects.filter(effect => effect.cost_minor > 0).map(effect => <article key={effect.id}><CircleDollarSign size={14} /><div><strong>{effectLabel(effect.kind)}</strong><small>{effect.state === 'succeeded' ? '处理已完成' : '处理结果还未最终确认'}</small></div><time>{effect.cost_minor} {effect.currency}</time></article>)}</div></section>; }

function RuntimeEvents({ events }: { events: RuntimeJobDetail['events'] }) { return <section className="admin-runtime-subsection"><header><div><span className="section-kicker">操作记录</span><strong>发生了什么</strong></div><span>{events.length} 条</span></header><div className="admin-runtime-event-list">{events.length === 0 ? <div className="admin-runtime-empty compact"><Clock3 size={18} />还没有操作记录</div> : events.slice().reverse().map(event => <article key={event.id}><Clock3 size={14} /><div><strong>{eventLabel(event.type)}</strong><small>记录编号 {event.sequence} · {event.node_key || '任务'} · {actorLabel(event.actor_type)}</small></div><time>{dateTime(event.occurred_at)}</time></article>)}</div></section>; }

function RuntimeTechnical({ detail, busy, onReplay }: { detail: RuntimeJobDetail; busy: string; onReplay: () => Promise<void> }) { const { summary } = detail; return <div className="admin-runtime-technical"><section className="admin-runtime-subsection"><header><div><span className="section-kicker">详细信息</span><strong>任务处理信息</strong></div><span>版本 {summary.contract_major}.{summary.contract_minor}</span></header><dl className="admin-runtime-identity"><div><dt>任务编号</dt><dd title={summary.root_job_run_id}>{summary.root_job_run_id.slice(0, 8)}</dd></div><div><dt>处理规则</dt><dd>{summary.runtime_policy_id}</dd></div><div><dt>任务安排记录</dt><dd title={summary.plan_digest}>{shortDigest(summary.plan_digest)}</dd></div><div><dt>功能设置记录</dt><dd title={summary.binding_digest}>{shortDigest(summary.binding_digest)}</dd></div><div><dt>输入资料记录</dt><dd title={summary.input_digest}>{shortDigest(summary.input_digest)}</dd></div></dl><div className="admin-runtime-technical-actions"><button className="button button-secondary" disabled={Boolean(busy)} onClick={() => void onReplay()}><History size={14} />{busy === 'replay' ? '整理中…' : '重新整理任务状态'}</button></div></section><RuntimeAgents agents={detail.agents} /><RuntimeAttempts attempts={detail.attempts} /><RuntimeStateCollections collections={detail.state_collections} /></div>; }

function RuntimeAgents({ agents }: { agents: RuntimeJobDetail['agents'] }) { return <section className="admin-runtime-subsection"><header><div><span className="section-kicker">详细信息</span><strong><Bot size={14} />自动处理任务</strong></div><span>{agents.length} 个</span></header>{agents.length === 0 ? <div className="admin-runtime-empty compact"><Bot size={18} />当前任务没有自动处理记录</div> : <div className="admin-runtime-node-table"><div className="admin-runtime-node-head"><span>角色</span><span>状态</span><span>预算</span><span>可用工具</span></div>{agents.map(agent => <div className="admin-runtime-node-row" key={agent.id}><div style={{ paddingLeft: `${Math.min(agent.depth, 5) * 12}px` }}><strong>{agent.role || '未命名角色'}</strong><small>{processingTypeLabel(agent.harness_kind)} · 层级 {agent.depth}{agent.parent_agent_instance_id ? ' · 后续实例' : ''}</small></div><span className={`runtime-state ${stateTone(agent.state)}`}>{agentStateLabel[agent.state] || agent.state}</span><span>{agent.used_cost_minor} / {agent.budget_minor}</span><div><strong>{agent.context_view.allowed_tools.join(' · ') || '无工具'}</strong><small>{agent.context_view.input_ref_count} 个输入引用 · {agent.session_bound ? '已建立连接' : '未建立连接'}</small></div></div>)}</div>}</section>; }

function RuntimeAttempts({ attempts }: { attempts: RuntimeAttemptView[] }) { return <section className="admin-runtime-subsection"><header><div><span className="section-kicker">处理记录</span><strong>每次运行情况</strong></div><span>{attempts.length} 次</span></header>{attempts.length === 0 ? <div className="admin-runtime-empty compact">还没有处理记录</div> : <div className="admin-runtime-event-list">{attempts.map(attempt => <article key={attempt.id}><Activity size={14} /><div><strong>第 {attempt.attempt_no} 次运行</strong><small>{processingTypeLabel(attempt.harness_kind)} · <span className={`runtime-state ${stateTone(attempt.state)}`}>{attemptStateLabel[attempt.state] || attempt.state}</span>{attempt.executor_ref ? ` · 电脑 ${attempt.executor_ref}` : ''}</small></div><time>{attempt.lease_expires_at ? `预计完成 ${dateTime(attempt.lease_expires_at)}` : dateTime(attempt.updated_at)}</time></article>)}</div>}</section>; }

function RuntimeStateCollections({ collections }: { collections: RuntimeStateCollectionView[] }) { return <section className="admin-runtime-subsection"><header><div><span className="section-kicker">任务共用资料</span><strong>可以共用的任务资料</strong></div><span>{collections.length} 组</span></header>{collections.length === 0 ? <div className="admin-runtime-empty compact">当前任务没有共用资料</div> : <div className="admin-runtime-event-list">{collections.map(collection => <article key={collection.id}><Workflow size={14} /><div><strong>{collection.collection_key}</strong><small>{collection.scope} · {consistencyLabel[collection.consistency] || '由系统统一维护'} · {collection.record_count} 条记录 · 第 {collection.revision} 版</small></div><time>{dateTime(collection.updated_at)}</time></article>)}</div>}</section>; }
