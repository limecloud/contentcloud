export type AdminRoute = 'dashboard'|'products'|'releases'|'customers'|'capabilities'|'skills'|'executors'|'jobs'|'alerts'|'tenants'|'audit'|'costs';

const adminPaths: Record<AdminRoute,string> = {
  dashboard: '/admin/dashboard',
  products: '/admin/products',
  releases: '/admin/releases',
  customers: '/admin/customers',
  capabilities: '/admin/capabilities',
  skills: '/admin/skills',
  executors: '/admin/executors',
  jobs: '/admin/jobs',
  alerts: '/admin/alerts',
  tenants: '/admin/tenants',
  audit: '/admin/audit',
  costs: '/admin/costs'
};

export function adminPath(route: AdminRoute): string {
  return adminPaths[route];
}

export function adminProductPath(productID:string):string {
  return `${adminPaths.products}/${encodeURIComponent(productID)}`;
}

export function adminProductVersionPath(productID:string,version:number):string {
  return `${adminProductPath(productID)}/versions/${version}`;
}

export function adminCustomerPath(environmentID:string):string {
  return `${adminPaths.customers}/${encodeURIComponent(environmentID)}`;
}

export function adminCustomersForProductPath(productID:string):string {
  return `${adminPaths.customers}?product=${encodeURIComponent(productID)}`;
}

export function adminReleaseResultPath(productID:string,version:number):string {
  return `${adminPaths.releases}/${encodeURIComponent(productID)}/versions/${version}`;
}

export function adminCapabilityPath(capabilityID:string,version:string):string {
  return `${adminPaths.capabilities}/${encodeURIComponent(capabilityID)}/versions/${encodeURIComponent(version)}`;
}

export function adminExecutorPath(executorID:string):string {
  return `${adminPaths.executors}/${encodeURIComponent(executorID)}`;
}

export function adminJobPath(jobID:string):string {
  return `${adminPaths.jobs}/${encodeURIComponent(jobID)}`;
}

export function adminSkillPath(skillID:string,version:string):string {
  return `${adminPaths.skills}/${encodeURIComponent(skillID)}/versions/${encodeURIComponent(version)}`;
}
