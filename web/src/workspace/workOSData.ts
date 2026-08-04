import type { AdminWorkOSView, Environment, ProjectSOPView, SOPDefinition, SOPSummary, SOPVersion, StageDefinition, GateDefinition, WorkTask, WorkTaskView } from '../types';

function list<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

function normalizeStage(stage: StageDefinition): StageDefinition {
  return {
    ...stage,
    owner_roles: list(stage.owner_roles),
    input_refs: list(stage.input_refs),
	accepted_input_types: list(stage.accepted_input_types),
	required_output_types: list(stage.required_output_types),
	output_schema_refs: list(stage.output_schema_refs),
    required_capabilities: list(stage.required_capabilities),
    execution_modes: list(stage.execution_modes),
    checks: list(stage.checks),
	gate_ids: list(stage.gate_ids),
	retry_policy: {...(stage.retry_policy || {}), retryable_error_codes: list(stage.retry_policy?.retryable_error_codes)},
	cost_policy: stage.cost_policy || {}
  };
}

function normalizeGate(gate: GateDefinition): GateDefinition {
  return {...gate, assignee_roles: list(gate.assignee_roles), input_refs: list(gate.input_refs), checks: list(gate.checks)};
}

export function normalizeSOPVersion(value: SOPVersion): SOPVersion {
  return {...value, content_types: list(value.content_types), stages: list(value.stages).map(normalizeStage), gates: list(value.gates).map(normalizeGate)};
}

export function normalizeSOPSummary(value: SOPSummary): SOPSummary {
  const definition: SOPDefinition = {...value.definition, content_types: list(value.definition.content_types)};
  return {...value, definition, versions: list(value.versions).map(normalizeSOPVersion)};
}

export function normalizeEnvironment(value: Environment): Environment {
  return {...value, capabilities: list(value.capabilities)};
}

export function normalizeAdminWorkOSView(value: AdminWorkOSView): AdminWorkOSView {
  const usage = value.usage || {task_count: 0, running_count: 0, waiting_gate_count: 0, by_execution_mode: {}};
  return {
    ...value,
    environments: list(value.environments).map(normalizeEnvironment),
    sops: list(value.sops).map(normalizeSOPSummary),
    gates: list(value.gates),
    capabilities: list(value.capabilities),
    audit: list(value.audit),
    usage: {...usage, by_execution_mode: usage.by_execution_mode || {}}
  };
}

export function normalizeProjectSOPView(value: ProjectSOPView): ProjectSOPView {
  return {...value, sop: normalizeSOPVersion(value.sop)};
}

function normalizeTask(value: WorkTask): WorkTask {
  return {...value, input_refs: list(value.input_refs), requested_output: value.requested_output || {}};
}

export function normalizeWorkTask(value: WorkTask): WorkTask {
  return normalizeTask(value);
}

export function normalizeWorkTaskView(value: WorkTaskView): WorkTaskView {
  return {
    ...value,
    task: normalizeTask(value.task),
    environment: normalizeEnvironment(value.environment),
		sop: normalizeSOPVersion(value.sop),
		source_revisions: list(value.source_revisions),
		knowledge_snapshots: list(value.knowledge_snapshots).map(snapshot => ({...snapshot, objects: list(snapshot.objects)})),
		approved_snapshots: list(value.approved_snapshots).map(snapshot => ({...snapshot, eligible_ids: list(snapshot.eligible_ids), artifacts: list(snapshot.artifacts)})),
		stage_runs: list(value.stage_runs).map(run => ({...run, input_refs: list(run.input_refs), output_refs: list(run.output_refs), outputs: list(run.outputs).map(output => ({...output, metadata: output.metadata || {}}))})),
    runs: list(value.runs).map(run => ({...run, output_refs: list(run.output_refs), delivery_profiles: list(run.delivery_profiles)})),
    gates: list(value.gates).map(gate => ({...gate, input_refs: list(gate.input_refs), checks: gate.checks || {}})),
    revisions: list(value.revisions).map(revision => ({...revision, content: revision.content || {}, knowledge_snapshot_ids: list(revision.knowledge_snapshot_ids), evidence_summary: revision.evidence_summary || {}, rights_summary: revision.rights_summary || {}})),
		deliveries: list(value.deliveries).map(delivery => ({...delivery, manifest: list(delivery.manifest)})),
		stage_outputs: list(value.stage_outputs).map(output => ({...output, metadata: output.metadata || {}})),
		media_jobs: list(value.media_jobs).map(job => ({...job, input_artifact_refs: list(job.input_artifact_refs)})),
		provider_attempts: list(value.provider_attempts).map(attempt => ({...attempt, safe_request_summary: attempt.safe_request_summary || {}, safe_response_summary: attempt.safe_response_summary || {}, disclosure_manifest: attempt.disclosure_manifest || {}})),
		media_reviews: list(value.media_reviews).map(review => ({...review, checks: review.checks || {}})),
		delivery_packages: list(value.delivery_packages).map(pkg => ({...pkg, approved_snapshot_ids: list(pkg.approved_snapshot_ids), manifest: list(pkg.manifest).map(artifact => ({...artifact, metadata: artifact.metadata || {}}))})),
		artifacts: list(value.artifacts).map(artifact => ({...artifact, metadata: artifact.metadata || {}})),
    allowed_actions: list(value.allowed_actions)
  };
}

export function normalizeWorkTaskList(value: WorkTask[]): WorkTask[] {
  return list(value).map(normalizeTask);
}
