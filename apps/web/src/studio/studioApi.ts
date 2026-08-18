import { api, post } from '../api';
import type {
  StudioAssetSurface,
  StudioBootstrap,
  StudioConnectSession,
  StudioCreateTaskInput,
  StudioCreativeResultDetail,
  StudioDeliveries,
  StudioExecutionClientCatalog,
  StudioTaskSummary,
  StudioTaskView,
  WorkspaceFolderItem,
  WorkspaceMaterialItem,
} from './studioTypes';

const studioPath=(suffix:string)=>`/api/studio${suffix}`;

export const studioApi={
  bootstrap:()=>api<StudioBootstrap>(studioPath('/bootstrap')),
  executionClients:()=>api<StudioExecutionClientCatalog>(studioPath('/execution-clients')),
  createConnectSession:(projectID:string)=>post<StudioConnectSession>(studioPath(`/projects/${encodeURIComponent(projectID)}/connect-sessions`)),
  connectSession:(sessionID:string)=>api<StudioConnectSession>(studioPath(`/connect-sessions/${encodeURIComponent(sessionID)}`)),
  approveConnectSession:(sessionID:string)=>post<StudioConnectSession>(studioPath(`/connect-sessions/${encodeURIComponent(sessionID)}/approve`)),
  denyConnectSession:(sessionID:string)=>post<StudioConnectSession>(studioPath(`/connect-sessions/${encodeURIComponent(sessionID)}/deny`)),
  cancelConnectSession:(sessionID:string)=>post<StudioConnectSession>(studioPath(`/connect-sessions/${encodeURIComponent(sessionID)}/cancel`)),
  tasks:()=>api<StudioTaskSummary[]>(studioPath('/tasks')),
  task:(taskID:string)=>api<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}`)),
  createTask:(input:StudioCreateTaskInput)=>post<StudioTaskView>(studioPath('/tasks'),input),
  taskAction:(taskID:string,action:string)=>post<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}/actions`),{action}),
  addInspiration:(taskID:string,input:{title:string;body:string;keep_as_project_reference:boolean;idempotency_key?:string})=>post<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}/inspirations`),input),
  decide:(taskID:string,decisionID:string,decision:'approved'|'changes_requested')=>post<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}/decisions/${encodeURIComponent(decisionID)}`),{decision,reason:decision==='approved'?'客户确认当前结果':'客户要求修改当前结果'}),
  attachAssets:(taskID:string,assetRefs:string[])=>post<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}/assets`),{asset_refs:assetRefs}),
  attachMaterials:(taskID:string,materialRefs:string[])=>post<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}/materials`),{material_refs:materialRefs}),
  assets:(projectID?:string)=>api<StudioAssetSurface>(studioPath(`/assets${projectID?`?project_id=${encodeURIComponent(projectID)}`:''}`)),
  creativeResult:(resultID:string,taskID:string)=>api<StudioCreativeResultDetail>(studioPath(`/assets/results/${encodeURIComponent(resultID.replace(/^result:/,''))}?task_id=${encodeURIComponent(taskID)}`)),
  createFolder:(input:{project_id:string;parent_ref?:string;name:string})=>post<WorkspaceFolderItem>(studioPath('/asset-folders'),input),
  uploadMaterial:(input:{project_id:string;folder_ref?:string;title?:string;file:File})=>{
    const body=new FormData();body.append('project_id',input.project_id);if(input.folder_ref)body.append('folder_ref',input.folder_ref);if(input.title)body.append('title',input.title);body.append('file',input.file,input.file.name);body.append('file_type',input.file.type);return fetch(studioPath('/materials'),{method:'POST',body,credentials:'include'}).then(async response=>{const envelope=await response.json();if(!response.ok||!envelope.ok)throw new Error(envelope?.error?.message||'资料上传失败');return envelope.data as WorkspaceMaterialItem})
  },
  deliveries:()=>api<StudioDeliveries>(studioPath('/deliveries')),
  switchTenant:(tenantID:string)=>post(studioPath('/session/switch'),{tenant_id:tenantID}),
  logout:()=>post(studioPath('/session/logout')),
};
