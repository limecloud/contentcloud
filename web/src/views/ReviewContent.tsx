import type { ArticleAssertion, ArticleBlock, ReviewComment } from '../types';
import { Status } from '../components/ui';
import { commentsForArticleBlock, type ArticleReviewSubject, type ReviewSubject, type VideoReviewSubject } from './reviewSubject';

export function ReviewContent({subject,comments}:{subject:ReviewSubject;comments:ReviewComment[]}) {
  return <>
    <section className="review-summary">
      <div><span>{subject.versionLabel}</span><h1>{subject.title}</h1><p>{subject.objective}</p></div>
      <div><Status value={subject.status}/><code>{subject.hash.slice(0,16)}</code></div>
    </section>
    {subject.kind==='video_script'?<VideoReviewContent subject={subject} comments={comments}/>:<ArticleReviewContent subject={subject} comments={comments}/>}
  </>;
}

function VideoReviewContent({subject,comments}:{subject:VideoReviewSubject;comments:ReviewComment[]}) {
  return <section className="review-shot-list">{subject.shots.map((shot,index)=><article key={shot.shot_id}>
    <header><span>{String(index+1).padStart(2,'0')} · {Math.max(1,Math.round((shot.end_ms-shot.start_ms)/1000))}s</span><Status value={shot.role}/></header>
    <h2>{shot.narrative_purpose}</h2>
    <dl><div><dt>画面</dt><dd>{shot.visual_intent}</dd></div><div><dt>动作</dt><dd>{shot.subject_action}</dd></div><div><dt>口播 / 字幕</dt><dd>{shot.voiceover||shot.on_screen_text||'无'}</dd></div><div><dt>验收</dt><dd>{shot.acceptance_criteria.join('；')||'无'}</dd></div></dl>
    <ReviewComments values={comments.filter(comment=>comment.shot_id===shot.shot_id)}/>
  </article>)}</section>;
}

export function ArticleReviewContent({subject,comments}:{subject:ArticleReviewSubject;comments:ReviewComment[]}) {
  return <section className="review-article">
    <header><span>作者</span><strong>{subject.authorDisplayName||'未署名'}</strong></header>
    <div className="review-article-body">{subject.blocks.map((block,index)=><ArticleBlockView key={block.id} block={block} comments={commentsForArticleBlock(comments,block,index)}/>)}</div>
    {(subject.attribution.source_names.length>0||subject.attribution.disclosure)&&<footer><strong>来源与披露</strong>{subject.attribution.source_names.length>0&&<p>{subject.attribution.source_names.join('、')}</p>}{subject.attribution.disclosure&&<small>{subject.attribution.disclosure}</small>}</footer>}
  </section>;
}

function ArticleBlockView({block,comments}:{block:ArticleBlock;comments:ReviewComment[]}) {
  return <article className={`review-article-block is-${block.type}`} data-block-id={block.id}>
    <ArticleBlockBody block={block}/>
    {block.assertions.length>0&&<div className="review-assertions"><strong>主张与引用</strong>{block.assertions.map(assertion=><Assertion key={assertion.id} value={assertion}/>)}</div>}
    <ReviewComments values={comments}/>
  </article>;
}

function ArticleBlockBody({block}:{block:ArticleBlock}) {
  switch(block.type) {
    case 'heading': return block.level>=4?<h4>{block.text}</h4>:block.level===3?<h3>{block.text}</h3>:<h2>{block.text}</h2>;
    case 'paragraph': return <p>{block.text}</p>;
    case 'list': return block.ordered?<ol>{block.items.map((item,index)=><li key={index}>{item}</li>)}</ol>:<ul>{block.items.map((item,index)=><li key={index}>{item}</li>)}</ul>;
    case 'quote': return <blockquote className="review-article-quote">{block.text}{block.caption&&<cite>{block.caption}</cite>}</blockquote>;
    case 'image': return <figure><div className="review-article-image"><span>{block.alt_text||'文章配图'}</span><code>{block.asset_ref||'待上传素材'}</code></div>{block.caption&&<figcaption>{block.caption}</figcaption>}</figure>;
    case 'callout': return <aside><strong>{block.callout_kind||'提示'}</strong><p>{block.text}</p></aside>;
    case 'divider': return <hr/>;
    case 'cta': return <section className="review-article-cta"><strong>{block.text}</strong>{block.target&&<code>{block.target}</code>}</section>;
  }
}

function Assertion({value}:{value:ArticleAssertion}) {
  const references=[...value.knowledge_refs,...value.evidence_refs];
  return <div><span>{assertionLabel(value.type)}</span>{references.length>0?<code>{references.join(' · ')}</code>:<small>编辑性表达，无外部引用</small>}{value.attribution&&<small>{value.attribution}</small>}</div>;
}

function assertionLabel(value:ArticleAssertion['type']) {
  return ({fact:'事实',commercial_claim:'商业主张',quotation:'引语',editorial_opinion:'编辑观点',personal_experience:'个人经历',hypothesis:'假设'})[value];
}

function ReviewComments({values}:{values:ReviewComment[]}) {
  return <>{values.map(comment=><blockquote className="review-comment" key={comment.id}>{comment.body}{comment.json_pointer&&<code>{comment.json_pointer}</code>}</blockquote>)}</>;
}
