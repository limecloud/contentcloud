import { useLocation } from 'react-router-dom';
import { AdminWorkOSPage as WorkOSAdminPage } from '../../workspace/workOS';

const pageKinds = {
  '/admin/dashboard': 'overview',
  '/admin/environments': 'environment',
  '/admin/sops': 'sop',
  '/admin/gates': 'gate',
  '/admin/capabilities': 'capability',
  '/admin/audit': 'audit',
  '/admin/usage': 'usage'
} as const;

export function AdminWorkOSRoutePage() {
  const location = useLocation();
  const kind = pageKinds[location.pathname as keyof typeof pageKinds] || 'environment';
  return <WorkOSAdminPage kind={kind}/>;
}
