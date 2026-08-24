import { AlertTriangle, CheckCircle2, Clock3, RefreshCw, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { api, post } from '../../api';
import type { RuntimeCleanupDiagnostic, RuntimeCleanupStatus } from '../../types';

const statusOptions: Array<{ value: '' | RuntimeCleanupStatus; label: string }> = [
  { value: '', label: '全部状态' },
  { value: 'pending', label: '待清理' },
  { value: 'retrying', label: '清理中' },
  { value: 'failed', label: '清理失败' },
  { value: 'cleaned', label: '已清理' },
  { value: 'not_found', label: '对象已不存在' },
];

export function cleanupStatusLabel(value: string): string {
  return statusOptions.find(item => item.value === value)?.label || value || '未知状态';
}

function statusTone(value: string): string {
  if (value === 'cleaned' || value === 'not_found') return 'success';
  if (value === 'failed') return 'danger';
  if (value === 'pending' || value === 'retrying') return 'warning';
  return 'neutral';
}

function dateTime(value?: string): string {
  if (!value) return '未安排';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '未安排';
  return new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date);
}

function short(value: string, length = 24): string {
  return value.length > length ? `${value.slice(0, length - 1)}…` : value;
}

export function AdminCleanupPage() {
  const [status, setStatus] = useState<'' | RuntimeCleanupStatus>('');
  const [items, setItems] = useState<RuntimeCleanupDiagnostic[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      const query = status ? `?status=${encodeURIComponent(status)}&limit=100` : '?limit=100';
      const next = await api<RuntimeCleanupDiagnostic[]>(`/api/bff/runtime/cleanup-diagnostics${query}`);
      setItems(next || []);
    } catch (value) {
      setError(value instanceof Error ? value.message : '清理诊断读取失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, [status]);

  const retry = async (item: RuntimeCleanupDiagnostic) => {
    if (!window.confirm(`确认重试清理临时对象“${item.object_key}”？这不会重新提交业务任务或生成新的产物。`)) return;
    setBusy(item.id);
    setError('');
    setNotice('');
    try {
      const result = await post<{ diagnostic: RuntimeCleanupDiagnostic; delete_attempted: boolean }>(`/api/bff/runtime/cleanup-diagnostics/${encodeURIComponent(item.id)}/retry`);
      setItems(current => current.map(value => value.id === item.id ? result.diagnostic : value));
      setNotice(result.diagnostic.status === 'cleaned' || result.diagnostic.status === 'not_found' ? '清理诊断已收敛，业务事实保持不变。' : '清理仍未完成，系统已记录下一次重试时间。');
    } catch (value) {
      setError(value instanceof Error ? value.message : '清理重试失败');
    } finally {
      setBusy('');
    }
  };

  return <div className="operations-page operations-cleanup-page">
    <header className="operations-page-intro"><div><span className="operations-eyebrow">运行治理 / 临时对象</span><h1>清理诊断</h1><p>处理数据库事实已经回滚、但临时 Blob 仍需清理的异常。这里不会创建任务、审批或产物事实。</p></div><div className="operations-page-actions"><button className="icon-button" aria-label="刷新清理诊断" title="刷新清理诊断" disabled={loading} onClick={() => void load()}><RefreshCw size={16} className={loading ? 'is-spinning' : ''} /></button></div></header>
    <section className="operations-brief"><div className="operations-brief-lead"><span className="operations-live-dot"/><div><strong>Runtime 清理事实</strong><span>状态、版本和重试次数由服务端持久化并通过 CAS 更新；清理成功不会改变 ApprovedSnapshot、Artifact 或 Delivery。</span></div></div><div className="operations-brief-facts"><span><small>待处理</small><b>{items.filter(item => item.status === 'pending' || item.status === 'failed').length}</b></span><span><small>清理中</small><b>{items.filter(item => item.status === 'retrying').length}</b></span><span><small>已收敛</small><b>{items.filter(item => item.status === 'cleaned' || item.status === 'not_found').length}</b></span></div></section>
    {error && <div className="workos-notice is-error" role="alert"><AlertTriangle size={16}/><span>{error}</span></div>}
    {notice && <div className="workos-notice is-info" role="status"><CheckCircle2 size={16}/><span>{notice}</span></div>}
    <section className="operations-section"><header className="operations-section-title"><div><span>持久化诊断</span><h2>{items.length} 条记录</h2></div><label className="operations-filter"><span>状态</span><select value={status} onChange={event => setStatus(event.target.value as '' | RuntimeCleanupStatus)}>{statusOptions.map(option => <option value={option.value} key={option.value}>{option.label}</option>)}</select></label></header>
      {loading ? <div className="operations-empty"><RefreshCw size={20} className="is-spinning"/><span>正在读取清理诊断…</span></div> : items.length === 0 ? <div className="operations-empty"><Trash2 size={22}/><strong>没有匹配的清理诊断</strong><span>数据库事实和临时对象保持一致。</span></div> : <div className="operations-table operations-cleanup-table"><header><span>临时对象</span><span>失败原因</span><span>状态</span><span>尝试</span><span>下次重试</span><span>更新时间</span><span>操作</span></header>{items.map(item => <div className="operations-table-row" key={item.id}><span className="operations-primary-cell"><span className={`operations-object-icon ${statusTone(item.status)}`}><Trash2 size={16}/></span><span><strong title={item.object_key}>{short(item.object_key)}</strong><small>{item.task_id} · {item.manifest_digest ? short(item.manifest_digest, 18) : '无摘要'}</small></span></span><span><strong>{item.cause_code}</strong><small>{item.cause_summary}</small>{item.cleanup_error && <small className="is-danger-text">{item.cleanup_error}</small>}</span><span><span className={`config-state is-${statusTone(item.status)}`}>{cleanupStatusLabel(item.status)}</span></span><span>{item.attempt_count}</span><span><Clock3 size={13}/>{dateTime(item.next_retry_at)}</span><span>{dateTime(item.updated_at)}</span><span>{(item.status === 'pending' || item.status === 'failed') ? <button className="text-action" disabled={busy === item.id} onClick={() => void retry(item)}>{busy === item.id ? '处理中…' : '重试清理'}</button> : <span className="operations-muted">无需处理</span>}</span></div>)}</div>}
    </section>
  </div>;
}
