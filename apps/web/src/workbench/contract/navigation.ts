import type { WorkbenchIconName, WorkbenchNavigationItem } from '../registry/registry';

export const routeByNavigationID:Record<string,string>={
  overview:'/studio',
  tasks:'/studio/tasks',
  assets:'/studio/assets',
  library:'/studio/assets',
  catalog:'/studio/assets',
  deliveries:'/studio/deliveries',
};

export function navigation(id:string,label:string,icon:WorkbenchIconName):WorkbenchNavigationItem {
  return {id,label,icon,href:routeByNavigationID[id]||'/studio'};
}
