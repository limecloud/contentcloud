import { Navigate, type RouteObject } from 'react-router-dom';

export const appRoutes: RouteObject[] = [
  {path: '/', element: <Navigate to="/workspace/dashboard" replace />},
  {path: '/login', lazy: async()=>({Component:(await import('./views/auth/AuthRoutes')).LoginRoute})},
  {path: '/register', lazy: async()=>({Component:(await import('./views/auth/AuthRoutes')).RegisterRoute})},
  {path: '/device-auth', lazy: async()=>({Component:(await import('./views/PublicRoutes')).DeviceAuthRoute})},
  {path: '/review/:token', lazy: async()=>({Component:(await import('./views/PublicRoutes')).PublicReviewRoute})},
  {
    path: '/workspace',
    lazy: async()=>({Component:(await import('./App')).App}),
    children: [
      {
        lazy: async()=>({Component:(await import('./workspace/WorkspaceShell')).WorkspaceShell}),
        children: [
          {index: true, element: <Navigate to="dashboard" replace />},
          {path: 'dashboard', lazy: async()=>({Component:(await import('./workspace/pages')).WorkspaceDashboardPage})},
          {path: 'team', lazy: async()=>({Component:(await import('./workspace/pages')).WorkspaceTeamPage})},
          {
            path: 'projects/:projectID',
            children: [
              {index: true, element: <Navigate to="overview" replace />},
              ...projectRoutes(),
              {path: '*', element: <Navigate to="overview" replace />}
            ]
          },
          {path: '*', element: <Navigate to="dashboard" replace />}
        ]
      }
    ]
  },
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
  {path: '*', element: <Navigate to="/workspace/dashboard" replace />}
];

function projectRoutes():RouteObject[] {
  const views=['overview','sources','assets','knowledge','strategy','briefs','scripts','submissions','results','lineage','audit'] as const;
  return views.map(view=>({
    path:view,
    lazy:async()=>{
      const {WorkspaceProjectPage}=await import('./workspace/pages');
      return {Component:()=> <WorkspaceProjectPage view={view}/>};
    }
  }));
}
