import { afterEach, describe, expect, it, vi } from 'vitest';
import type { AdminWorkOSView } from '../types';
import { loadAdminSnapshot } from './context';

const workOS:AdminWorkOSView={
  environments:[],
  sops:[{definition:{id:'product-1',tenant_id:'platform',name:'真实创作产品',description:'',content_types:['marketing_video'],current_version:1,created_by:'operator',created_at:'2026-08-08T01:00:00Z',updated_at:'2026-08-08T01:00:00Z'},versions:[]}],
  gates:[],
  capabilities:[],
  audit:[],
  usage:{task_count:1,running_count:1,waiting_gate_count:0,by_execution_mode:{managed:1}},
  generated_at:'2026-08-08T02:30:00Z'
};

function jsonResponse(data:unknown,status=200) {
  return new Response(JSON.stringify({ok:status>=200&&status<300,data}),{status,headers:{'Content-Type':'application/json'}});
}

describe('admin data loading',()=>{
  afterEach(()=>vi.unstubAllGlobals());

  it('keeps work-os available when both optional operations directories return 404',async()=>{
    vi.stubGlobal('fetch',vi.fn((path:string)=>{
      if(path==='/api/bff/admin/work-os')return Promise.resolve(jsonResponse(workOS));
      return Promise.resolve(new Response('404 page not found',{status:404,headers:{'Content-Type':'text/plain; charset=utf-8'}}));
    }));

    const snapshot=await loadAdminSnapshot(true);

    expect(snapshot.workOS.sops[0]?.definition.name).toBe('真实创作产品');
    expect(snapshot.data.counts.active_runs).toBe(1);
    expect(snapshot.executorDirectory.executors).toEqual([]);
    expect(snapshot.skillDirectory).toMatchObject({configured:false,skills:[]});
    expect(snapshot.executorDirectoryError).toContain('/api/bff/operations/executors');
    expect(snapshot.executorDirectoryError).toContain('HTTP 404');
    expect(snapshot.skillDirectoryError).toContain('/api/bff/operations/skills');
  });

  it('still rejects when the critical work-os request fails',async()=>{
    vi.stubGlobal('fetch',vi.fn((path:string)=>Promise.resolve(path==='/api/bff/admin/work-os'
      ?new Response('service unavailable',{status:503,headers:{'Content-Type':'text/plain'}})
      :jsonResponse({executors:[],skills:[]}))));

    await expect(loadAdminSnapshot(true)).rejects.toThrow('/api/bff/admin/work-os');
  });
});
