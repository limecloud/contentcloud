import { Navigate, type RouteObject } from 'react-router-dom';
import { projectRoute, projectViewIDs } from './v3/page-contracts';

export const appRoutes: RouteObject[] = [
  {path: '/', lazy: async()=>({Component:(await import('./marketing/MarketingHome')).MarketingHome})},
  {
    path: '/docs',
    lazy: async()=>({Component:(await import('./docs/DocsRoutes')).DocsRoute}),
    children: [
      {index:true,lazy:async()=>({Component:(await import('./docs/DocsRoutes')).DocsHomeRoute})},
      {path:'clients/:clientID',lazy:async()=>({Component:(await import('./docs/DocsRoutes')).DocsClientRoute})},
      {path:'content/:contentKind',lazy:async()=>({Component:(await import('./docs/DocsRoutes')).DocsContentRoute})},
      {path:'guides/:contentKind/:clientID',lazy:async()=>({Component:(await import('./docs/DocsRoutes')).DocsGuideRoute})},
      {path:'pages/*',lazy:async()=>({Component:(await import('./docs/DocsRoutes')).DocsGenericPageRoute})},
      {path:'*',element:<Navigate to="/docs" replace/>}
    ]
  },
  {path: '/login', lazy: async()=>({Component:(await import('./views/auth/AuthRoutes')).LoginRoute})},
  {path: '/register', lazy: async()=>({Component:(await import('./views/auth/AuthRoutes')).RegisterRoute})},
  {path: '/device-auth', lazy: async()=>({Component:(await import('./views/PublicRoutes')).DeviceAuthRoute})},
  {path: '/review/:token', lazy: async()=>({Component:(await import('./views/PublicRoutes')).PublicReviewRoute})},
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
  protectedConsoleRoute('/workspace', [
    {index: true, lazy: async()=>({Component:(await import('./workspace/pages')).ConsoleDashboardPage})},
    {path: '*', element: <Navigate to="/workspace" replace />}
  ]),
  protectedConsoleRoute('/team', [
    {index: true, lazy: async()=>({Component:(await import('./workspace/pages')).ConsoleTeamPage})},
    {path: '*', element: <Navigate to="/team" replace />}
  ]),
  protectedConsoleRoute('/projects/:projectID', [
    {index: true, element: <Navigate to="setup" replace />},
    ...projectRoutes(),
    {path: '*', element: <Navigate to="setup" replace />}
  ]),
  {path: '*', element: <Navigate to="/" replace />}
];

function protectedConsoleRoute(path:string, children:RouteObject[]):RouteObject {
  return {
    path,
    lazy:async()=>({Component:(await import('./App')).App}),
    children:[{
      lazy:async()=>({Component:(await import('./workspace/WorkspaceShell')).ConsoleShell}),
      children
    }]
  };
}

function projectRoutes():RouteObject[] {
  return projectViewIDs.map(view=>({
    path:projectRoute(view),
    lazy:async()=>{
      const {ConsoleProjectPage}=await import('./workspace/pages');
      return {Component:()=> <ConsoleProjectPage view={view}/>};
    }
  }));
}
