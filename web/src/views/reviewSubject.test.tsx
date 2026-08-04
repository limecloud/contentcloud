import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { ArticleBlock, ReviewComment, ReviewProjection } from '../types';
import { ArticleReviewContent } from './ReviewContent';
import { commentsForArticleBlock, reviewSubject } from './reviewSubject';

const baseProjection:ReviewProjection={
  project:{id:'project-1',brand_name:'Brand',product_name:'Product',content_type:'wechat_article',channel:'wechat_official_account',stage_objective:'',status:'active',owner_name:'',reviewer_name:'',client_approver:'',row_version:1,connected_devices:1,knowledge_ready:1,open_blockers:0,updated_at:'2026-07-29T00:00:00Z'},
  comments:[],verified:true,
};

describe('public review subject routing',()=>{
  it('keeps the existing video review route',()=>{
    const subject=reviewSubject({...baseProjection,submission:{submission_id:'submission-1',submission_revision_id:'revision-1',subject_hash:'sha256:video',schema_version:'contentcloud.content_batch/3.0',base_snapshot_ids:[],environment_digest:'sha256:env',object_refs:[],objects:[{type:'content_item',schema_version:'contentcloud.content-item/3.0',status:'candidate',title:'视频标题',duration_ms:15000,direction:{title:'目标'},experiment:{hypothesis:''},shots:[]}]}});
    expect(subject).toMatchObject({kind:'video_script',title:'视频标题',versionLabel:'视频剧本 3.0 · 15s'});
  });

  it('selects and renders a governed ArticleItem without interpreting embedded HTML',()=>{
    const projection:ReviewProjection={...baseProjection,submission:{submission_id:'submission-2',submission_revision_id:'revision-2',subject_hash:'sha256:article',schema_version:'contentcloud.content_batch/3.0',base_snapshot_ids:[],environment_digest:'sha256:env',object_refs:[],objects:[articleObject()]}};
    const subject=reviewSubject(projection);
    expect(subject).toMatchObject({kind:'wechat_article',title:'选中的标题',authorDisplayName:'编辑部'});
    if(!subject||subject.kind!=='wechat_article')throw new Error('expected article subject');
    const markup=renderToStaticMarkup(<ArticleReviewContent subject={subject} comments={[]}/>);
    expect(markup).toContain('&lt;script&gt;window.pwned=true&lt;/script&gt;');
    expect(markup).not.toContain('<script>');
    expect(markup).toContain('商业主张');
    expect(markup).toContain('claim-1');
  });

  it('routes block comments by index or id inside a JSON pointer',()=>{
    const block=articleObject().blocks[0] as ArticleBlock;
    const comments=[comment('index',{json_pointer:'/0/blocks/0/text'}),comment('id',{json_pointer:'/blocks/block-1/assertions/0'}),comment('other',{json_pointer:'/blocks/1/text'})];
    expect(commentsForArticleBlock(comments,block,0).map(value=>value.id)).toEqual(['index','id']);
  });

  it('rejects unknown content schemas',()=>{
    const subject=reviewSubject({...baseProjection,submission:{submission_id:'submission-3',submission_revision_id:'revision-3',subject_hash:'sha256:unknown',schema_version:'contentcloud.content_batch/3.0',base_snapshot_ids:[],environment_digest:'sha256:env',object_refs:[],objects:[{type:'article_item',schema_version:'contentcloud.article/2.0'}]}});
    expect(subject).toBeUndefined();
  });
});

function articleObject() {
  return {
    type:'article_item',schema_version:'contentcloud.article/1.0',status:'candidate',summary:'文章摘要',author_display_name:'编辑部',selected_title_id:'title-2',
    title_candidates:[{id:'title-1',text:'候选标题',strategy:'',risk_refs:[]},{id:'title-2',text:'选中的标题',strategy:'',risk_refs:[]}],
    blocks:[{id:'block-1',type:'paragraph',text:'<script>window.pwned=true</script>',level:0,ordered:false,items:[],asset_ref:'',rights_ref:'',alt_text:'',caption:'',purpose:'',callout_kind:'',target:'',assertions:[{id:'assertion-1',type:'commercial_claim',knowledge_refs:['claim-1'],evidence_refs:[],attribution:''}],style_marks:[]}],
    attribution:{source_names:['批准知识'],disclosure:'经内部审核'},
  };
}

function comment(id:string,anchor:Pick<ReviewComment,'json_pointer'>):ReviewComment {
  return {id,review_cycle_id:'cycle-1',subject_id:'revision-1',body:id,visibility:'client',author_id:'user-1',created_at:'2026-07-29T00:00:00Z',...anchor};
}
