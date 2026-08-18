import type { StudioAssetItem, WorkspaceMaterialItem } from '../studioTypes';

export const completedTaskStatuses=['delivered','cancelled','canceled'];
export const resultTypeLabels:Record<StudioAssetItem['result_type'],string>={persona:'人物原型',script:'剧本',storyboard:'分镜',image:'图片',video:'视频'};
export const materialTypeLabels:Record<WorkspaceMaterialItem['material_kind'],string>={document:'文档',image:'图片',video:'视频',audio:'音频',table:'表格',other:'其他文件'};

export function materialHref(item:WorkspaceMaterialItem){return `/api/studio/materials/${encodeURIComponent(item.material_ref.replace(/^material:/,''))}/download`}
export function materialOriginLabel(value:WorkspaceMaterialItem['origin']){return {uploaded:'上传',imported:'导入',linked:'外部链接'}[value]}
export function materialStateLabel(value:WorkspaceMaterialItem['processing_state']){return {uploading:'上传中',processing:'处理中',ready:'可预览',failed:'处理失败'}[value]}
export function resultStateLabel(item:StudioAssetItem){return {draft:'草稿',pending_confirmation:'待确认',changes_requested:'需修改',confirmed:'已确认',delivered:'已交付',superseded:'已被替代',blocked:'已阻止'}[item.status]||item.status}
export function formatAssetDate(value:string){const date=new Date(value);return Number.isNaN(date.getTime())?'未知时间':new Intl.DateTimeFormat('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(date)}
export function formatAssetBytes(value:number){if(value<1024)return`${value} B`;if(value<1024*1024)return`${(value/1024).toFixed(1)} KB`;return`${(value/(1024*1024)).toFixed(1)} MB`}

export function parseCSV(value:string):string[][]{
  const rows:string[][]=[];let row:string[]=[];let field='';let quoted=false;
  for(let index=0;index<value.length;index+=1){
    const char=value[index];
    if(quoted){if(char==='"'&&value[index+1]==='"'){field+='"';index+=1}else if(char==='"')quoted=false;else field+=char;continue}
    if(char==='"'){quoted=true;continue}
    if(char===','){row.push(field);field='';continue}
    if(char==='\n'){row.push(field);rows.push(row);row=[];field='';continue}
    if(char!=='\r')field+=char;
  }
  if(field||row.length){row.push(field);rows.push(row)}
  return rows;
}
