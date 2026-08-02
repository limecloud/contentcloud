export type AdminRoute = 'dashboard'|'tenants'|'users'|'environments'|'sops'|'gates'|'capabilities'|'audit'|'usage';

const adminPaths: Record<AdminRoute,string> = {
  dashboard: '/admin/dashboard',
  tenants: '/admin/tenants',
  users: '/admin/users',
  environments: '/admin/environments',
  sops: '/admin/sops',
  gates: '/admin/gates',
  capabilities: '/admin/capabilities',
  audit: '/admin/audit',
  usage: '/admin/usage'
};

export function adminPath(route: AdminRoute): string {
  return adminPaths[route];
}
