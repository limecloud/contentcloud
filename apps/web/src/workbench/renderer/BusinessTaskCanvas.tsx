import type { BusinessTaskCanvasProps } from '../contract/businessTaskTypes';
import { ArticleTaskCanvas } from '../../workbenches/article/ArticleTaskCanvas';
import { CommerceTaskCanvas } from '../../workbenches/commerce/CommerceTaskCanvas';
import { VideoTaskCanvas } from '../../workbenches/marketing-video/VideoTaskCanvas';
import { NovelTaskCanvas } from '../../workbenches/serialized-novel/NovelTaskCanvas';
import './workbench.css';

export function BusinessTaskCanvas({task,step,results,fallback,workbench}:BusinessTaskCanvasProps){
  const contentType=task.content_type.trim().toLowerCase().replace(/\s+/g,'_');
  // A frozen server layout is authoritative. Unknown layouts fail closed so
  // an unapproved plugin cannot silently become another business surface.
  if(workbench?.layout){
    if(workbench.layout==='stage-canvas-context')return <VideoTaskCanvas task={task} step={step} results={results}/>;
    if(workbench.layout==='article-editor')return <ArticleTaskCanvas task={task} step={step} results={results} fallback={fallback}/>;
    if(workbench.layout==='product-variants')return <CommerceTaskCanvas task={task} step={step} results={results} fallback={fallback}/>;
    if(workbench.layout==='novel-editor')return <NovelTaskCanvas task={task} step={step} results={results}/>;
    return <>{fallback}</>;
  }
  if(['marketing_video','marketing-video','video_script'].includes(contentType))return <VideoTaskCanvas task={task} step={step} results={results}/>;
  if(['article','wechat_article','article_content'].includes(contentType))return <ArticleTaskCanvas task={task} step={step} results={results} fallback={fallback}/>;
  if(['commerce','ecommerce','douyin_commerce','product_content'].includes(contentType))return <CommerceTaskCanvas task={task} step={step} results={results} fallback={fallback}/>;
  if(contentType==='serialized_novel')return <NovelTaskCanvas task={task} step={step} results={results}/>;
  return <>{fallback}</>;
}
