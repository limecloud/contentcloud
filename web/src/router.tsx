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
          {path: 'dashboard', lazy: async()=>({Component:(await import('./admin/views/AdminWorkOSPage')).AdminWorkOSRoutePage})},
          // Platform directory pages belonged to the retired operations console. Keep
          // their URLs as safe deep links, but do not expose sample tenant/user data.
          {path: 'tenants', element: <Navigate to="/admin/dashboard" replace />},
          {path: 'users', element: <Navigate to="/admin/dashboard" replace />},
          {path: 'environments', lazy: async()=>({Component:(await import('./admin/views/AdminWorkOSPage')).AdminWorkOSRoutePage})},
          {path: 'sops', lazy: async()=>({Component:(await import('./admin/views/AdminWorkOSPage')).AdminWorkOSRoutePage})},
          {path: 'gates', lazy: async()=>({Component:(await import('./admin/views/AdminWorkOSPage')).AdminWorkOSRoutePage})},
          {path: 'capabilities', lazy: async()=>({Component:(await import('./admin/views/AdminWorkOSPage')).AdminWorkOSRoutePage})},
          {path: 'audit', lazy: async()=>({Component:(await import('./admin/views/AdminWorkOSPage')).AdminWorkOSRoutePage})},
          {path: 'usage', lazy: async()=>({Component:(await import('./admin/views/AdminWorkOSPage')).AdminWorkOSRoutePage})},
          {path: '*', element: <Navigate to="dashboard" replace />}
        ]
      }
    ]
  },
  protectedConsoleRoute('/workspace', [
    {index: true, lazy: async()=>({Component:(await import('./workspace/pages')).ConsoleDashboardPage})},
    {path: 'tasks/new', lazy: async()=>({Component:(await import('./workspace/pages')).WorkOSNewTaskPage})},
    {path: 'tasks/:taskID', lazy: async()=>({Component:(await import('./workspace/pages')).WorkOSTaskDetailPage})},
    {path: 'tasks', lazy: async()=>({Component:(await import('./workspace/pages')).WorkOSTaskListPage})},
    {path: 'my-tasks', lazy: async()=>({Component:(await import('./workspace/pages')).WorkOSTaskListPage})},
    {path: 'inbox', lazy: async()=>({Component:(await import('./workspace/pages')).WorkOSInboxPage})},
    {path: 'knowledge', lazy: async()=>({Component:(await import('./workspace/pages')).WorkspaceKnowledgeRedirectPage})},
    {path: '*', element: <Navigate to="/workspace" replace />}
  ]),
  protectedConsoleRoute('/team', [
    {index: true, lazy: async()=>({Component:(await import('./workspace/pages')).ConsoleTeamPage})},
    {path: '*', element: <Navigate to="/team" replace />}
  ]),
  protectedConsoleRoute('/projects/:projectID', [
    {index: true, element: <Navigate to="setup" replace />},
    {path: 'tasks/new', lazy: async()=>({Component:(await import('./workspace/pages')).WorkOSNewTaskPage})},
    {path: 'tasks', lazy: async()=>({Component:(await import('./workspace/pages')).WorkOSTaskListPage})},
    {path: 'tasks/:taskID', lazy: async()=>({Component:(await import('./workspace/pages')).WorkOSTaskDetailPage})},
    {path: 'sop', lazy: async()=>({Component:(await import('./workspace/pages')).WorkOSSOPPage})},
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
  return projectViewIDs.map(view=>{
    if(view==='knowledge'){
      return {path:projectRoute(view),lazy:async()=>({Component:(await import('./workspace/workOS')).WorkOSKnowledgePage})};
    }
    return {
      path:projectRoute(view),
      lazy:async()=>{
        const {ConsoleProjectPage}=await import('./workspace/pages');
        return {Component:()=> <ConsoleProjectPage view={view}/>};
      }
    };
  });
}
