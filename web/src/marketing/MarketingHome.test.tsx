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
    expect(markup).toContain('让内容生产从一次交付，变成');
    expect(markup).toContain('<em>持续积累</em>');
    expect(markup).toContain('class="marketing-home-scene"');
    expect(markup).toContain('href="/studio"');
    expect(markup).toContain('创作资产目录，不是另一个文件夹');
    expect(markup).toContain('结果资产参与每一次创作');
    expect(markup).not.toContain('品牌、灵感和已批准结果会继续服务下一次任务');
    expect(markup).toContain('三种执行平面协作，但不混淆事实所有权');
    expect(markup).toContain('开放 Agent Harness');
    expect(markup).toContain('代码已经具备什么，外部还需要接通什么');
    expect(markup).toContain('source.search / source.fetch');
    expect(markup).toContain('公众号当前为人工发布');
    expect(markup).toContain('抖音电商与小说连载');
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
