import { Navigate, useLocation, type RouteObject } from 'react-router-dom';
import { canonicalConsolePath } from './consoleRoutes';

export const appRoutes: RouteObject[] = [
  {path: '/login', lazy: async()=>({Component:(await import('./views/auth/AuthRoutes')).LoginRoute})},
  {path: '/register', lazy: async()=>({Component:(await import('./views/auth/AuthRoutes')).RegisterRoute})},
  {path: '/device-auth', lazy: async()=>({Component:(await import('./views/PublicRoutes')).DeviceAuthRoute})},
  {path: '/review/:token', lazy: async()=>({Component:(await import('./views/PublicRoutes')).PublicReviewRoute})},
  {path: '/workspace/*', Component: LegacyWorkspaceRedirect},
  {
    path: '/admin',
    lazy: async()=>({Component:(await import('./admin/AdminRoute')).AdminRoute}),
    children: [
      {
        lazy: async()=>({Component:(await import('./admin/AdminShell')).AdminShell}),
        children: [
          {index: true, element: <Navigate to="dashboard" replace />},
          {path: 'dashboard', lazy: async()=>({Component:(await import('./admin/views/AdminDashboardPage')).AdminDashboardPage})},
          {path: 'tenants', lazy: async()=>({Component:(await import('./admin/views/AdminTenantsPage')).AdminTenantsPage})},
          {path: 'users', lazy: async()=>({Component:(await import('./admin/views/AdminUsersPage')).AdminUsersPage})},
          {path: '*', element: <Navigate to="dashboard" replace />}
        ]
      }
    ]
  },
  {
    path: '/',
    lazy: async()=>({Component:(await import('./App')).App}),
    children: [
      {
        lazy: async()=>({Component:(await import('./workspace/WorkspaceShell')).ConsoleShell}),
        children: [
          {index: true, lazy: async()=>({Component:(await import('./workspace/pages')).ConsoleDashboardPage})},
          {path: 'team', lazy: async()=>({Component:(await import('./workspace/pages')).ConsoleTeamPage})},
          {
            path: 'projects/:projectID',
            children: [
              {index: true, element: <Navigate to="overview" replace />},
              ...projectRoutes(),
              {path: '*', element: <Navigate to="overview" replace />}
            ]
          },
          {path: '*', element: <Navigate to="/" replace />}
        ]
      }
    ]
  }
];

function LegacyWorkspaceRedirect(){
  const location=useLocation();
  return <Navigate to={`${canonicalConsolePath(location.pathname)}${location.search}${location.hash}`} replace/>;
}

function projectRoutes():RouteObject[] {
  const views=['overview','sources','assets','knowledge','strategy','briefs','scripts','submissions','results','lineage','audit'] as const;
  return views.map(view=>({
    path:view,
    lazy:async()=>{
      const {ConsoleProjectPage}=await import('./workspace/pages');
      return {Component:()=> <ConsoleProjectPage view={view}/>};
    }
  }));
}
