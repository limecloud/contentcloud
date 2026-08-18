import { existsSync, readFileSync } from 'node:fs';
import { renderToStaticMarkup } from 'react-dom/server';
import { StaticRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { MarketingHome } from './MarketingHome';

const renderHome=()=>renderToStaticMarkup(<StaticRouter location="/"><MarketingHome/></StaticRouter>);

describe('Content Work OS website',()=>{
  it('renders the product claim and routes the primary CTA to the authenticated workspace',()=>{
    const markup=renderHome();
    expect(markup).toContain('Content Work OS');
    expect(markup).toContain('从一句需求，');
    expect(markup).toContain('<strong>到可交付成果</strong>');
    expect(markup).toContain('class="marketing-pixel-scene"');
    expect(markup).toContain('href="/studio"');
    expect(markup).toContain('创作资料目录，不只是一个文件夹');
    expect(markup).toContain('确认结果会继续复用');
    expect(markup).not.toContain('品牌、灵感和已批准结果会继续服务下一次任务');
    expect(markup).toContain('电脑、平台和外部服务各司其职');
    expect(markup).toContain('电脑上的创作工具');
    expect(markup).toContain('哪些功能现在能用，哪些还需要连接');
    expect(markup).toContain('平台搜索 · 网页采集 · 来源记录 · 访问限制');
    expect(markup).toContain('公众号目前需要人工发布');
    expect(markup).toContain('抖音电商与小说连载');
  });

  it('keeps content-pack status pending until the public catalog has loaded',()=>{
    const markup=renderHome();
    expect(markup).toContain('正在读取可用内容场景');
    expect(markup).not.toContain('Claude Code</span><small>可用');
  });

  it('keeps the shared studio asset for auth while the homepage uses its own scene',()=>{
    const css=readFileSync(new URL('./marketing.css',import.meta.url),'utf8');
    expect(css).toContain("url('/images/content-work-os-studio.webp')");
    expect(css).toContain("url('/images/content-work-os-pixel-field.webp')");
    expect(existsSync(new URL('../../public/images/content-work-os-studio.webp',import.meta.url))).toBe(true);
    expect(existsSync(new URL('../../public/images/content-work-os-pixel-field.webp',import.meta.url))).toBe(true);
  });

  it('ships the scroll narrative with an accessible reduced-motion fallback',()=>{
    const markup=renderHome();
    const css=readFileSync(new URL('./marketing.css',import.meta.url),'utf8');
    expect(markup).toContain('class="marketing-pixel-field"');
    expect(markup).toContain('data-reveal="true"');
    expect(css).toContain('@keyframes marketing-pixel-float');
    expect(css).toContain('@media(prefers-reduced-motion:reduce)');
    expect(css).toContain('animation:none!important');
  });
});
