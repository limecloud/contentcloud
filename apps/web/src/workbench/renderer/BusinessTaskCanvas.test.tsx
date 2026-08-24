import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { BusinessTaskCanvas } from './BusinessTaskCanvas';

const task={title:'春季内容任务',intent:'突出商品的真实卖点',content_type:'commerce',asset_count:3,project:{brand_name:'金陵古都'}};
const step={title:'内容变体',outcome_description:'生成渠道版本',status:'working'};
const results=[{title:'版本 A',status:'draft',summary:'候选内容'}];

describe('business task canvas host',()=>{
  it('routes marketing video tasks to the production canvas',()=>{
    const markup=renderToStaticMarkup(<BusinessTaskCanvas task={{...task,content_type:'marketing_video'}} step={{...step,title:'视频分镜'}} results={results} fallback={<div>fallback</div>}/>);
    expect(markup).toContain('视频生产');
    expect(markup).toContain('生产阶段');
    expect(markup).not.toContain('fallback');
  });

  it('uses the frozen approved layout before content-type aliases',()=>{
    const markup=renderToStaticMarkup(<BusinessTaskCanvas task={{...task,content_type:'unknown'}} step={step} results={results} workbench={{plugin_id:'approved-video',layout:'stage-canvas-context'}} fallback={<div>fallback</div>}/>);
    expect(markup).toContain('视频生产');
    expect(markup).not.toContain('fallback');
  });

  it('fails closed for an unknown approved layout',()=>{
    const markup=renderToStaticMarkup(<BusinessTaskCanvas task={task} step={step} results={results} workbench={{plugin_id:'untrusted',layout:'remote-bundle'}} fallback={<div>fallback</div>}/>);
    expect(markup).toContain('fallback');
    expect(markup).not.toContain('商品事实');
  });

  it('routes commerce tasks to the product variant canvas',()=>{
    const markup=renderToStaticMarkup(<BusinessTaskCanvas task={task} step={step} results={results} fallback={<div>fallback</div>}/>);
    expect(markup).toContain('商品事实');
    expect(markup).toContain('卖点与内容变体');
    expect(markup).not.toContain('fallback');
  });

  it('routes serialized novel tasks to the Canon and chapter canvas',()=>{
    const markup=renderToStaticMarkup(<BusinessTaskCanvas task={{...task,content_type:'serialized_novel'}} step={{...step,title:'章节候选'}} results={results} fallback={<div>fallback</div>}/>);
    expect(markup).toContain('连载小说');
    expect(markup).toContain('章节流程');
    expect(markup).not.toContain('fallback');
  });

  it('keeps unknown content types on the shared fallback surface',()=>{
    const markup=renderToStaticMarkup(<BusinessTaskCanvas task={{...task,content_type:'unknown'} } step={step} results={results} fallback={<div>fallback</div>}/>);
    expect(markup).toContain('fallback');
  });
});
