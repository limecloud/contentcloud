import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { BusinessBriefFields } from './BusinessBriefFields';
import { emptyBusinessBriefForm } from '../contract/businessBrief';

describe('business brief workbench fields',()=>{
  it('keeps business-specific language in the business feature folders',()=>{
    const value=emptyBusinessBriefForm();
    expect(renderToStaticMarkup(<BusinessBriefFields contentType="marketing_video" value={value} onChange={()=>{}}/>)).toContain('视频业务简报');
    expect(renderToStaticMarkup(<BusinessBriefFields contentType="article" value={value} onChange={()=>{}}/>)).toContain('文章业务简报');
    expect(renderToStaticMarkup(<BusinessBriefFields contentType="product-content" value={value} onChange={()=>{}}/>)).toContain('商品业务简报');
    expect(renderToStaticMarkup(<BusinessBriefFields contentType="unknown" value={value} onChange={()=>{}}/>)).toBe('');
  });
});
