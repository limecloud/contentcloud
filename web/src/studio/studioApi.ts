import { api, post } from '../api';
import type {
  StudioAssetCatalog,
  StudioBootstrap,
  StudioCreateTaskInput,
  StudioDeliveries,
  StudioTaskSummary,
  StudioTaskView,
} from './studioTypes';

const studioPath=(suffix:string)=>`/api/studio${suffix}`;

export const studioApi={
  bootstrap:()=>api<StudioBootstrap>(studioPath('/bootstrap')),
  tasks:()=>api<StudioTaskSummary[]>(studioPath('/tasks')),
  task:(taskID:string)=>api<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}`)),
  createTask:(input:StudioCreateTaskInput)=>post<StudioTaskView>(studioPath('/tasks'),input),
  taskAction:(taskID:string,action:string)=>post<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}/actions`),{action}),
  addInspiration:(taskID:string,input:{title:string;body:string;save_for_reuse:boolean;idempotency_key?:string})=>post<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}/inspirations`),input),
  decide:(taskID:string,decisionID:string,decision:'approved'|'changes_requested')=>post<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}/decisions/${encodeURIComponent(decisionID)}`),{decision,reason:decision==='approved'?'客户确认当前结果':'客户要求修改当前结果'}),
  attachAssets:(taskID:string,assetRefs:string[])=>post<StudioTaskView>(studioPath(`/tasks/${encodeURIComponent(taskID)}/assets`),{asset_refs:assetRefs}),
  assets:(projectID?:string)=>api<StudioAssetCatalog>(studioPath(`/assets${projectID?`?project_id=${encodeURIComponent(projectID)}`:''}`)),
  deliveries:()=>api<StudioDeliveries>(studioPath('/deliveries')),
  switchTenant:(tenantID:string)=>post(studioPath('/session/switch'),{tenant_id:tenantID}),
  logout:()=>post(studioPath('/session/logout')),
};
