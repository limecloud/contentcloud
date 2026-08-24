import { describe, expect, it } from 'vitest';
import { getWorkbenchDefinition } from './registry';

const experience={
  content_type:'article',
  step_titles:['选题','资料与引用','文章草稿','校对与确认','交付'],
  workbench:{plugin_id:'contentcloud-workbench-article',version:'1.0.0',digest:'sha256:test',layout:'article-editor',density:'comfortable',theme:'editorial-green',navigation:[],stages:[]},
};

describe('workbench registry',()=>{
  it('keeps business-specific article composition',()=>{
    const definition=getWorkbenchDefinition(experience);
    expect(definition.key).toBe('article');
    expect(definition.panels.map(panel=>panel.title)).toEqual(['选题与资料','文章写作','校对与交付','资料与引用']);
    expect(definition.panels[1].stepTitles).toEqual(['文章草稿']);
    expect(definition.navigation.map(item=>item.label)).toEqual(['文章首页','文章任务','资料与引用','文章交付']);
  });

  it('keeps serialized novel Canon and chapter composition',()=>{
    const definition=getWorkbenchDefinition({content_type:'serialized_novel',step_titles:[],workbench:{plugin_id:'contentcloud-workbench-serialized-novel',version:'1.0.0',digest:'sha256:test',layout:'novel-editor',density:'comfortable',theme:'review-rose',navigation:[],stages:[]}});
    expect(definition.key).toBe('serialized-novel');
    expect(definition.panels.map(panel=>panel.title)).toEqual(['Canon 与卷章','章节写作','连续性与编辑','章节交付']);
    expect(definition.stepTitles).toEqual(['世界观 Canon','卷章规划','章节候选','连续性校验','编辑与确认','章节交付']);
  });

  it('uses the server navigation contract while keeping routes platform-owned',()=>{
    const definition=getWorkbenchDefinition({
      content_type:'commerce',
      step_titles:[],
      workbench:{plugin_id:'custom',version:'1.0.0',digest:'sha256:test',layout:'product-variants',density:'dense',theme:'production-coral',navigation:[{id:'catalog',label:'渠道素材',icon:'tags'}],stages:[]},
    });
    expect(definition.navigation).toEqual([{id:'catalog',label:'渠道素材',icon:'tags',href:'/studio/assets'}]);
  });

  it('uses server stage labels when a business template supplies a custom stage list',()=>{
    const definition=getWorkbenchDefinition({content_type:'article',step_titles:[],workbench:{plugin_id:'custom',version:'1.0.0',digest:'sha256:test',layout:'article-editor',density:'comfortable',theme:'editorial-green',navigation:[],stages:[{id:'one',label:'事实整理',outcome:'固定资料',primary_action:'save_sources'},{id:'two',label:'成稿',outcome:'完成文章',primary_action:'save_draft'}]}});
    expect(definition.stepTitles).toEqual(['事实整理','成稿']);
    expect(definition.panels[0].stepTitles).toEqual(['事实整理','成稿']);
  });

  it('uses custom server panels and stage language for an approved business surface',()=>{
    const definition=getWorkbenchDefinition({content_type:'article',step_titles:['平台步骤'],workbench:{plugin_id:'customer-article',version:'1.0.0',digest:'sha256:test',layout:'article-editor',density:'comfortable',theme:'editorial-green',navigation:[{id:'overview',label:'客户首页',icon:'file-text'}],stages:[{id:'custom-brief',label:'客户简报',outcome:'固定客户输入',primary_action:'save_brief'},{id:'custom-output',label:'客户交付',outcome:'完成客户交付',primary_action:'create_delivery'}],panels:[{id:'custom-panel',title:'客户流程',detail:'客户自己的工作语言',tone:'source',icon:'folder',stage_ids:['custom-brief','custom-output'],target:'start',action_label:'开始客户流程'}]}});
    expect(definition.stepTitles).toEqual(['客户简报','客户交付']);
    expect(definition.panels[0].stepTitles).toEqual(['客户简报','客户交付']);
    expect(definition.panels[0].title).toBe('客户流程');
  });

  it('falls back without exposing runtime concepts',()=>{
    const definition=getWorkbenchDefinition({content_type:'unknown',step_titles:[],workbench:{plugin_id:'',version:'',digest:'',layout:'',density:'',theme:'',navigation:[],stages:[]}});
    expect(definition.key).toBe('general');
    expect(JSON.stringify(definition)).not.toContain('NodeRun');
  });

  it('falls back to a platform icon when a stale BFF sends an unknown icon',()=>{
    const definition=getWorkbenchDefinition({content_type:'article',step_titles:[],workbench:{plugin_id:'custom',version:'1.0.0',digest:'sha256:test',layout:'article-editor',density:'comfortable',theme:'editorial-green',navigation:[{id:'overview',label:'客户首页',icon:'remote-svg'}],stages:[],panels:[{id:'panel',title:'客户流程',detail:'客户自己的工作语言',tone:'source',icon:'remote-svg',stage_ids:[],target:'start',action_label:'开始'}]}});
    expect(definition.navigation[0].icon).toBe('archive');
    expect(definition.panels[0].icon).toBe('archive');
  });
});
