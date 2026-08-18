import {
  Archive,
  ArrowRight,
  BookOpen,
  CircleDot,
  FileSearch,
  FileText,
  Lightbulb,
  Menu,
  Play,
  Sparkles,
  Video,
  Workflow,
  X,
} from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { BrandMark } from '../components/Brand';
import { consolePath } from '../consoleRoutes';
import { docsStatusLabel, docsWebPath, loadDocsCatalog, type DocsCatalog } from '../docs/docs';
import { MarketingSystemShowcase } from './MarketingProductVisuals';
import './marketing.css';

const creationStages = [
  ['01', '资料准备', '已完成', 'is-complete'],
  ['02', '内容策略', '等待确认', 'is-current'],
  ['03', '脚本方案', '待开始', ''],
  ['04', '分镜与素材', '待开始', ''],
  ['05', '交付', '待开始', ''],
] as const;

const contentIcons = { 'marketing-video': Video, 'wechat-article': FileText } as const;

const pixelClusters = [
  {className:'is-top-left',rows:[[3],[2],[1]]},
  {className:'is-left-center',rows:[[1],[2],[1]]},
  {className:'is-left-bottom',rows:[[5],[2,1],[6],[3],[2]]},
  {className:'is-right-top',rows:[[8,3],[5,4,2],[7,2],[3,6],[2,3,4],[6,2],[4]]},
  {className:'is-right-center',rows:[[1],[2],[1,2],[3],[2]]},
  {className:'is-right-bottom',rows:[[5],[3,1],[6],[2,3],[4],[2]]},
] as const;

function PixelField() {
  return <div className="marketing-pixel-field" aria-hidden="true">
    {pixelClusters.map(cluster=><div className={`marketing-pixel-cluster ${cluster.className}`} key={cluster.className}>
      {cluster.rows.map((row,rowIndex)=><div className="marketing-pixel-row" key={rowIndex}>
        {row.map((width,index)=><span style={{width:width*18}} key={`${rowIndex}-${index}`}/>) }
      </div>)}
    </div>)}
  </div>;
}

export function MarketingHome() {
  const pageRef = useRef<HTMLDivElement>(null);
  const [catalog, setCatalog] = useState<DocsCatalog>();
  const [catalogState, setCatalogState] = useState<'loading'|'ready'|'unavailable'>('loading');
  const [menuOpen, setMenuOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    document.title = 'Content Work OS';
    const description = 'Content Work OS 连接资料、参考内容、制作步骤和人工确认，让每次内容结果都能审核、交付并继续复用。';
    let meta = document.querySelector<HTMLMetaElement>('meta[name="description"]');
    if (!meta) {
      meta = document.createElement('meta');
      meta.name = 'description';
      document.head.appendChild(meta);
    }
    meta.content = description;
  }, []);

  useEffect(() => {
    let active = true;
    loadDocsCatalog()
      .then(value => { if (active) { setCatalog(value); setCatalogState('ready'); } })
      .catch(() => { if (active) setCatalogState('unavailable'); });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    let frame = 0;
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const onScroll = () => {
      if (frame) return;
      frame = window.requestAnimationFrame(() => {
        const scrollY = window.scrollY;
        setScrolled(scrollY > 24);
        pageRef.current?.style.setProperty('--marketing-hero-shift', reduceMotion ? '0px' : `${Math.max(-32, scrollY * -0.045)}px`);
        frame = 0;
      });
    };
    onScroll();
    window.addEventListener('scroll', onScroll, {passive:true});
    return () => {
      window.removeEventListener('scroll', onScroll);
      if (frame) window.cancelAnimationFrame(frame);
    };
  }, []);

  useEffect(() => {
    const page = pageRef.current;
    if (!page) return;
    const targets = Array.from(page.querySelectorAll<HTMLElement>('[data-reveal]'));
    page.classList.add('is-motion-ready');
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches || !('IntersectionObserver' in window)) {
      targets.forEach(target => target.classList.add('is-visible'));
      return;
    }
    const observer = new IntersectionObserver(entries => {
      entries.forEach(entry => {
        if (!entry.isIntersecting) return;
        (entry.target as HTMLElement).classList.add('is-visible');
        observer.unobserve(entry.target);
      });
    }, {rootMargin:'0px 0px -10% 0px',threshold:0.12});
    targets.forEach(target => observer.observe(target));
    return () => observer.disconnect();
  }, [catalogState]);

  return <div className="marketing-page" ref={pageRef}>
    <header className={`marketing-header ${scrolled ? 'is-scrolled' : ''} ${menuOpen ? 'is-menu-open' : ''}`}>
      <button className="marketing-scene-menu" type="button" aria-label={menuOpen?'关闭网站导航':'打开网站导航'} aria-expanded={menuOpen} onClick={() => setMenuOpen(value=>!value)}>
        {menuOpen?<X size={24}/>:<Menu size={24}/>}<span>导航</span>
      </button>
      <div className="marketing-header-inner">
        <a className="marketing-brand" href="#overview" aria-label="Content Work OS 首页" onClick={() => setMenuOpen(false)}>
          <BrandMark className="marketing-brand-mark"/>
          <span><strong>Content Work OS</strong><small>内容创作工作台</small></span>
        </a>
        <nav className="marketing-nav" aria-label="官网主导航">
          <a className="marketing-nav-link" data-label="创作流程" href="#product"><span>创作流程</span></a>
          <a className="marketing-nav-link" data-label="平台能力" href="#system"><span>平台能力</span></a>
          <a className="marketing-nav-link" data-label="可用场景" href="#content-packs"><span>可用场景</span></a>
          <Link className="marketing-nav-link" data-label="文档" to="/docs"><span>文档</span></Link>
        </nav>
        <div className="marketing-header-actions">
          <Link className="marketing-text-link" to="/login">登录</Link>
          <Link className="marketing-header-button" to={consolePath.studio}>开始创作 <ArrowRight size={15}/></Link>
        </div>
        <button className="marketing-menu-button" type="button" aria-label={menuOpen?'关闭导航':'打开导航'} aria-expanded={menuOpen} onClick={() => setMenuOpen(value=>!value)}>
          {menuOpen?<X size={21}/>:<Menu size={21}/>}
        </button>
      </div>
      {menuOpen&&<nav className="marketing-mobile-nav" aria-label="移动端导航">
        <a href="#product" onClick={()=>setMenuOpen(false)}>创作流程</a>
        <a href="#system" onClick={()=>setMenuOpen(false)}>平台能力</a>
        <a href="#content-packs" onClick={()=>setMenuOpen(false)}>可用场景</a>
        <Link to="/docs" onClick={()=>setMenuOpen(false)}>文档</Link>
        <Link to="/login" onClick={()=>setMenuOpen(false)}>登录</Link>
        <Link to={consolePath.studio} onClick={()=>setMenuOpen(false)}>进入创作台 <ArrowRight size={16}/></Link>
      </nav>}
    </header>

    <main className="marketing-main">
      <section className="marketing-hero" id="overview">
        <div className="marketing-pixel-scene" aria-hidden="true"><PixelField/></div>
        <div className="marketing-shell marketing-hero-content">
          <span className="marketing-overline" data-reveal data-reveal-order="1"><CircleDot size={13}/> CONTENT WORK OS</span>
          <h1 data-reveal data-reveal-order="2">从一句需求，<strong>到可交付成果</strong></h1>
          <p data-reveal data-reveal-order="3">资料、策略、创作和审核持续接力，把每次内容成果沉淀为下一次任务可以复用的资产。</p>
          <div className="marketing-hero-actions" data-reveal data-reveal-order="4">
            <Link className="marketing-primary-button" to={consolePath.studio}><Sparkles size={17}/>开始一项创作</Link>
            <a className="marketing-secondary-button" href="#product"><Workflow size={17}/>查看创作流程</a>
          </div>
        </div>
      </section>

      <section className="marketing-section marketing-product" id="product">
        <div className="marketing-shell">
          <div className="marketing-product-glimpse" aria-label="Content Work OS 创作任务摘要">
            <div className="marketing-glimpse-tabs">
              <span><i/>资料按版本固定</span><span><i/>关键决定人工确认</span><span><i/>结果进入下一次创作</span>
            </div>
            <div className="marketing-glimpse-body">
              <BrandMark/>
              <span><small>春日线香品牌片</small><strong>内容策略正在等待确认</strong></span>
              <Link to={consolePath.studio}>继续任务 <ArrowRight size={15}/></Link>
            </div>
          </div>
        </div>
        <div className="marketing-shell marketing-product-story">
          <header className="marketing-section-heading marketing-sticky-heading" data-reveal>
            <span>从任务输入，到可交付结果</span>
            <h2>复杂执行留在系统里，<br/>客户只处理当前决定</h2>
            <p>资料、项目参考、当前进度、候选结果和确认事项都集中在一项创作任务里；每一步都会告诉你为什么停下，以及接下来该做什么。</p>
          </header>
          <div className="marketing-workspace-preview" aria-label="Content Work OS 客户创作任务预览" data-reveal data-reveal-order="2">
            <div className="marketing-preview-topbar">
              <span className="marketing-preview-brand"><BrandMark/> 春日线香品牌片</span>
              <span className="marketing-preview-health"><i/> 营销视频创作</span>
              <span className="marketing-preview-runtime">草稿已保存 · 2 分钟前</span>
            </div>
            <div className="marketing-studio-body">
              <header className="marketing-task-heading">
                <div><span>创作任务 / 内容策略</span><h3>确认这版核心受众</h3></div>
                <strong>等待你确认</strong>
                <p>系统已经整理好项目参考、已确认的人物原型和本次灵感。确认后，流水线会继续生成剧本方案。</p>
              </header>
              <ol className="marketing-task-progress" aria-label="营销视频创作进度">
                {creationStages.map(([index,title,status,state])=><li className={state} key={index}><i>{index}</i><span>{title}</span><strong>{status}</strong></li>)}
              </ol>
              <div className="marketing-task-workarea">
                <section className="marketing-task-sources">
                  <header><div><FileSearch size={17}/><span><strong>本次创作输入</strong><small>所有引用都保留来源和固定版本</small></span></div><b>8 项</b></header>
                  <div><span><Lightbulb size={15}/>灵感观察</span><strong>春日居家放松场景</strong><small>人工补充</small></div>
                  <div><span><Archive size={15}/>项目参考</span><strong>线香品牌规范与产品边界</strong><small>项目参考</small></div>
                  <div><span><FileText size={15}/>已有创作结果</span><strong>核心人物原型与高互动脚本 03</strong><small>已确认</small></div>
                </section>
                <aside className="marketing-next-action">
                  <span>下一步</span>
                  <h4>确认核心受众</h4>
                  <p>都市独居女性，关注睡前放松和空间氛围。</p>
                  <Link to={consolePath.studio}>进入任务 <ArrowRight size={15}/></Link>
                  <small>确认后进入剧本方案</small>
                </aside>
              </div>
              <footer className="marketing-task-assets">
                <Archive size={19}/><span><strong>确认结果会继续复用</strong><small>人物、剧本、分镜、图片和视频结果会继续服务下一次任务。</small></span><a href="#system">查看资料复用 <ArrowRight size={14}/></a>
              </footer>
            </div>
          </div>
        </div>
      </section>

      <MarketingSystemShowcase/>

      <section className="marketing-section marketing-packs" id="content-packs">
        <div className="marketing-shell">
          <header className="marketing-section-heading is-centered" data-reveal>
            <span>当前可用场景</span>
            <h2>先选想完成的内容，<br/>再进入已验证的制作流程</h2>
            <p>这里显示的是当前真正可以使用的内容场景。系统已有基础功能，不代表外部平台账号、额度或自动发布已经接通。</p>
          </header>
          <div className="marketing-pack-list">
            {catalogState==='ready'&&catalog?.content_types.map((content,index)=>{
              const Icon=contentIcons[content.id as keyof typeof contentIcons]||FileText;
              const path=docsWebPath(content.page_slug)||'/docs';
              return <article key={content.id} className={index===0?'is-primary':''} data-reveal data-reveal-order={String(Math.min(index+1,5))}>
                <header><span>内容场景 {String(index+1).padStart(2,'0')}</span><b className={`is-${content.status}`}>{docsStatusLabel(content.status)}</b></header>
                <Icon size={26}/><h3>{content.title}</h3><p>{content.summary}</p>
                <Link to={path}>查看内容形态 <ArrowRight size={15}/></Link>
              </article>;
            })}
            {catalogState==='loading'&&<div className="marketing-pack-state" data-reveal><CircleDot size={22}/><div><strong>正在读取可用内容场景</strong><span>状态以平台当前记录为准。</span></div></div>}
            {catalogState==='unavailable'&&<div className="marketing-pack-state is-error" data-reveal><CircleDot size={22}/><div><strong>暂时无法读取场景状态</strong><span>我们不会猜测哪些功能已经开通。</span><Link to="/docs">前往使用文档 <ArrowRight size={14}/></Link></div></div>}
            <article className="is-planned" data-reveal data-reveal-order="3"><header><span>扩展中的场景</span><b>按状态开放</b></header><Sparkles size={26}/><h3>抖音电商与小说连载</h3><p>内容规则、基础检查和渠道功能正在准备；只有完整客户路径经过验证后才会进入公开可用目录。</p><Link to="/docs">查看可用范围 <ArrowRight size={15}/></Link></article>
          </div>
        </div>
      </section>

      <section className="marketing-cta">
        <div className="marketing-shell" data-reveal>
          <span>CONTENT WORK OS</span>
          <h2>带入已有资料，<br/>开始一项明确的创作任务</h2>
          <p>选择已验证的内容场景，在需要判断的地方做决定，其余执行、版本和追溯交给系统。</p>
          <div><Link className="marketing-primary-button" to={consolePath.studio}><Play size={16}/>开始创作</Link><Link className="marketing-secondary-button" to="/docs"><BookOpen size={16}/>查看使用文档</Link></div>
        </div>
      </section>
    </main>

    <footer className="marketing-footer">
      <div className="marketing-shell marketing-footer-grid">
        <div><a className="marketing-brand is-footer" href="#overview"><BrandMark className="marketing-brand-mark"/><span><strong>Content Work OS</strong><small>内容创作工作台</small></span></a><p>连接资料、参考内容和制作步骤，让每次结果都能审核、交付并继续复用。</p></div>
        <nav aria-label="产品链接"><strong>产品</strong><a href="#product">创作流程</a><a href="#system">平台能力</a><a href="#content-packs">可用场景</a></nav>
        <nav aria-label="资源链接"><strong>资源</strong><Link to="/docs">使用文档</Link><Link to="/docs/clients/codex">Codex 接入</Link><Link to="/docs/content/wechat-article">公众号文章</Link></nav>
        <nav aria-label="账户链接"><strong>账户</strong><Link to="/login">登录</Link><Link to="/register">注册</Link><Link to={consolePath.studio}>创作台</Link></nav>
      </div>
      <div className="marketing-shell marketing-footer-bottom"><span>© {new Date().getFullYear()} Content Work OS</span><span>资料可追溯 · 步骤可确认 · 结果可复用</span></div>
    </footer>
  </div>;
}
