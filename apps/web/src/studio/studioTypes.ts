export interface StudioUser {
  id:string;
  display_name:string;
}

export interface StudioTenant {
  id:string;
  name:string;
}

export interface StudioSession {
  user:StudioUser;
  tenant:StudioTenant;
  role:string;
  operations_path?:string;
  can_create:boolean;
  can_connect_execution_client:boolean;
  can_manage_team:boolean;
  can_view_operations:boolean;
}

export interface StudioProject {
  id:string;
  brand_name:string;
  product_name:string;
  content_type:string;
  channel:string;
  status:string;
  execution_client_connected:boolean;
  connected_client_count:number;
}

export interface StudioExecutionClient {
  id:string;
  display_name:string;
  available:boolean;
}

export interface StudioExecutionClientCatalog {
  clients:StudioExecutionClient[];
}

export type StudioConnectionStatus='waiting_for_computer'|'connecting'|'confirmation_required'|'connected'|'failed'|'expired'|'canceled';

export interface StudioConnectSession {
  id:string;
  project_id:string;
  status:StudioConnectionStatus;
  message:string;
  requires_confirmation:boolean;
  verification_code?:string;
  support_code?:string;
  expires_at:string;
}

export interface StudioExperience {
  id:string;
  version:string;
  name:string;
  description:string;
  content_type:string;
  status:string;
  project_ids:string[];
  step_titles:string[];
  available_collection_methods:string[];
  unavailable_collection_methods:string[];
  workbench:StudioWorkbench;
}

export interface StudioWorkbench {
  plugin_id:string;
  version:string;
  digest:string;
  layout:'stage-canvas-context'|'article-editor'|'product-variants'|'novel-editor'|string;
  density:'compact'|'comfortable'|'dense'|string;
  theme:string;
  navigation:StudioWorkbenchNavigation[];
  stages:StudioWorkbenchStage[];
  panels?:StudioWorkbenchPanel[];
}

export interface StudioWorkbenchNavigation {
  id:string;
  label:string;
  icon:string;
}

export interface StudioWorkbenchStage {
  id:string;
  label:string;
  outcome:string;
  primary_action:string;
}

export interface StudioWorkbenchPanel {
  id:string;
  title:string;
  detail:string;
  tone:'source'|'knowledge'|'strategy'|'production'|'review'|string;
  icon:string;
  stage_ids:string[];
  target:'start'|'tasks'|'assets'|'deliveries'|string;
  action_label:string;
}

export interface StudioBootstrap {
  session:StudioSession;
  tenants:StudioTenant[];
  projects:StudioProject[];
  experiences:StudioExperience[];
  generated_at:string;
}

export type StudioStepStatus='completed'|'working'|'ready'|'needs_input'|'needs_decision'|'blocked'|'not_started';

export interface StudioCustomerStep {
  id:string;
  title:string;
  outcome_description:string;
  status:StudioStepStatus;
  progress_summary:string;
}

export interface StudioTaskSummary {
  id:string;
  project:StudioProject;
  experience_id:string;
  title:string;
  intent:string;
  content_type:string;
  status:string;
  status_label:string;
  current_step_id:string;
  next_action:string;
  asset_count:number;
  created_at:string;
  updated_at:string;
}

export interface StudioInspiration {
  id:string;
  title:string;
  summary:string;
  source_type:string;
  source_label:string;
  saved_as_project_reference:boolean;
  created_at:string;
}

export interface StudioDecision {
  id:string;
  title:string;
  summary:string;
  result_count:number;
  can_decide:boolean;
}

export interface StudioDownload {
  id:string;
  file_name:string;
  media_type:string;
  byte_size:number;
  href:string;
}

export interface StudioResult {
  id:string;
  kind:string;
  title:string;
  status:string;
  summary:string;
  downloads:StudioDownload[];
  created_at:string;
}

export interface StudioAssetItem {
  ref:string;
  result_type:'persona'|'script'|'storyboard'|'image'|'video';
  project_id:string;
  project_name:string;
  task_id:string;
  task_title:string;
  title:string;
  summary:string;
  version:string;
  status:string;
  reusable:boolean;
  blocked_reason?:string;
  downloads:StudioDownload[];
  created_at:string;
}

export interface StudioAssetCatalog {
  items:StudioAssetItem[];
  counts:Record<string,number>;
  generated_at:string;
}

export interface StudioAssetMedia {
  asset_ref:string;
  shot_id?:string;
  role?:string;
  file:StudioDownload;
}

export interface StudioCreativeResultDetail {
  item:StudioAssetItem;
  content_format:StudioAssetItem['result_type'];
  content:unknown;
  media:StudioAssetMedia[];
  read_only:true;
}

export interface WorkspaceFolderItem {
  folder_ref:string;
  parent_ref?:string;
  project_id:string;
  project_name:string;
  name:string;
  child_count:number;
  created_at:string;
  updated_at:string;
}

export type WorkspaceMaterialKind='document'|'image'|'video'|'audio'|'table'|'other';
export type WorkspaceMaterialOrigin='uploaded'|'imported'|'linked';
export type WorkspaceMaterialUsage='project_material'|'project_reference';
export type WorkspaceMaterialProcessingState='uploading'|'processing'|'ready'|'failed';

export interface WorkspaceMaterialItem {
  material_ref:string;
  folder_ref?:string;
  project_id:string;
  project_name:string;
  material_kind:WorkspaceMaterialKind;
  origin:WorkspaceMaterialOrigin;
  usage:WorkspaceMaterialUsage;
  title:string;
  file_name:string;
  mime_type:string;
  byte_size:number;
  preview_ref?:string;
  processing_state:WorkspaceMaterialProcessingState;
  rights_summary:string;
  created_at:string;
  updated_at:string;
  last_used_at?:string;
}

export interface WorkspaceMaterialProjection {
  folders:WorkspaceFolderItem[];
  materials:WorkspaceMaterialItem[];
  counts:Record<string,number>;
  generated_at:string;
}

export interface StudioAssetSurface {
  workspace:WorkspaceMaterialProjection;
  creative_results:StudioAssetCatalog;
  recent:{materials:WorkspaceMaterialItem[];results:StudioAssetItem[]};
  generated_at:string;
}

export interface StudioTaskView {
  task:StudioTaskSummary;
  workbench:StudioWorkbench;
  steps:StudioCustomerStep[];
  inspirations:StudioInspiration[];
  pending_decisions:StudioDecision[];
  results:StudioResult[];
  attached_assets:StudioAssetItem[];
  pipeline:StudioPipelineSummary;
  allowed_actions:string[];
  generated_at:string;
}

export interface StudioPipelineSummary {
  stage_count:number;
  completed_stage_count:number;
  execution_count:number;
  pending_decision_count:number;
  approved_version_count:number;
  artifact_count:number;
  delivery_package_count:number;
  performance_observation_count:number;
  learning_decision_count:number;
}

export interface StudioDeliveryPackage {
  id:string;
  project_name:string;
  status:string;
  files:StudioDownload[];
  created_at:string;
}

export interface StudioPublication {
  id:string;
  project_name:string;
  destination:string;
  account_ref:string;
  status:string;
  external_url?:string;
  published_at?:string;
  updated_at:string;
}

export interface StudioDeliveries {
  packages:StudioDeliveryPackage[];
  publications:StudioPublication[];
  generated_at:string;
}

export interface StudioCreateTaskInput {
  experience_id:string;
  project_id:string;
  title:string;
  goal:string;
  inspiration:string;
  asset_refs:string[];
  material_refs:string[];
  business_brief?:BusinessBriefContract;
  idempotency_key?:string;
}
import type { BusinessBriefContract } from '../workbench/contract/businessBrief';
