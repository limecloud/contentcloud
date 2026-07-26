export type AdminRoute = 'dashboard'|'tenants'|'users';

const adminPaths: Record<AdminRoute,string> = {
  dashboard: '/admin/dashboard',
  tenants: '/admin/tenants',
  users: '/admin/users'
};

export function adminPath(route: AdminRoute): string {
  return adminPaths[route];
}
