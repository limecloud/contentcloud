import type { ReactNode } from 'react';

export interface WorkbenchTaskData {
  title:string;
  intent:string;
  content_type:string;
  asset_count:number;
  project:{brand_name:string};
}

export interface WorkbenchStepData {
  title:string;
  outcome_description:string;
  status:string;
}

export interface WorkbenchResultData {
  title:string;
  status:string;
  summary:string;
}

export interface WorkbenchSurfaceContract {
  plugin_id?:string;
  version?:string;
  digest?:string;
  layout?:string;
  density?:string;
  theme?:string;
}

export interface BusinessTaskCanvasProps {
  task:WorkbenchTaskData;
  step:WorkbenchStepData;
  results:WorkbenchResultData[];
  fallback:ReactNode;
  workbench?:WorkbenchSurfaceContract;
}
