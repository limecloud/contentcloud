// @ts-expect-error Vitest 在 Node 中运行；Web 应用的类型边界不引入 Node globals。
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
    expect(markup).toContain('把任务输入、创作与交付组织成一个系统');
    expect(markup).toContain('class="marketing-home-scene"');
    expect(markup).toContain('href="/studio"');
    expect(markup).toContain('创作资产目录，不是另一个文件夹');
    expect(markup).toContain('结果资产参与每一次创作');
    expect(markup).not.toContain('品牌、灵感和已批准结果会继续服务下一次任务');
    expect(markup).toContain('客户页面保持简单，复杂能力留在平台底层');
    expect(markup).toContain('ContentCloud Agentic Job Runtime');
  });

  it('keeps content-pack status pending until the public catalog has loaded',()=>{
    const markup=renderHome();
    expect(markup).toContain('正在读取内容能力包目录');
    expect(markup).not.toContain('Claude Code</span><small>可用');
  });

  it('keeps the natural-light studio image behind the product canvas',()=>{
    const css=readFileSync(new URL('./marketing.css',import.meta.url),'utf8');
    expect(css).toContain("url('/images/content-work-os-studio.webp')");
    expect(existsSync(new URL('../../public/images/content-work-os-studio.webp',import.meta.url))).toBe(true);
  });
});
