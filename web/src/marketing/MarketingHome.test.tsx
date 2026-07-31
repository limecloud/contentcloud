import { renderToStaticMarkup } from 'react-dom/server';
import { StaticRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { MarketingHome } from './MarketingHome';

const renderHome=()=>renderToStaticMarkup(<StaticRouter location="/"><MarketingHome/></StaticRouter>);

describe('Content Work OS website',()=>{
  it('renders the product claim and routes the primary CTA to the authenticated workspace',()=>{
    const markup=renderHome();
    expect(markup).toContain('Content Work OS');
    expect(markup).toContain('让内容工作，自然流转');
    expect(markup).toContain('class="marketing-home-scene"');
    expect(markup).toContain('href="/workspace"');
    expect(markup).toContain('视频默认，公众号按租户开通');
  });

  it('keeps capability status pending until the public catalog has loaded',()=>{
    const markup=renderHome();
    expect(markup).toContain('正在读取客户端目录');
    expect(markup).toContain('正在读取 Content Pack 目录');
    expect(markup).not.toContain('Claude Code</span><small>可用');
  });
});
