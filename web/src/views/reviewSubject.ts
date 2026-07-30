import type { ArticleBlock, ArticleItem, ContentItem, ContentShot, ReviewComment, ReviewProjection } from '../types';

type ReviewBase = {
  versionLabel:string;
  title:string;
  objective:string;
  status:string;
  hash:string;
};

export type VideoReviewSubject = ReviewBase & {
  kind:'video_script';
  shots:Array<Pick<ContentShot,'shot_id'|'start_ms'|'end_ms'|'role'|'narrative_purpose'|'visual_intent'|'subject_action'|'voiceover'|'on_screen_text'|'acceptance_criteria'>>;
};

export type ArticleReviewSubject = ReviewBase & {
  kind:'wechat_article';
  authorDisplayName:string;
  blocks:ArticleBlock[];
  attribution:{source_names:string[];disclosure:string};
};

export type ReviewSubject = VideoReviewSubject|ArticleReviewSubject;

export function reviewSubject(projection:ReviewProjection):ReviewSubject|undefined {
  if(!projection.submission)return undefined;
  for(const object of projection.submission.objects) {
    if(isVideoItem(object)) {
      return {
        kind:'video_script',
        versionLabel:`视频剧本 3.0 · ${Math.round(object.duration_ms/1000)}s`,
        title:object.title,
        objective:object.direction?.title||object.direction?.angle||object.experiment?.hypothesis||'',
        status:object.status,
        hash:projection.submission.subject_hash,
        shots:object.shots,
      };
    }
    if(isArticleItem(object)) {
      const selected=object.title_candidates.find(candidate=>candidate.id===object.selected_title_id);
      return {
        kind:'wechat_article',
        versionLabel:'微信公众号文章 1.0',
        title:selected?.text||object.title_candidates[0]?.text||'未命名文章',
        objective:object.summary,
        status:object.status,
        hash:projection.submission.subject_hash,
        authorDisplayName:object.author_display_name,
        blocks:object.blocks,
        attribution:object.attribution,
      };
    }
  }
  return undefined;
}

export function commentsForArticleBlock(comments:ReviewComment[],block:ArticleBlock,index:number):ReviewComment[] {
  return comments.filter(comment=>pointerTargetsBlock(comment.json_pointer,index,block.id));
}

function pointerTargetsBlock(pointer:string|undefined,index:number,blockID:string):boolean {
  if(!pointer?.startsWith('/'))return false;
  const tokens=pointer.slice(1).split('/').map(token=>token.replace(/~1/g,'/').replace(/~0/g,'~'));
  return tokens.some((token,position)=>token==='blocks'&&(tokens[position+1]===String(index)||tokens[position+1]===blockID));
}

function isVideoItem(value:Record<string,unknown>):value is Record<string,unknown>&ContentItem {
  return value.type==='content_item'&&value.schema_version==='contentcloud.content-item/3.0'&&typeof value.title==='string'&&typeof value.status==='string'&&typeof value.duration_ms==='number'&&Array.isArray(value.shots)&&value.shots.every(isVideoShot);
}

function isArticleItem(value:Record<string,unknown>):value is Record<string,unknown>&ArticleItem {
  return value.type==='article_item'&&value.schema_version==='contentcloud.article/1.0'&&typeof value.status==='string'&&typeof value.summary==='string'&&typeof value.author_display_name==='string'&&typeof value.selected_title_id==='string'&&Array.isArray(value.title_candidates)&&value.title_candidates.every(isArticleTitle)&&Array.isArray(value.blocks)&&value.blocks.every(isArticleBlock)&&isAttribution(value.attribution);
}

function isAttribution(value:unknown):value is ArticleItem['attribution'] {
  return isRecord(value)&&isStringArray(value.source_names)&&typeof value.disclosure==='string';
}

function isVideoShot(value:unknown):value is ContentShot {
  return isRecord(value)&&typeof value.shot_id==='string'&&typeof value.start_ms==='number'&&typeof value.end_ms==='number'&&typeof value.role==='string'&&typeof value.narrative_purpose==='string'&&typeof value.visual_intent==='string'&&typeof value.subject_action==='string'&&isStringArray(value.acceptance_criteria);
}

function isArticleTitle(value:unknown):value is ArticleItem['title_candidates'][number] {
  return isRecord(value)&&typeof value.id==='string'&&typeof value.text==='string'&&typeof value.strategy==='string'&&isStringArray(value.risk_refs);
}

function isArticleBlock(value:unknown):value is ArticleBlock {
  const blockTypes=new Set<ArticleBlock['type']>(['heading','paragraph','list','quote','image','callout','divider','cta']);
  return isRecord(value)&&typeof value.id==='string'&&typeof value.type==='string'&&blockTypes.has(value.type as ArticleBlock['type'])&&typeof value.text==='string'&&typeof value.level==='number'&&typeof value.ordered==='boolean'&&isStringArray(value.items)&&typeof value.asset_ref==='string'&&typeof value.rights_ref==='string'&&typeof value.alt_text==='string'&&typeof value.caption==='string'&&typeof value.purpose==='string'&&typeof value.callout_kind==='string'&&typeof value.target==='string'&&Array.isArray(value.assertions)&&value.assertions.every(isArticleAssertion)&&isStringArray(value.style_marks);
}

function isArticleAssertion(value:unknown):value is ArticleBlock['assertions'][number] {
  const assertionTypes=new Set(['fact','commercial_claim','quotation','editorial_opinion','personal_experience','hypothesis']);
  return isRecord(value)&&typeof value.id==='string'&&typeof value.type==='string'&&assertionTypes.has(value.type)&&isStringArray(value.knowledge_refs)&&isStringArray(value.evidence_refs)&&typeof value.attribution==='string';
}

function isStringArray(value:unknown):value is string[] {
  return Array.isArray(value)&&value.every(item=>typeof item==='string');
}

function isRecord(value:unknown):value is Record<string,unknown> {
  return typeof value==='object'&&value!==null&&!Array.isArray(value);
}
