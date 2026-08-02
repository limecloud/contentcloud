import {describe,expect,it} from 'vitest';
import {appRoutes} from '../router';

function nestedChildren(routePath:string) {
  const route=appRoutes.find(item=>item.path===routePath);
  const shell=route?.children?.[0];
  return shell?.children||[];
}

async function loadComponent(route:ReturnType<typeof nestedChildren>[number]|undefined) {
  if(!route||typeof route.lazy!=='function') throw new Error('目标路由没有懒加载组件');
  return route.lazy();
}

describe('knowledge navigation',()=>{
  it('redirects the legacy workspace URL and serves the governed Project page',async()=>{
    const workspaceKnowledge=nestedChildren('/workspace').find(item=>item.path==='knowledge');
    const projectKnowledge=nestedChildren('/projects/:projectID').find(item=>item.path==='knowledge');
    const workspaceLoaded=await loadComponent(workspaceKnowledge);
    const projectLoaded=await loadComponent(projectKnowledge);
    expect(workspaceLoaded?.Component?.name).toBe('WorkspaceKnowledgeRedirectPage');
    expect(projectLoaded?.Component?.name).toBe('WorkOSKnowledgePage');
  });
});
