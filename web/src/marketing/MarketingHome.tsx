import {
  Archive,
  ArrowRight,
  BookOpen,
  CheckCircle2,
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
import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { BrandMark } from '../components/Brand';
import { consolePath } from '../consoleRoutes';
import { docsStatusLabel, docsWebPath, loadDocsCatalog, type DocsCatalog } from '../docs/docs';
import { HeroWorkCanvas, MarketingSystemShowcase } from './MarketingProductVisuals';
import './marketing.css';

const creationStages = [
  ['01', '资料准备', '已完成', 'is-complete'],
  ['02', '内容策略', '等待确认', 'is-current'],
  ['03', '脚本方案', '待开始', ''],
  ['04', '分镜与素材', '待开始', ''],
  ['05', '交付', '待开始', ''],
] as const;

const contentIcons = { 'marketing-video': Video, 'wechat-article': FileText } as const;

export function MarketingHome() {
  const [catalog, setCatalog] = useState<DocsCatalog>();
  const [catalogState, setCatalogState] = useState<'loading'|'ready'|'unavailable'>('loading');
  const [menuOpen, setMenuOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    document.title = 'Content Work OS';
    const description = 'Content Work OS 连接资料、证据、知识、Agent、模型、Worker、人工和发布渠道，让内容结果可以审核、交付并持续复用。';
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
    const onScroll = () => setScrolled(window.scrollY > 24);
    onScroll();
    window.addEventListener('scroll', onScroll, {passive:true});
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  return <div className="marketing-page">
    <header className={`marketing-header ${scrolled || menuOpen ? 'is-scrolled' : ''}`}>
      <div className="marketing-header-inner">
        <a className="marketing-brand" href="#overview" aria-label="Content Work OS 首页" onClick={() => setMenuOpen(false)}>
          <BrandMark className="marketing-brand-mark"/>
          <span><strong>Content Work OS</strong><small>内容创作 AI Infra</small></span>
        </a>
        <nav className="marketing-nav" aria-label="官网主导航">
          <a href="#product">创作流程</a>
          <a href="#system">AI Infra</a>
          <a href="#content-packs">可用场景</a>
          <Link to="/docs">文档</Link>
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
        <a href="#system" onClick={()=>setMenuOpen(false)}>AI Infra</a>
        <a href="#content-packs" onClick={()=>setMenuOpen(false)}>可用场景</a>
        <Link to="/docs" onClick={()=>setMenuOpen(false)}>文档</Link>
        <Link to={consolePath.studio} onClick={()=>setMenuOpen(false)}>进入创作台 <ArrowRight size={16}/></Link>
      </nav>}
    </header>

    <main className="marketing-main">
      <HeroWorkCanvas/>
      <section className="marketing-hero" id="overview">
        <div className="marketing-shell marketing-hero-content">
          <span className="marketing-overline"><CircleDot size={13}/> STUDIO-FIRST CONTENT CREATION INFRA</span>
          <h1><span>Content Work OS</span><strong>让内容生产从一次交付，变成<em>持续积累</em></strong></h1>
          <p>把资料和已有成果带进任务。系统组织证据、知识、Agent、模型、Worker 和人工确认，产出可审核、可交付、可复用的内容结果。</p>
          <div className="marketing-hero-actions">
            <Link className="marketing-primary-button" to={consolePath.studio}><Sparkles size={17}/>开始一项创作</Link>
            <a className="marketing-secondary-button" href="#product"><Workflow size={17}/>查看创作流程</a>
          </div>
          <div className="marketing-hero-facts" aria-label="产品能力摘要">
            <span><CheckCircle2 size={15}/>来源、版本与审核全程可追溯</span>
            <span><CheckCircle2 size={15}/>人物、剧本、分镜与媒体结果持续复用</span>
          </div>
        </div>
      </section>

      <section className="marketing-section marketing-product" id="product">
        <div className="marketing-shell">
          <header className="marketing-section-heading">
            <span>从任务输入，到可交付结果</span>
            <h2>复杂执行留在系统里，<br/>客户只处理当前决定</h2>
            <p>资料、项目参考、当前进度、候选结果和人工门禁收敛在一项创作任务里；每一步都显示阻断原因和唯一推荐动作。</p>
          </header>
          <div className="marketing-workspace-preview" aria-label="Content Work OS 客户创作任务预览">
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
                <Archive size={19}/><span><strong>结果资产参与每一次创作</strong><small>人物、剧本、分镜、图片和视频结果会继续服务下一次任务。</small></span><a href="#system">查看资产闭环 <ArrowRight size={14}/></a>
              </footer>
            </div>
          </div>
        </div>
      </section>

      <MarketingSystemShowcase/>

      <section className="marketing-section marketing-packs" id="content-packs">
        <div className="marketing-shell">
          <header className="marketing-section-heading is-centered">
            <span>当前可用场景</span>
            <h2>先选想完成的内容，<br/>再进入已验证的生产链</h2>
            <p>公开场景状态来自文档目录。底层能力已实现，不代表外部平台账号、配额或自动发布已经接通。</p>
          </header>
          <div className="marketing-pack-list">
            {catalogState==='ready'&&catalog?.content_types.map((content,index)=>{
              const Icon=contentIcons[content.id as keyof typeof contentIcons]||FileText;
              const path=docsWebPath(content.page_slug)||'/docs';
              return <article key={content.id} className={index===0?'is-primary':''}>
                <header><span>能力包 {String(index+1).padStart(2,'0')}</span><b className={`is-${content.status}`}>{docsStatusLabel(content.status)}</b></header>
                <Icon size={26}/><h3>{content.title}</h3><p>{content.summary}</p>
                <Link to={path}>查看内容形态 <ArrowRight size={15}/></Link>
              </article>;
            })}
            {catalogState==='loading'&&<div className="marketing-pack-state"><CircleDot size={22}/><div><strong>正在读取内容能力包目录</strong><span>能力状态以公开登记目录为准。</span></div></div>}
            {catalogState==='unavailable'&&<div className="marketing-pack-state is-error"><CircleDot size={22}/><div><strong>状态目录暂不可用</strong><span>官网不会猜测客户端或内容能力包的启用状态。</span><Link to="/docs">前往使用文档 <ArrowRight size={14}/></Link></div></div>}
            <article className="is-planned"><header><span>扩展中的场景</span><b>按状态开放</b></header><Sparkles size={26}/><h3>抖音电商与小说连载</h3><p>类型化契约、本地校验和渠道基础能力正在形成；只有完整客户路径经过验证后才会进入公开可用目录。</p><Link to="/docs">查看能力边界 <ArrowRight size={15}/></Link></article>
          </div>
        </div>
      </section>

      <section className="marketing-cta">
        <div className="marketing-shell">
          <span>CONTENT WORK OS</span>
          <h2>带入已有资料，<br/>开始一项明确的创作任务</h2>
          <p>选择已验证的内容场景，在需要判断的地方做决定，其余执行、版本和追溯交给系统。</p>
          <div><Link className="marketing-primary-button" to={consolePath.studio}><Play size={16}/>开始创作</Link><Link className="marketing-secondary-button" to="/docs"><BookOpen size={16}/>查看使用文档</Link></div>
        </div>
      </section>
    </main>

    <footer className="marketing-footer">
      <div className="marketing-shell marketing-footer-grid">
        <div><a className="marketing-brand is-footer" href="#overview"><BrandMark className="marketing-brand-mark"/><span><strong>Content Work OS</strong><small>内容创作 AI Infra</small></span></a><p>连接资料、证据、执行者与发布渠道，让每次结果可以审核、交付和继续复用。</p></div>
        <nav aria-label="产品链接"><strong>产品</strong><a href="#product">创作流程</a><a href="#system">AI Infra</a><a href="#content-packs">可用场景</a></nav>
        <nav aria-label="资源链接"><strong>资源</strong><Link to="/docs">使用文档</Link><Link to="/docs/clients/codex">Codex 接入</Link><Link to="/docs/content/wechat-article">公众号文章</Link></nav>
        <nav aria-label="账户链接"><strong>账户</strong><Link to="/login">登录</Link><Link to="/register">注册</Link><Link to={consolePath.studio}>创作台</Link></nav>
      </div>
      <div className="marketing-shell marketing-footer-bottom"><span>© {new Date().getFullYear()} Content Work OS</span><span>本地优先 · 受控流程 · 全程可追溯</span></div>
    </footer>
  </div>;
}
