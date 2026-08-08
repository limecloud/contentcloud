export interface User { id: string; email: string; display_name: string }
export interface Tenant { id: string; name: string; slug: string; status:string; created_at:string }
export interface Session { user: User; tenant: Tenant; role: string; is_platform_admin:boolean }
export type ContentType = 'video_script'|'marketing_video'|'wechat_article';
export interface PlatformTenant extends Tenant { member_count:number; project_count:number; device_count:number; active_run_count:number; content_types:ContentType[]; last_activity_at?:string }
export interface PlatformUserMembership { tenant_id:string; tenant_name:string; role:string; status:string }
export interface PlatformUser { id:string; email:string; display_name:string; verified_at?:string; created_at:string; is_platform_admin:boolean; memberships:PlatformUserMembership[] }
export interface PlatformOverview { counts:{tenants:number;active_tenants:number;users:number;projects:number;online_devices:number;active_runs:number}; tenants:PlatformTenant[]; users:PlatformUser[]; generated_at:string }
export interface Project { id: string; brand_name: string; product_name: string; content_type: string; channel: string; stage_objective: string; status: string; owner_name: string; reviewer_name: string; client_approver: string; row_version:number; connected_devices: number; knowledge_ready: number; open_blockers: number; updated_at: string }
export interface ProjectTemplate { id:string; name:string; channel:string; stage_objective:string; created_by:string; created_at:string }
export interface Run { id: string; project_id: string; capability_id:string; state: string; task_type: string; progress_label?: string; error_code?: string; created_at: string }
export interface Audit { id: string; action: string; subject_type: string; subject_id: string; summary: Record<string, unknown>; created_at: string }
export interface Submission { id:string; project_id:string; workspace_id:string; submission_type:string; status:string; current_revision_id:string; created_by:string; created_at:string; updated_at:string }
export interface SourceDisclosure { id:string; source_ref:string; level:'metadata_only'|'evidence_pack'|'full_source'; sha256:string; byte_size:number; evidence_pack?:unknown }
export interface SubmissionObjectRef {id:string;type:string;version:number;digest:string;path:string;content:Record<string,unknown>|unknown[]}
export interface SubmissionRevision { id:string; project_id:string; workspace_id:string; submission_id:string; revision_no:number; schema_version:string; content_hash:string; base_snapshot_ids:string[]; environment_digest:string; local_run_summary:{run_id?:string;stage?:string;checks:{name:string;status:string;detail?:string}[];input_hash?:string;output_hash?:string;versions?:Record<string,string>}; objects:SubmissionObjectRef[]; artifacts:{name:string;media_type:string;sha256:string;byte_size:number}[]; message?:string; idempotency_key:string; evidence_limited:boolean; created_by:string; created_at:string; source_disclosures:SourceDisclosure[] }
export interface SubmissionDetails { submission:Submission; revisions:SubmissionRevision[]; comments:ReviewComment[] }
export interface SubmissionRevisionView { submission:Submission; revision:SubmissionRevision; comments:ReviewComment[] }
export interface ApprovedSnapshot { id:string; tenant_id?:string; project_id:string; workspace_id:string; submission_id:string; submission_revision_id:string; submission_type:string; schema_version:string; content_hash:string; subject_hash:string; canonical_content?:unknown; eligible_ids:string[]; artifacts:{name:string;media_type:string;sha256:string;byte_size:number}[]; decision_id:string; created_by:string; created_at:string }
export interface Dashboard { tenant: Tenant; projects: Project[]; recent_runs: Run[]; recent_audit: Audit[]; counts: Record<string, number>; pipeline: {id: string; label: string; count: number; blocked: number}[] }
export interface EnvironmentCapability { id:string; version:string; enabled:boolean }
export interface Environment { id:string; tenant_id:string; name:string; slug:string; status:'active'|'paused'|string; manifest_digest:string; default_sop_id?:string; default_sop_version?:number; capabilities:EnvironmentCapability[]; created_at:string; updated_at:string }
export interface StageObjectRequirement { output_type:string; role?:string; min_status?:string; min_count?:number }
export interface StageRetryPolicy { max_attempts?:number; backoff_seconds?:number; allow_partial_retry?:boolean; retryable_error_codes?:string[] }
export interface StageCostPolicy { currency?:string; max_estimated_cost_minor?:number; require_approval_above_minor?:number; estimate_ttl_seconds?:number }
export interface StageDefinition { stage_id:string; name:string; order:number; owner_roles:string[]; input_refs:string[]; output_schema:string; output_schema_refs:string[]; required_capabilities:string[]; execution_modes:string[]; checks:string[]; gate_ids:string[]; retry_max_attempts:number; accepted_input_types:StageObjectRequirement[]; required_output_types:StageObjectRequirement[]; completion_policy?:string; executor_policy?:string; retry_policy:StageRetryPolicy; cost_policy:StageCostPolicy }
export interface GateDefinition { gate_id:string; name:string; mode:'none'|'advisory'|'required'|'required_check'|'internal_review'|'client_decision'|string; blocking:boolean; assignee_roles:string[]; input_refs:string[]; checks:string[]; on_reject:string; escalation_hours:number }
export interface SOPDefinition { id:string; tenant_id:string; name:string; description:string; content_types:string[]; current_version:number; template_key?:string; built_in?:boolean; source_ref?:string; created_by:string; created_at:string; updated_at:string }
export interface SOPVersion { id:string; tenant_id:string; sop_id:string; version:number; schema_version:string; name:string; description:string; content_types:string[]; stages:StageDefinition[]; gates:GateDefinition[]; default_execution_mode:string; digest:string; status:'draft'|'published'|'retired'|string; created_by:string; published_by?:string; created_at:string; published_at?:string }
export interface SOPSummary { definition:SOPDefinition; versions:SOPVersion[] }
export interface SOPLintIssue { code:string; path:string; message:string }
export interface SOPLintReport { valid:boolean; errors:SOPLintIssue[]; warnings:SOPLintIssue[] }
export interface SOPDiffChange { path:string; before:unknown; after:unknown }
export interface SOPVersionDiff { sop_id:string; from_version:number; to_version:number; same:boolean; changes:SOPDiffChange[] }
export interface SOPEnvironmentImpact { environment_id:string; name:string; status:string; default_sop_id:string; default_sop_version:number }
export interface SOPProjectImpact { project_id:string; name:string; status:string; bound_sop_version:number }
export interface SOPTaskImpact { task_id:string; project_id:string; title:string; status:string; sop_version:number; task_run_bound:boolean }
export interface SOPVersionImpact { sop_id:string; version:number; environments:SOPEnvironmentImpact[]; projects:SOPProjectImpact[]; tasks:SOPTaskImpact[]; counts:Record<string,number> }
export interface SOPRollbackResult { version:SOPVersion; target_version:number; previous_version:number; rebound_environments:number; rebound_projects:number; impact:SOPVersionImpact }
export interface GateSummary { sop_id:string; sop_name:string; sop_version:number; gate_id:string; name:string; mode:string; blocking:boolean; usage_count:number }
export interface UsageSummary { task_count:number; running_count:number; waiting_gate_count:number; by_execution_mode:Record<string,number> }
export interface AdminCapability { id:string; version:string; kind:string; input_schema?:string; output_schema?:string; presentation_profiles?:string[]; local_only:boolean; digest:string }
export interface AdminWorkOSView { environments:Environment[]; sops:SOPSummary[]; gates:GateSummary[]; capabilities:AdminCapability[]; audit:Audit[]; usage:UsageSummary; generated_at:string }
export interface OperationsExecutorProject { id:string; brand_name:string; product_name:string; status:string }
export interface OperationsExecutor { id:string; tenant_id:string; display_name:string; executor_type:string; status:'online'|'offline'|'revoked'|string; status_reason:string; hostname:string; platform:string; arch:string; version:string; capabilities:AdminCapability[]; projects:OperationsExecutorProject[]; last_seen_at:string; revoked_at?:string }
export interface OperationsExecutorDirectory { executors:OperationsExecutor[]; generated_at:string; online_window_seconds:number }
export interface OperationsSkillSource { repository:string; ref:string; license:string }
export interface OperationsSkillSignature { status:string; algorithm:string; key_id:string }
export interface OperationsSkillDataFlow { local_by_default:boolean; cloud_actions:string[] }
export interface OperationsSkillCost { model:string; currency?:string; unit?:string; unit_price?:string; notice:string }
export interface OperationsSkillEvaluation { status:string; report?:string; digest?:string; evidence:string[] }
export interface OperationsSkillRevocation { status:string; severity?:string; reason?:string }
export interface OperationsSkill { id:string; version:string; digest:string; kind:string; lifecycle:string; available_for_new_runs:boolean; source:OperationsSkillSource; signature:OperationsSkillSignature; compatible_profiles:string[]; permissions:string[]; data_flow:OperationsSkillDataFlow; cost:OperationsSkillCost; output_schemas:string[]; evaluation:OperationsSkillEvaluation; revocation:OperationsSkillRevocation }
export interface OperationsSkillDirectory { configured:boolean; source?:string; registry_schema_version?:string; skills:OperationsSkill[]; generated_at:string }
export interface RuntimeJobSummary { id:string; work_task_id:string; task_title:string; project_id:string; project_name:string; state:string; plan_digest:string; priority:number; error_code?:string; node_count:number; node_states:Record<string,number>; effect_count:number; checkpoint_count:number; created_at:string; updated_at:string }
export interface RuntimeJobList { items:RuntimeJobSummary[]; next_after?:number; generated_at:string }
export interface RuntimeNodeView { id:string; node_key:string; name:string; kind:string; customer_step_id?:string; state:string; attempt_count:number; output_digest?:string; error_code?:string; lease_owner?:string; updated_at:string }
export interface RuntimeEventView { id:string; sequence:number; type:string; node_key?:string; actor_type:string; payload:Record<string,unknown>; occurred_at:string }
export interface RuntimeEffectView { id:string; node_run_id:string; kind:string; state:string; external_id?:string; request_digest:string; response_digest?:string; cost_minor:number; currency:string; safe_summary:Record<string,unknown>; error_code?:string; version:number; created_at:string; updated_at:string }
export interface RuntimeCheckpointView { id:string; node_key:string; plan_digest:string; state_ref_count:number; output_ref_count:number; completed_nodes:string[]; digest:string; created_at:string }
export interface RuntimePlanView { id:string; sop_id:string; sop_version:number; sop_digest:string; schema_version:string; digest:string; customer_steps:{id:string;title:string;node_keys:string[]}[]; compiled_at:string }
export interface RuntimeContextView { id:string; node_run_id:string; attempt_id:string; schema_version:string; input_ref_count:number; state_ref_count:number; event_ref_count:number; allowed_tools:string[]; max_tokens:number; budget_minor:number; digest:string; created_at:string; expires_at:string }
export interface RuntimeAgentView { id:string; node_run_id:string; parent_agent_instance_id?:string; role:string; harness_kind:string; execution_profile_id:string; context_view_id:string; session_bound:boolean; state:string; depth:number; remaining_descendants:number; budget_minor:number; used_cost_minor:number; version:number; context_view:RuntimeContextView; created_at:string; updated_at:string }
export interface RuntimeJobDetail { summary:RuntimeJobSummary; plan:RuntimePlanView; nodes:RuntimeNodeView[]; events:RuntimeEventView[]; effects:RuntimeEffectView[]; checkpoints:RuntimeCheckpointView[]; agents:RuntimeAgentView[]; generated_at:string }
export interface ProjectSOPBinding { tenant_id:string; project_id:string; environment_id:string; sop_id:string; sop_version:number; sop_digest:string; bound_by:string; bound_at:string }
export interface WorkTask { id:string; tenant_id:string; project_id:string; environment_id:string; sop_id:string; sop_version:number; sop_digest:string; title:string; intent:string; content_type:string; input_refs:string[]; requested_output:Record<string,unknown>; assignee_user_id?:string; priority:string; due_at?:string; risk_profile:string; status:string; current_stage_id:string; next_action:string; created_by:string; created_at:string; updated_at:string }
export interface InputItem { id:string; tenant_id:string; project_id?:string; source_type:string; title:string; summary:string; body?:string; source_ref?:string; source_digest?:string; disclosure:string; status:string; target_task_id?:string; assignee_user_id?:string; missing_fields:string[]; metadata:Record<string,unknown>; idempotency_key?:string; row_version:number; created_by:string; created_at:string; updated_at:string }
export interface TaskStageOutput { id:string; tenant_id:string; project_id:string; task_id:string; stage_run_id:string; stage_id:string; output_type:string; object_id:string; object_version?:number; object_digest:string; role:string; status:string; metadata:Record<string,unknown>; created_by:string; created_at:string }
export interface StageRun { id:string; tenant_id:string; task_id:string; stage_id:string; status:string; execution_mode:string; input_refs:string[]; output_refs:string[]; outputs:TaskStageOutput[]; started_at?:string; completed_at?:string; updated_at:string }
export interface TaskRun { id:string; tenant_id:string; project_id:string; work_task_id?:string; sop_id?:string; sop_version?:number; sop_digest?:string; stage_id?:string; execution_mode?:string; executor_kind?:string; output_refs?:string[]; task_revision_id?:string; gate_evaluation_id?:string; input_snapshot_id:string; task_type:string; capability_id:string; capability_version:string; input_schema:string; output_schema:string; output_count:number; delivery_profiles:string[]; state:string; priority:number; attempt_count:number; progress_label?:string; error_code?:string; created_at:string; updated_at:string }
export interface GateEvaluation { id:string; tenant_id:string; project_id:string; task_id:string; stage_run_id:string; gate_id:string; gate_mode:string; status:string; revision_id?:string; input_refs:string[]; checks:Record<string,unknown>; decision?:string; reason?:string; decided_by?:string; decided_at?:string; expires_at?:string; created_at:string; updated_at:string }
export interface TaskRevision { id:string; tenant_id:string; project_id:string; task_id:string; revision_no:number; content_type:string; schema_version:string; content:Record<string,unknown>; content_hash:string; sop_digest:string; knowledge_snapshot_ids:string[]; evidence_summary:Record<string,unknown>; rights_summary:Record<string,unknown>; status:string; submitted_by:string; submitted_at?:string; created_at:string }
export interface TaskDelivery { id:string; tenant_id:string; project_id:string; task_id:string; revision_id:string; destination:string; status:string; manifest:string[]; delivery_package_id?:string; integrity_status:string; delivery_digest:string; delivered_by?:string; delivered_at?:string; error_code?:string; created_at:string; updated_at:string }
export interface MediaGenerationJob { id:string; tenant_id:string; project_id:string; task_id:string; stage_run_id:string; storyboard_snapshot_id:string; prompt_package_artifact_id?:string; provider_id:string; profile_version:string; profile_digest:string; model:string; mode:string; aspect_ratio:string; duration_seconds:number; input_artifact_refs:string[]; state:string; estimated_cost_minor:number; actual_cost_minor:number; currency:string; attempt_count:number; max_attempts:number; lease_owner?:string; lease_expires_at?:string; cancel_requested_at?:string; error_code?:string; error_detail_safe?:string; row_version:number; created_by:string; created_at:string; updated_at:string }
export interface ProviderAttempt { id:string; tenant_id:string; project_id:string; generation_job_id:string; attempt_number:number; provider_id:string; request_digest:string; external_job_id?:string; provider_state:string; safe_request_summary:Record<string,unknown>; safe_response_summary:Record<string,unknown>; disclosure_manifest:Record<string,unknown>; http_status?:number; provider_request_id?:string; estimated_cost_minor:number; actual_cost_minor:number; currency:string; last_polled_at?:string; next_poll_at?:string; submitted_at?:string; downloaded_at?:string; completed_at?:string; retry_after_seconds?:number; error_code?:string; error_detail_safe?:string; created_at:string; updated_at:string }
export interface MediaReview { id:string; tenant_id:string; project_id:string; task_id:string; generation_job_id?:string; subject_artifact_id:string; subject_digest:string; review_kind:'technical'|'content'|'final'|string; status:string; checks:Record<string,unknown>; selected:boolean; decision_reason?:string; decided_by?:string; decided_at?:string; row_version:number; created_by:string; created_at:string; updated_at:string }
export interface ProviderProfile { provider_id:string; version:string; digest:string; adapter_version:string; model:string; region:string; modes:string[]; input_media_types:string[]; output_media_type:string; limits:Record<string,unknown>; data_retention:string; pricing:Record<string,unknown>; status:string; verified_at:string; expires_at:string }
export interface ProviderBinding { tenant_id:string; provider_id:string; profile_version:string; state:string; egress_policy:string; monthly_budget_minor:number; max_job_cost_minor:number; max_concurrency:number; max_retries:number; updated_by:string; updated_at:string }
export interface WorkTaskView { task:WorkTask; project:Project; environment:Environment; sop:SOPVersion; source_revisions:SourceRevision[]; knowledge_snapshots:KnowledgeSnapshot[]; approved_snapshots:ApprovedSnapshot[]; stage_runs:StageRun[]; runs:TaskRun[]; gates:GateEvaluation[]; revisions:TaskRevision[]; deliveries:TaskDelivery[]; stage_outputs:TaskStageOutput[]; media_jobs:MediaGenerationJob[]; provider_attempts:ProviderAttempt[]; media_reviews:MediaReview[]; delivery_packages:DeliveryPackage[]; artifacts:Artifact[]; allowed_actions:string[]; generated_at:string }
export interface ProjectSOPView { binding:ProjectSOPBinding; sop:SOPVersion }
export type KnowledgeObjectType = 'FactAssertion'|'Claim'|'Audience'|'Scenario'|'Insight'|'BrandRule'|'ConstraintRecord'|'Process'|'Campaign'|'Learning'|'Asset'|'RightsRecord'|'ConflictRecord'|'KnowledgeGap'|'DomainObject'|string;
export type KnowledgeLayer = 'identity'|'product'|'market'|'expression'|'operations'|'content_engine'|'compliance'|string;
export type KnowledgeObjectStatus = 'candidate'|'needs_review'|'pending'|'verified'|'approved'|'valid'|'active'|'blocked'|'conflicted'|'prohibited'|'expired'|'rejected'|'superseded'|'revoked'|'open'|'source_missing'|'collecting'|'resolved'|'waived'|'accepted_risk'|string;
export interface KnowledgeObject { id:string; tenant_id?:string; project_id:string; object_type:KnowledgeObjectType; layer:KnowledgeLayer; version:number; status:KnowledgeObjectStatus; title:string; statement:string; payload:Record<string, unknown>; dimensions:string[]; allowed_channels:string[]; evidence_refs:string[]; relation_refs:string[]; rights_refs:string[]; conflict_refs:string[]; decision_ref?:string; next_action?:string; impact?:string; valid_from?:string; valid_until?:string; expires_at?:string; digest:string; created_by?:string; created_at:string; updated_at:string }
export interface KnowledgeDecision { id:string; tenant_id?:string; project_id:string; object_id:string; previous_version:number; result_version:number; subject_digest:string; decision:'approve'|'reject'; reason:string; actor_id:string; created_at:string }
export interface KnowledgePackObjectRef { object_id:string; version:number }
export interface KnowledgeQueryPolicy { eligible_statuses:string[]; allowed_object_types:string[]; require_evidence:boolean; block_on_conflict:boolean; block_on_rights_failure:boolean }
export interface KnowledgePack { id:string; tenant_id?:string; project_id:string; name:string; purpose:string; version:number; status:'draft'|'published'|'retired'|string; object_refs:KnowledgePackObjectRef[]; query_policy:KnowledgeQueryPolicy; digest:string; created_by?:string; published_by?:string; created_at:string; published_at?:string }
export interface KnowledgeSnapshot { id:string; tenant_id?:string; project_id:string; pack_id:string; pack_version:number; pack_digest:string; objects:KnowledgeObject[]; digest:string; created_by?:string; created_at:string }
export interface KnowledgeQueryEntry { object_id:string; object_type:KnowledgeObjectType; layer:KnowledgeLayer; status:KnowledgeObjectStatus; evidence_refs:string[]; reasons?:string[] }
export interface KnowledgeGapResult { object_id:string; layer:KnowledgeLayer; next_action:string; impact?:string }
export interface KnowledgeQueryResult { snapshot_id:string; query_digest:string; eligible:KnowledgeQueryEntry[]; blocked:KnowledgeQueryEntry[]; gaps:KnowledgeGapResult[] }
export interface Source { id:string; tenant_id?:string; project_id:string; name:string; source_type:string; status:string; revision_count:number; latest_revision_id:string; created_at:string }
export interface SourceRevision { id:string; tenant_id?:string; project_id:string; source_id:string; file_name:string; sha256:string; byte_size:number; declared_mime:string; detected_mime:string; processing_status:string; parser_version?:string; error_code?:string; created_at:string }
export interface EvidenceSpan { id:string; tenant_id?:string; project_id:string; revision_id:string; locator_kind:string; locator:Record<string,unknown>; quote_text:string; quote_hash:string; ocr_confidence?:number; review_status:string; reviewed_by?:string; reviewed_at?:string; created_at:string }
export interface Source { id:string; project_id:string; name:string; source_type:string; status:string; revision_count:number; latest_revision_id:string; created_at:string }
export interface SourceRevision { id:string; source_id:string; file_name:string; sha256:string; byte_size:number; declared_mime:string; detected_mime:string; processing_status:string; parser_version?:string; error_code?:string; supersedes_id?:string; uploaded_by?:string; effective_from?:string; effective_to?:string; created_at:string }
export interface EvidenceSpan { id:string; revision_id:string; locator_kind:string; locator:Record<string,unknown>; quote_text:string; quote_hash:string; ocr_confidence?:number; review_status:string; reviewed_by?:string; reviewed_at?:string }
export interface SourceImpactItem { object_type:string; object_id:string; reason:string; current_status:string; suggested_action:string }
export interface Asset { id:string; project_id:string; name:string; asset_type:string; source_revision_id:string; usage_mode:string; status:string; created_by:string; created_at:string; updated_at:string }
export interface RightsRecord { id:string; project_id:string; asset_id:string; rights_holder:string; rights_type:string; territories:string[]; channels:string[]; valid_from?:string; valid_until?:string; proof_source_revision_id:string; restrictions:string[]; status:string; reviewed_by?:string; reviewed_at?:string; row_version:number; created_at:string; updated_at:string }
export interface Device { id: string; display_name: string; platform: string; arch: string; daemon_version: string; last_seen_at: string; capabilities: {id:string;version:string}[] }
export interface ReviewComment { id:string; review_cycle_id:string; subject_id:string; carried_from_comment_id?:string; shot_id?:string; json_pointer?:string; body:string; visibility:string; author_id:string; resolved_at?:string; created_at:string }
export interface ReviewCycle { id:string; subject_id:string; cycle_number:number; status:string; conclusion?:string; assignee_user_id?:string; opened_by:string; decided_by?:string; opened_at:string; decided_at?:string }
export interface ReviewGrant { id:string; subject_id:string; reviewer_email:string; expires_at:string; verified_at?:string; revoked_at?:string; decision_at?:string; created_at:string; review_token?:string; dev_otp?:string }
export interface Membership { tenant_id:string; user_id:string; role:string; status:string; created_at:string; revoked_at?:string }
export interface Member { membership:Membership; email:string; display_name:string }
export interface MembershipInvite { id:string; email:string; role:string; status:string; expires_at:string; accepted_by?:string; accepted_at?:string; revoked_at?:string; created_at:string; invite_token?:string }
export interface Artifact { id:string; tenant_id?:string; project_id:string; approved_snapshot_id:string; kind:'delivery'|'storyboard_image'|'storyboard_media'|'generated_video'|'final_render'|string; capability_id:string; capability_version:string; capability_digest:string; schema_id:string; media_type:string; file_name:string; sha256:string; byte_size:number; visibility:string; retention_class:string; purpose:string; metadata:Record<string,unknown>; created_at:string }
export interface DeliveryPackage { id:string; tenant_id?:string; project_id:string; approved_snapshot_ids:string[]; content_item_id:string; status:string; manifest:Artifact[]; created_by:string; created_at:string }
export interface StoryboardAsset { id:string; role:string; shot_id?:string; path:string; media_type:string; sha256:string; byte_size:number; rights_refs:string[] }
export interface StoryboardShot { shot_id:string; start_ms:number; end_ms:number; role:string; first_frame_artifact_id:string; end_frame_artifact_id?:string; image_prompt_zh:string; subject:string; product:string; scene:string; composition:string; lighting:string; camera:string; action:string; incoming_state:string; outgoing_state:string; movement_axis:string; lighting_lock:string; product_lock:string; anchors:string[]; asset_refs:string[]; rights_refs:string[]; knowledge_refs:string[]; claim_refs:string[]; negative_constraints:string[]; acceptance_criteria:string[]; plan_b:string }
export interface StoryboardPackage { id:string; type:'storyboard_package'; schema_version:string; project_id:string; approved_snapshot_id:string; content_item_id:string; generator_capability:{id:string;version:string;digest:string}; status:string; shots:StoryboardShot[]; assets:StoryboardAsset[]; review_sheet_artifact_id?:string; rights_refs:string[]; source_digest:string; locked_digest:string }
export interface PerformanceObservation { id:string; project_id:string; import_batch_id:string; row_number:number; approved_snapshot_id:string; platform:string; account_alias:string; published_at:string; window_hours:number; sample_status:string; metrics:Record<string,number>; currency?:string; spend:number; gmv:number; roi?:number; dedup_key:string; issue_category:string; notes:string; created_at:string }
export interface PerformanceImportBatch { id:string; project_id:string; source_name:string; source_format:'manual'|'json'|'csv'|'xlsx'; source_sha256:string; currency?:string; row_count:number; imported_count:number; status:'imported'; created_by:string; created_at:string }
export interface RatingDecision { id:string; project_id:string; subject_type:'approved_snapshot'; subject_id:string; observation_ids:string[]; rating:'seed_candidate'|'repairable'|'discarded'|'insufficient_sample'; reason:string; next_action:string; created_by:string; created_at:string }
export interface LineageNode { key:string; type:string; id:string; label:string; status:string; stage:string; created_at:string; metadata?:Record<string,unknown> }
export interface LineageEdge { from:string; to:string; relation:string; reason:string }
export interface LineageGraph { project_id:string; focus_key?:string; direction:'both'|'upstream'|'downstream'; nodes:LineageNode[]; edges:LineageEdge[]; stage_count:Record<string,number>; generated_at:string }
export interface ImpactItem { node:LineageNode; depth:number; severity:'blocked'|'attention'|'review'; reason:string; current_status:string; recommended_action:string }
export interface ImpactAnalysis { project_id:string; focus?:LineageNode; items:ImpactItem[]; generated_at:string }
export interface ContentShot { shot_id:string; start_ms:number; end_ms:number; role:string; narrative_purpose:string; subject:string; visual_intent:string; subject_action:string; composition:string; camera_motion:string; voiceover?:string; on_screen_text?:string; acceptance_criteria:string[] }
export interface ContentItem { id:string; type:'content_item'; status:string; schema_version:'contentcloud.content-item/3.0'; deliverability:string; project_id:string; content_id:string; content_batch_id:string; title:string; channel:string; duration_ms:number; aspect_ratio:string; direction:{title:string;angle:string}; experiment:{primary_variable:string;hypothesis:string}; shots:ContentShot[] }
export interface ArticleTitle { id:string; text:string; strategy:string; risk_refs:string[] }
export interface ArticleAssertion { id:string; type:'fact'|'commercial_claim'|'quotation'|'editorial_opinion'|'personal_experience'|'hypothesis'; knowledge_refs:string[]; evidence_refs:string[]; attribution:string }
export interface ArticleBlock { id:string; type:'heading'|'paragraph'|'list'|'quote'|'image'|'callout'|'divider'|'cta'; text:string; level:number; ordered:boolean; items:string[]; asset_ref:string; rights_ref:string; alt_text:string; caption:string; purpose:string; callout_kind:string; target:string; assertions:ArticleAssertion[]; style_marks:string[] }
export interface ArticleItem { id:string; type:'article_item'; status:string; schema_version:'contentcloud.article/1.0'; deliverability:string; project_id:string; content_id:string; content_batch_id:string; brief_ref:string; language:string; title_candidates:ArticleTitle[]; selected_title_id:string; summary:string; author_display_name:string; blocks:ArticleBlock[]; attribution:{source_names:string[];disclosure:string} }
export interface SubmissionReviewSubject { submission_id:string; submission_revision_id:string; subject_hash:string; schema_version:string; base_snapshot_ids:string[]; environment_digest:string; object_refs:SubmissionObjectRef[]; objects:Record<string,unknown>[] }
export interface ReviewProjection { project:Project; submission?:SubmissionReviewSubject; comments:ReviewComment[]; verified:boolean }

export interface ProjectionSection {
  status:'empty'|'ready'|'pending'|'blocked'|string;
  count:number;
  pending:number;
  blocked:number;
  updated_at?:string;
}
export interface ProjectionGovernance {
  pending_reviews:number;
  changes_requested:number;
  blocked_content_batches:number;
  active_automation_runs:number;
}
export interface ProjectionSubmission {
  id:string;
  type:string;
  status:string;
  current_revision_id:string;
  updated_at:string;
}
export interface ProjectionSnapshot {
  id:string;
  type:string;
  submission_revision_id:string;
  eligible_count:number;
  created_at:string;
}
export interface ProjectionAction {
  id:string;
  kind:string;
  label:string;
  enabled:boolean;
  reason?:string;
  navigation:{view:string;focus?:{kind:string;id:string;digest?:string}};
}
export interface ProjectProjection {
  schema_version:'contentcloud.project-projection/3.0'|string;
  project:Pick<Project,'id'|'brand_name'|'product_name'|'channel'|'stage_objective'|'status'>;
  sections:Record<string,ProjectionSection>;
  governance:ProjectionGovernance;
  submissions:ProjectionSubmission[];
  snapshots:ProjectionSnapshot[];
  next_actions:ProjectionAction[];
  generated_at:string;
}
