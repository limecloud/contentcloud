import { Navigate, type RouteObject } from 'react-router-dom';

const routeHydrateFallback = {hydrateFallbackElement: <div className="splash" aria-live="polite"><strong>正在加载…</strong></div>};

export const appRoutes: RouteObject[] = [
  {...routeHydrateFallback, path: '/', lazy: async()=>({Component:(await import('./marketing/MarketingHome')).MarketingHome})},
  {
    ...routeHydrateFallback,
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
  {...routeHydrateFallback, path: '/login', lazy: async()=>({Component:(await import('./views/auth/AuthRoutes')).LoginRoute})},
  {...routeHydrateFallback, path: '/register', lazy: async()=>({Component:(await import('./views/auth/AuthRoutes')).RegisterRoute})},
  {...routeHydrateFallback, path: '/device-auth', lazy: async()=>({Component:(await import('./views/PublicRoutes')).DeviceAuthRoute})},
  {...routeHydrateFallback, path: '/review/:token', lazy: async()=>({Component:(await import('./views/PublicRoutes')).PublicReviewRoute})},
  {
    ...routeHydrateFallback,
    path: '/admin',
    lazy: async()=>({Component:(await import('./admin/AdminRoute')).AdminRoute}),
    children: [
      {
        lazy: async()=>({Component:(await import('./admin/AdminShell')).AdminShell}),
        children: [
          {index: true, element: <Navigate to="dashboard" replace />},
          {path: 'dashboard', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminOperationsOverview})},
          {path: 'products', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminProductsPage})},
          {path: 'products/:productID', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminProductDetailPage})},
          {path: 'products/:productID/versions/:version', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminProductDetailPage})},
          {path: 'releases', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminProductReleasesPage})},
          {path: 'releases/:productID/versions/:version', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminReleaseResultPage})},
          {path: 'customers', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminCustomersPage})},
          {path: 'customers/:environmentID', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminCustomerDetailPage})},
          {path: 'capabilities', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminCapabilityCatalogPage})},
          {path: 'capabilities/:capabilityID/versions/:capabilityVersion', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminCapabilityDetailPage})},
          {path: 'skills', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminSkillsPage})},
          {path: 'skills/:skillID/versions/:skillVersion', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminSkillDetailPage})},
          {path: 'executors', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminExecutorsPage})},
          {path: 'executors/:executorID', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminExecutorDetailPage})},
          {path: 'jobs', lazy: async()=>({Component:(await import('./admin/views/AdminRuntimePage')).AdminRuntimePage})},
          {path: 'alerts', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminAlertsPage})},
          {path: 'tenants', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminTenantsPage})},
          {path: 'audit', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminAuditPage})},
          {path: 'costs', lazy: async()=>({Component:(await import('./admin/views/AdminOperationsPages')).AdminCostsPage})},
          {path: '*', element: <Navigate to="dashboard" replace />}
        ]
      }
    ]
  },
  protectedStudioRoute('/studio', [
    {index: true, lazy: async()=>({Component:(await import('./studio/StudioPages')).StudioHomePage})},
    {path: 'connect', lazy: async()=>({Component:(await import('./studio/StudioPages')).StudioConnectPage})},
    {path: 'tasks/new', lazy: async()=>({Component:(await import('./studio/StudioPages')).StudioNewTaskPage})},
    {path: 'tasks/:taskID', lazy: async()=>({Component:(await import('./studio/StudioPages')).StudioTaskPage})},
    {path: 'tasks', lazy: async()=>({Component:(await import('./studio/StudioPages')).StudioTasksPage})},
    {path: 'assets/materials/:materialID', lazy: async()=>({Component:(await import('./studio/assets')).StudioMaterialDetailPage})},
    {path: 'assets/results/:taskID/:resultID', lazy: async()=>({Component:(await import('./studio/assets')).StudioCreativeResultDetailPage})},
    {path: 'assets', lazy: async()=>({Component:(await import('./studio/StudioPages')).StudioAssetsPage})},
    {path: 'knowledge', lazy: async()=>({Component:(await import('./studio/StudioKnowledgePage')).StudioKnowledgePage})},
    {path: 'team', lazy: async()=>({Component:(await import('./studio/StudioTeamPage')).StudioTeamPage})},
    {path: 'deliveries', lazy: async()=>({Component:(await import('./studio/StudioPages')).StudioDeliveriesPage})},
    {path: '*', element: <Navigate to="/studio" replace />}
  ]),
  {path: '*', element: <Navigate to="/" replace />}
];

function protectedStudioRoute(path:string, children:RouteObject[]):RouteObject {
  return {
    ...routeHydrateFallback,
    path,
    lazy:async()=>({Component:(await import('./studio/StudioContext')).CustomerStudioApp}),
    children:[{
      lazy:async()=>({Component:(await import('./studio/CustomerStudioShell')).CustomerStudioShell}),
      children
    }]
  };
}
