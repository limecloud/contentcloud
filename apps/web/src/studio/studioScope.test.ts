import { describe, expect, it } from 'vitest';
import { ALL_EXPERIENCES, scopedStudioProjects, scopedStudioTasks } from './studioScope';
import type { StudioExperience, StudioProject, StudioTaskSummary } from './studioTypes';

const project=(id:string,status='active'):StudioProject=>({id,brand_name:id,product_name:id,content_type:'article',channel:'web',status,execution_client_connected:false,connected_client_count:0});
const experience=(id:string,projectIDs:string[]):StudioExperience=>({id,version:'1',name:id,description:'',content_type:'article',status:'published',project_ids:projectIDs,step_titles:[],available_collection_methods:[],unavailable_collection_methods:[],workbench:{plugin_id:'test',version:'1',digest:'digest',layout:'article-editor',density:'comfortable',theme:'signal-blue',navigation:[],stages:[]}});
const task=(id:string,experienceID:string):StudioTaskSummary=>({id,project:project('project'),experience_id:experienceID,title:id,intent:'',content_type:'article',status:'running',status_label:'进行中',current_step_id:'step',next_action:'继续',asset_count:0,created_at:'',updated_at:''});

describe('studio scope',()=>{
  it('returns all active projects for the all-business view',()=>{
    expect(scopedStudioProjects([project('one'),project('archived','archived')],ALL_EXPERIENCES,undefined).map(item=>item.id)).toEqual(['one']);
  });

  it('keeps only projects assigned to the selected business',()=>{
    expect(scopedStudioProjects([project('one'),project('two')],'video',experience('video',['two'])).map(item=>item.id)).toEqual(['two']);
    expect(scopedStudioProjects([project('one')],'missing',undefined)).toEqual([]);
  });

  it('keeps only tasks assigned to the selected business',()=>{
    const tasks=[task('article-task','article'),task('video-task','video')];
    expect(scopedStudioTasks(tasks,'video').map(item=>item.id)).toEqual(['video-task']);
    expect(scopedStudioTasks(tasks,ALL_EXPERIENCES)).toEqual(tasks);
  });
});
