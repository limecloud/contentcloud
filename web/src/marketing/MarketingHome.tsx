import {
  ArrowRight,
  BookOpen,
  Check,
  ChevronRight,
  CircleDot,
  FileText,
  LayoutDashboard,
  Menu,
  MonitorUp,
  PackageCheck,
  Play,
  ShieldCheck,
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
import './marketing.css';

const flow = [
  ['01', 'Context', '品牌与项目边界'],
  ['02', 'Knowledge', '可信事实与证据'],
  ['03', 'Strategy', '人群与内容假设'],
  ['04', 'Content Pack', '视频或文章'],
  ['05', 'Review', '评论与批准'],
  ['06', 'Delivery', '批准后交付'],
] as const;

const contentIcons = { 'marketing-video': Video, 'wechat-article': FileText } as const;

export function MarketingHome() {
  const [catalog, setCatalog] = useState<DocsCatalog>();
  const [catalogState, setCatalogState] = useState<'loading'|'ready'|'unavailable'>('loading');
  const [menuOpen, setMenuOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    document.title = 'Content Work OS';
    const description = 'Content Work OS 让本地 Agent、团队审核与多渠道交付在同一条可追溯生产链中协作。';
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
          <span><strong>Content Work OS</strong><small>Content operations</small></span>
        </a>
        <nav className="marketing-nav" aria-label="官网主导航">
          <a href="#product">产品</a>
          <a href="#content-packs">内容形态</a>
          <a href="#governance">治理</a>
          <Link to="/docs">文档</Link>
        </nav>
        <div className="marketing-header-actions">
          <Link className="marketing-text-link" to="/login">登录</Link>
          <Link className="marketing-header-button" to={consolePath.dashboard}>进入工作台 <ArrowRight size={15}/></Link>
        </div>
        <button className="marketing-menu-button" type="button" aria-label={menuOpen?'关闭导航':'打开导航'} aria-expanded={menuOpen} onClick={() => setMenuOpen(value=>!value)}>
          {menuOpen?<X size={21}/>:<Menu size={21}/>}
        </button>
      </div>
      {menuOpen&&<nav className="marketing-mobile-nav" aria-label="移动端导航">
        <a href="#product" onClick={()=>setMenuOpen(false)}>产品</a>
        <a href="#content-packs" onClick={()=>setMenuOpen(false)}>内容形态</a>
        <a href="#governance" onClick={()=>setMenuOpen(false)}>治理</a>
        <Link to="/docs" onClick={()=>setMenuOpen(false)}>文档</Link>
        <Link to={consolePath.dashboard} onClick={()=>setMenuOpen(false)}>进入工作台 <ArrowRight size={16}/></Link>
      </nav>}
    </header>

    <main className="marketing-main">
      <div className="marketing-home-scene" aria-hidden="true"/>
      <section className="marketing-hero" id="overview">
        <div className="marketing-shell marketing-hero-content">
          <span className="marketing-overline"><CircleDot size={13}/> CONTENT WORK OS</span>
          <h1><span>Content Work OS</span><strong>让内容工作，自然流转</strong></h1>
          <p>在熟悉的本地 Agent 中完成创作，让团队在云端接住审核、批准与交付。每个版本都有来源，每种能力都有边界。</p>
          <div className="marketing-hero-actions">
            <Link className="marketing-primary-button" to={consolePath.dashboard}><LayoutDashboard size={17}/>进入工作台</Link>
            <Link className="marketing-secondary-button" to="/docs"><BookOpen size={17}/>查看使用文档</Link>
          </div>
          <div className="marketing-entry-grid" aria-label="ContentCloud 三个核心工作面">
            <a href="#runtime"><MonitorUp size={18}/><span><strong>本地工作区</strong><small>Agent、Skill 与项目上下文</small></span><ChevronRight size={16}/></a>
            <a href="#governance"><ShieldCheck size={18}/><span><strong>云端治理</strong><small>版本、审核与审计事实</small></span><ChevronRight size={16}/></a>
            <a href="#content-packs"><PackageCheck size={18}/><span><strong>内容形态</strong><small>视频默认，公众号按租户开通</small></span><ChevronRight size={16}/></a>
          </div>
        </div>
      </section>

      <section className="marketing-section marketing-product" id="product">
        <div className="marketing-shell">
          <header className="marketing-section-heading is-centered">
            <span>WORK SURFACE</span>
            <h2>不只是生成，<br/>更是团队可接住的工作系统</h2>
            <p>本地候选与云端正式事实各司其职，创作速度和团队治理不必互相牺牲。</p>
          </header>
          <div className="marketing-workspace-preview" aria-label="Content Work OS 工作台预览">
            <div className="marketing-preview-topbar">
              <span className="marketing-preview-brand"><BrandMark/> 金陵古法线香</span>
              <span className="marketing-preview-health"><i/> Workspace 已连接</span>
              <span className="marketing-preview-runtime">Codex · manifest verified</span>
            </div>
            <div className="marketing-preview-body">
              <aside className="marketing-preview-sidebar">
                <span>WORKSPACE</span>
                <b className="is-current"><LayoutDashboard size={14}/>今天的生产面</b>
                <b><Sparkles size={14}/>内容策划</b>
                <b><ShieldCheck size={14}/>审核协作</b>
                <b><Workflow size={14}/>Automation</b>
                <small>CONTENT PACKS</small>
                <b><Video size={14}/>营销视频</b>
                <b><FileText size={14}/>公众号文章</b>
              </aside>
              <div className="marketing-preview-main">
                <header><div><span>2026.07.31 / WORK SURFACE</span><h3>今天的内容工作</h3></div><button type="button" aria-label="筛选当前工作面">全部项目 <ChevronRight size={14}/></button></header>
                <div className="marketing-preview-metrics">
                  <div><span>待审核</span><strong>06</strong><small>2 项今天到期</small></div>
                  <div><span>运行中</span><strong>02</strong><small>Daemon 状态正常</small></div>
                  <div><span>已交付</span><strong>18</strong><small>本月批准快照</small></div>
                </div>
                <div className="marketing-preview-list">
                  <header><span>正在推进</span><span>类型</span><span>状态</span></header>
                  <article><span><b>618 促销视频 · variant B</b><small>submission rev 18 · 3 分钟前</small></span><em>营销视频</em><strong className="is-review">等待审核</strong></article>
                  <article><span><b>春日线香故事 · draft 02</b><small>article item · 12 分钟前</small></span><em>公众号文章</em><strong className="is-ready">租户已开通</strong></article>
                  <article><span><b>品牌知识更新 · attempt 7f1</b><small>automation · 处理 7 / 12</small></span><em>知识提取</em><strong className="is-running">运行中</strong></article>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="marketing-section marketing-flow-section">
        <div className="marketing-shell">
          <header className="marketing-section-heading">
            <span>GOVERNED FLOW</span>
            <h2>从上下文到交付，<br/>每一步都有明确事实</h2>
            <p>Candidate 可以快速变化，批准快照保持稳定。Agent 可以创造，服务端负责权限、版本与审计。</p>
          </header>
          <div className="marketing-flow" aria-label="ContentCloud 受治理生产链">
            {flow.map(([index,title,description])=><div key={index}><span>{index}</span><strong>{title}</strong><small>{description}</small><ArrowRight size={15}/></div>)}
          </div>
        </div>
      </section>

      <section className="marketing-section marketing-runtime" id="runtime">
        <div className="marketing-shell marketing-runtime-layout">
          <div className="marketing-runtime-copy">
            <span>LOCAL RUNTIME</span>
            <h2>你选择 Agent，<br/>ContentCloud 守住边界</h2>
            <p>Agent、Skill、MCP 和媒体工具在本地运行。控制面只签发允许的工作范围，接收明确提交的结果。</p>
            <ul>
              <li><Check size={16}/><span><strong>Workspace 可恢复</strong><small>项目上下文、批准事实与待办在新会话中继续。</small></span></li>
              <li><Check size={16}/><span><strong>Automation 可诊断</strong><small>租约、进度、取消、断网重报与版本门禁都有记录。</small></span></li>
              <li><Check size={16}/><span><strong>外部平台不越权</strong><small>登录、上传和发布始终由用户在目标平台完成。</small></span></li>
            </ul>
          </div>
          <div className="marketing-runtime-console">
            <header><span>runtime / current workspace</span><b>MANIFEST VERIFIED</b></header>
            <div className="marketing-runtime-row"><i>01</i><span><strong>CONTROL PLANE</strong><small>tenant / project / audit</small></span><b>ONLINE</b></div>
            <div className="marketing-runtime-row"><i>02</i><span><strong>WORKSPACE</strong><small>local context / sync state</small></span><b>BOUND</b></div>
            <div className="marketing-runtime-row"><i>03</i><span><strong>AGENT RUNTIME</strong><small>codex / daemon / lease</small></span><b>READY</b></div>
            <div className="marketing-runtime-clients">
              {catalogState==='ready'&&catalog?.clients.map(client=><span key={client.id} className={client.status==='available'?'is-available':''}><i/>{client.display_name}<small>{docsStatusLabel(client.status)}</small></span>)}
              {catalogState==='loading'&&<span className="is-loading"><i/>正在读取客户端目录</span>}
              {catalogState==='unavailable'&&<span className="is-loading"><i/>客户端目录暂不可用</span>}
            </div>
            <footer><span>ENVIRONMENT</span><code>sha256: signed / scoped / traceable</code></footer>
          </div>
        </div>
      </section>

      <section className="marketing-section marketing-packs" id="content-packs">
        <div className="marketing-shell">
          <header className="marketing-section-heading is-centered">
            <span>CONTENT PACKS</span>
            <h2>按内容形态扩展，<br/>不把流程藏进 Prompt</h2>
            <p>每个 Content Pack 都有自己的契约、Skill、审核和交付边界，状态来自公开文档目录。</p>
          </header>
          <div className="marketing-pack-list">
            {catalogState==='ready'&&catalog?.content_types.map((content,index)=>{
              const Icon=contentIcons[content.id as keyof typeof contentIcons]||FileText;
              const path=docsWebPath(content.page_slug)||'/docs';
              return <article key={content.id} className={index===0?'is-primary':''}>
                <header><span>PACK / {String(index+1).padStart(2,'0')}</span><b className={`is-${content.status}`}>{docsStatusLabel(content.status)}</b></header>
                <Icon size={26}/><h3>{content.title}</h3><p>{content.summary}</p>
                <Link to={path}>查看内容形态 <ArrowRight size={15}/></Link>
              </article>;
            })}
            {catalogState==='loading'&&<div className="marketing-pack-state"><CircleDot size={22}/><div><strong>正在读取 Content Pack 目录</strong><span>能力状态将以公开 Registry 为准。</span></div></div>}
            {catalogState==='unavailable'&&<div className="marketing-pack-state is-error"><CircleDot size={22}/><div><strong>状态目录暂不可用</strong><span>官网不会猜测客户端或 Content Pack 的启用状态。</span><Link to="/docs">前往使用文档 <ArrowRight size={14}/></Link></div></div>}
            <article className="is-planned"><header><span>PACK / NEXT</span><b>ROADMAP</b></header><Sparkles size={26}/><h3>更多内容形态</h3><p>Newsletter、社交媒体与播客脚本只有在契约、Skill 和审核链真实可用后才会开放。</p><Link to="/docs">查看路线图 <ArrowRight size={15}/></Link></article>
          </div>
        </div>
      </section>

      <section className="marketing-section marketing-governance" id="governance">
        <div className="marketing-shell">
          <header className="marketing-section-heading">
            <span>GOVERNANCE</span>
            <h2>创作可以快，<br/>正式事实不能含糊</h2>
          </header>
          <div className="marketing-governance-grid">
            <article><span>01</span><ShieldCheck size={24}/><h3>不可变审核</h3><p>审核绑定明确 Revision，批准后生成稳定快照，不会跟着“最新草稿”变化。</p></article>
            <article><span>02</span><PackageCheck size={24}/><h3>租户能力</h3><p>视频默认启用，公众号由后台按租户开通。每一层都会重新验证能力。</p></article>
            <article><span>03</span><Workflow size={24}/><h3>可追溯运行</h3><p>Workspace、Runtime、Pack、Manifest、Submission 和交付之间保留完整关系。</p></article>
          </div>
        </div>
      </section>

      <section className="marketing-cta">
        <div className="marketing-shell">
          <span>CONTENTCLOUD / CONTENT WORK OS</span>
          <h2>让每个 Agent，<br/>都有一块可信的工作面</h2>
          <p>从视频剧本开始，也可以为租户开启公众号文章。创作留在本地，正式工作交给团队。</p>
          <div><Link className="marketing-primary-button" to={consolePath.dashboard}><Play size={16}/>开始使用</Link><Link className="marketing-secondary-button" to="/docs">查看文档 <ArrowRight size={15}/></Link></div>
        </div>
      </section>
    </main>

    <footer className="marketing-footer">
      <div className="marketing-shell marketing-footer-grid">
        <div><a className="marketing-brand is-footer" href="#overview"><BrandMark className="marketing-brand-mark"/><span><strong>Content Work OS</strong><small>Content operations</small></span></a><p>本地 Agent 创作，云端治理，批准后交付。</p></div>
        <nav aria-label="产品链接"><strong>产品</strong><a href="#product">工作系统</a><a href="#content-packs">内容形态</a><a href="#governance">治理</a></nav>
        <nav aria-label="资源链接"><strong>资源</strong><Link to="/docs">使用文档</Link><Link to="/docs/clients/codex">Codex 接入</Link><Link to="/docs/content/wechat-article">公众号文章</Link></nav>
        <nav aria-label="账户链接"><strong>账户</strong><Link to="/login">登录</Link><Link to="/register">注册</Link><Link to={consolePath.dashboard}>工作台</Link></nav>
      </div>
      <div className="marketing-shell marketing-footer-bottom"><span>© {new Date().getFullYear()} Content Work OS</span><span>LOCAL-FIRST · GOVERNED · TRACEABLE</span></div>
    </footer>
  </div>;
}
