import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { RuntimeJobDetail } from '../types';
import { RuntimeDetail } from './views/AdminRuntimePage';

const detail: RuntimeJobDetail = {
  summary: {
    id: 'job-forked-1', work_task_id: 'task-1', task_title: '恢复任务', customer_name: '果木食品客户', project_id: 'project-1', project_name: '果木食品', product_name: '品牌短视频', product_version: 2, current_step_name: '生成分镜', completed_steps: 1, total_steps: 3, task_status: 'running', task_next_action: '继续处理', state: 'running', status_since: '2026-08-08T02:00:00Z', blocking_reason: '任务正在按计划处理', recommended_action: '继续观察当前任务', cost: { status: 'not_recorded', amount_minor: 0, effect_count: 0 }, plan_digest: 'sha256:plan', binding_digest: 'sha256:binding', input_digest: 'sha256:input', runtime_policy_id: 'runtime-policy/customer-studio-v1', contract_major: 1, contract_minor: 0, root_job_run_id: 'job-source-1', source_job_run_id: 'job-source-1', checkpoint_id: 'checkpoint-1', priority: 1, allowed_actions: ['replay', 'refresh', 'cancel'], node_count: 1, node_states: { running: 1 }, effect_count: 1, checkpoint_count: 1, created_at: '2026-08-08T02:00:00Z', updated_at: '2026-08-08T02:01:00Z'
  },
  plan: { id: 'plan-1', sop_id: 'sop-1', sop_version: 1, sop_digest: 'sha256:sop', schema_version: 'contentcloud.job-plan/1.0', digest: 'sha256:plan', customer_steps: [], compiled_at: '2026-08-08T02:00:00Z' },
  nodes: [], attempts: [], events: [], agents: [], gates: [], state_collections: [],
  effects: [{ id: 'effect-1', node_run_id: 'node-1', kind: 'media.generate', state: 'unknown', request_digest: 'sha256:req', cost_minor: 0, currency: 'CNY', safe_summary: {}, version: 2, allowed_actions: ['reconcile'], created_at: '2026-08-08T02:00:00Z', updated_at: '2026-08-08T02:00:30Z' }],
  checkpoints: [{ id: 'checkpoint-1', node_key: 'brief', plan_digest: 'sha256:plan', state_ref_count: 1, output_ref_count: 1, completed_nodes: ['brief'], digest: 'sha256:checkpoint', allowed_actions: [], blocked_reason: '先暂停或结束源执行实例', created_at: '2026-08-08T02:00:00Z' }],
  generated_at: '2026-08-08T02:01:00Z'
};

describe('runtime recovery work surface', () => {
  it('renders replay, checkpoint fork, lineage and unknown-effect reconciliation controls', () => {
    const replay = { tenant_id: 'tenant-1', job_run_id: 'job-forked-1', event_count: 4, last_sequence: 9, external_calls: 0, projection_rebuilt: true, integrity_status: 'verified' };
    const markup = renderToStaticMarkup(<RuntimeDetail detail={detail} busy="" initialTab="requests" replay={replay} onAction={async () => {}} onReplay={async () => {}} onForkCheckpoint={async () => {}} onReconcileEffect={async () => {}} />);
    expect(markup).toContain('从安全检查点创建，源任务 job-sour');
    expect(markup).toContain('结果待核对');
    expect(markup).toContain('核对结果');
    expect(markup).not.toContain('重新提交 Provider');
    const technical = renderToStaticMarkup(<RuntimeDetail detail={detail} busy="" initialTab="technical" replay={replay} onAction={async () => {}} onReplay={async () => {}} onForkCheckpoint={async () => {}} onReconcileEffect={async () => {}} />);
    expect(technical).toContain('重新整理任务状态');
    expect(technical).toContain('任务状态已重新整理；完整性已验证，已核对 4 条记录，外部服务调用 0 次，最新记录编号 9。');
    expect(technical).toContain('处理规则');
    expect(technical).toContain('runtime-policy/customer-studio-v1');
  });

  it('keeps the business overview customer-facing and separates diagnostics', () => {
    const markup = renderToStaticMarkup(<RuntimeDetail detail={detail} busy="" onAction={async () => {}} onReplay={async () => {}} onForkCheckpoint={async () => {}} onReconcileEffect={async () => {}} />);
    expect(markup).toContain('任务概况');
    expect(markup).toContain('果木食品客户');
    expect(markup).toContain('建议下一步');
    expect(markup).toContain('先暂停或结束源执行实例');
    expect(markup).not.toContain('ContextView');
    expect(markup).not.toContain('执行绑定摘要');
  });
});
