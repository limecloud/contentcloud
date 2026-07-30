export interface User { id: string; email: string; display_name: string }
export interface Tenant { id: string; name: string; slug: string; status:string; created_at:string }
export interface Session { user: User; tenant: Tenant; role: string; is_platform_admin:boolean }
export type ContentType = 'video_script'|'wechat_article';
export interface PlatformTenant extends Tenant { member_count:number; project_count:number; device_count:number; active_run_count:number; content_types:ContentType[]; last_activity_at?:string }
export interface PlatformUserMembership { tenant_id:string; tenant_name:string; role:string; status:string }
export interface PlatformUser { id:string; email:string; display_name:string; verified_at?:string; created_at:string; is_platform_admin:boolean; memberships:PlatformUserMembership[] }
export interface PlatformOverview { counts:{tenants:number;active_tenants:number;users:number;projects:number;online_devices:number;active_runs:number}; tenants:PlatformTenant[]; users:PlatformUser[]; generated_at:string }
export interface Project { id: string; brand_name: string; product_name: string; channel: string; stage_objective: string; status: string; owner_name: string; reviewer_name: string; client_approver: string; row_version:number; connected_devices: number; knowledge_ready: number; open_blockers: number; updated_at: string }
export interface ProjectTemplate { id:string; name:string; channel:string; stage_objective:string; created_by:string; created_at:string }
export interface Run { id: string; project_id: string; capability_id:string; state: string; task_type: string; progress_label?: string; error_code?: string; created_at: string }
export interface Audit { id: string; action: string; subject_type: string; subject_id: string; summary: Record<string, unknown>; created_at: string }
export interface Submission { id:string; project_id:string; workspace_id:string; submission_type:string; status:string; current_revision_id:string; created_by:string; created_at:string; updated_at:string }
export interface SourceDisclosure { id:string; source_ref:string; level:'metadata_only'|'evidence_pack'|'full_source'; sha256:string; byte_size:number; evidence_pack?:unknown }
export interface SubmissionObjectRef {id:string;type:string;version:number;digest:string;path:string;content:Record<string,unknown>|unknown[]}
export interface SubmissionRevision { id:string; project_id:string; workspace_id:string; submission_id:string; revision_no:number; schema_version:string; content_hash:string; base_snapshot_ids:string[]; environment_digest:string; local_run_summary:{run_id?:string;stage?:string;checks:{name:string;status:string;detail?:string}[];input_hash?:string;output_hash?:string;versions?:Record<string,string>}; objects:SubmissionObjectRef[]; artifacts:{name:string;media_type:string;sha256:string;byte_size:number}[]; message?:string; idempotency_key:string; evidence_limited:boolean; created_by:string; created_at:string; source_disclosures:SourceDisclosure[] }
export interface SubmissionDetails { submission:Submission; revisions:SubmissionRevision[]; comments:ReviewComment[] }
export interface SubmissionRevisionView { submission:Submission; revision:SubmissionRevision; comments:ReviewComment[] }
export interface ApprovedSnapshot { id:string; project_id:string; workspace_id:string; submission_id:string; submission_revision_id:string; submission_type:string; schema_version:string; content_hash:string; subject_hash:string; canonical_content?:unknown; eligible_ids:string[]; artifacts:{name:string;media_type:string;sha256:string;byte_size:number}[]; decision_id:string; created_by:string; created_at:string }
export interface Dashboard { tenant: Tenant; projects: Project[]; recent_runs: Run[]; recent_audit: Audit[]; counts: Record<string, number>; pipeline: {id: string; label: string; count: number; blocked: number}[] }
export interface Evidence { source_revision_id: string; locator_kind: string; locator: string; quote: string }
export interface TypedValue { type:string; text?:string; number?:number; boolean?:boolean; unit?:string }
export interface KnowledgeScope { regions:string[]; channels:string[]; audiences:string[]; product_variants:string[] }
export interface Knowledge { id: string; project_id: string; kind: string; title: string; statement: string; subject:string; predicate:string; value:TypedValue; scope:KnowledgeScope; status: string; risk_level: string; allowed_channels: string[]; evidence: Evidence[]; forbidden_extensions:string[]; depends_on_fact_ids:string[]; valid_from?:string; valid_until?:string; expires_at?:string; approved_by?:string; approved_at?:string; row_version: number }
export interface KnowledgeConflict { id:string; project_id:string; subject:string; predicate:string; knowledge_item_ids:string[]; reason:string; status:string; resolved_by?:string; resolved_at?:string; resolution?:string; created_at:string }
export interface DecisionRequest { id:string; project_id:string; conflict_id:string; question:string; knowledge_item_ids:string[]; status:string; requested_by:string; resolved_by?:string; resolved_at?:string; selected_knowledge_id?:string; notes?:string; created_at:string }
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
export interface Artifact { id:string; project_id:string; approved_snapshot_id:string; kind:'delivery'; capability_id:string; capability_version:string; capability_digest:string; schema_id:string; media_type:string; file_name:string; sha256:string; byte_size:number; visibility:string; retention_class:string; purpose:string; metadata:Record<string,unknown>; created_at:string }
export interface DeliveryPackage { id:string; project_id:string; approved_snapshot_ids:string[]; content_item_id:string; status:string; manifest:Artifact[]; created_by:string; created_at:string }
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
