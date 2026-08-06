import { BookOpen, ChevronRight, CircleHelp, Clock3, ExternalLink, FileText, House, Menu, MonitorUp, Video, X } from 'lucide-react';
import { createContext, useContext, useEffect, useMemo, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Link, NavLink, Outlet } from 'react-router-dom';
import { Banner, IconButton, Loading } from '../components/ui';
import { BrandLockup } from '../components/Brand';
import { consolePath } from '../consoleRoutes';
import { docsStatusLabel, docsWebPath, loadDocsCatalog, loadDocsPage, resolveMarkdownHref, type DocsCatalog, type DocsPage, type DocsStatus } from './docs';
import './docs.css';

const DocsContext=createContext<DocsCatalog|undefined>(undefined);

export function DocsShell(){
  const [catalog,setCatalog]=useState<DocsCatalog>();
  const [error,setError]=useState('');
  const [mobileOpen,setMobileOpen]=useState(false);
  const load=()=>{setError('');loadDocsCatalog().then(setCatalog).catch(value=>setError(value instanceof Error?value.message:'文档目录加载失败'))};
  useEffect(()=>{load()},[]);
  useEffect(()=>{
    const previousTitle=document.title;document.title='Content Work OS 使用文档';
    let robots=document.head.querySelector<HTMLMetaElement>('meta[name="robots"]');
    const created=!robots;if(!robots){robots=document.createElement('meta');robots.name='robots';document.head.appendChild(robots)}
    const previous=robots.content;robots.content='noindex, nofollow';
    return()=>{document.title=previousTitle;if(created)robots?.remove();else if(robots)robots.content=previous};
  },[]);
  return <DocsContext.Provider value={catalog}><div className="docs-shell">
    <header className="docs-topbar"><Link className="docs-brand" to="/"><BrandLockup subtitle="使用文档"/></Link><nav><Link to="/docs"><House size={16}/>文档首页</Link><Link to={consolePath.dashboard}><MonitorUp size={16}/>工作台</Link><IconButton className="docs-menu-button" label="打开文档导航" onClick={()=>setMobileOpen(true)}><Menu size={19}/></IconButton></nav></header>
    <div className="docs-layout">
      <aside className={`docs-sidebar ${mobileOpen?'is-open':''}`}><div className="docs-sidebar-mobile"><strong>文档导航</strong><IconButton label="关闭文档导航" onClick={()=>setMobileOpen(false)}><X size={18}/></IconButton></div>{catalog?<DocsNavigation catalog={catalog} onNavigate={()=>setMobileOpen(false)}/>:<div className="docs-sidebar-loading"><Loading/></div>}</aside>
      {mobileOpen&&<button className="docs-scrim" aria-label="关闭文档导航" onClick={()=>setMobileOpen(false)}/>}
      <main className="docs-main">{error?<div className="docs-state"><Banner kind="error">{error}</Banner><button className="button button-secondary" onClick={load}>重试</button></div>:!catalog?<div className="docs-state"><Loading/></div>:<Outlet/>}</main>
    </div>
  </div></DocsContext.Provider>;
}

function DocsNavigation({catalog,onNavigate}:{catalog:DocsCatalog;onNavigate:()=>void}){
  return <nav aria-label="文档目录">
    <NavLink end to="/docs" onClick={onNavigate}><BookOpen size={16}/>概览</NavLink>
    {catalog.sections.map(section=><section key={section.id}><span>{section.title}</span>{section.pages.map(page=><DocNavLink key={page.slug} page={page} onClick={onNavigate}/>)}</section>)}
    <section><span>客户端</span>{catalog.clients.map(client=><NavLink key={client.id} to={docsWebPath(client.page_slug)!} onClick={onNavigate}>{client.display_name}<StatusDot status={client.status}/></NavLink>)}</section>
    <section><span>内容形态</span>{catalog.content_types.map(content=><NavLink key={content.id} to={docsWebPath(content.page_slug)!} onClick={onNavigate}>{content.title}<StatusDot status={content.status}/></NavLink>)}</section>
    <section><span>场景指南</span>{catalog.guides.map(guide=><NavLink key={guide.id} to={docsWebPath(guide.page_slug)!} onClick={onNavigate}>{guide.title}</NavLink>)}</section>
  </nav>;
}

function DocNavLink({page,onClick}:{page:DocsCatalog['pages'][number];onClick:()=>void}){return <NavLink to={docsWebPath(page.slug)!} onClick={onClick}>{page.title}</NavLink>}
function StatusDot({status}:{status:DocsStatus}){return <i className={`docs-status-dot is-${status}`} title={docsStatusLabel(status)} aria-label={docsStatusLabel(status)}/>}

export function DocsHome(){
  const catalog=useDocsCatalog();
  useEffect(()=>{document.title='Content Work OS 使用文档'},[]);
  return <div className="docs-home">
    <header className="docs-home-heading"><span className="eyebrow">使用文档</span><h1>{catalog.home.title}</h1><p>{catalog.home.description}</p><div><Link className="button button-primary" to={docsWebPath('getting-started')!}>开始使用<ChevronRight size={16}/></Link><Link className="button button-secondary" to={docsWebPath('concepts/governed-workflow')!}>了解工作流</Link></div></header>
    <section className="docs-index-section"><header><div><span className="section-kicker">智能客户端</span><h2>按客户端查看</h2></div><p>能力状态直接来自智能客户端目录。</p></header><div className="docs-client-index">{catalog.clients.map(client=><Link key={client.id} to={docsWebPath(client.page_slug)!}><span className="docs-index-icon"><MonitorUp size={18}/></span><span><strong>{client.display_name}</strong><small>{client.summary}</small></span><b className={`docs-status is-${client.status}`}>{docsStatusLabel(client.status)}</b><ChevronRight size={17}/></Link>)}</div></section>
    <section className="docs-index-section"><header><div><span className="section-kicker">内容类型</span><h2>按内容形态查看</h2></div><p>只有完整实现的组合才提供可执行的场景教程。</p></header><div className="docs-content-index">{catalog.content_types.map(content=><Link key={content.id} to={docsWebPath(content.page_slug)!}><span className="docs-index-icon">{content.id==='marketing-video'?<Video size={19}/>:<FileText size={19}/>}</span><span><strong>{content.title}</strong><small>{content.summary}</small></span><b className={`docs-status is-${content.status}`}>{docsStatusLabel(content.status)}</b><ChevronRight size={17}/></Link>)}</div></section>
    <section className="docs-index-section docs-guide-section"><header><div><span className="section-kicker">可用指南</span><h2>当前可用场景</h2></div></header>{catalog.guides.map(guide=><Link key={guide.id} to={docsWebPath(guide.page_slug)!}><BookOpen size={18}/><span><strong>{guide.title}</strong><small>{guide.summary}</small></span><ChevronRight size={17}/></Link>)}</section>
  </div>;
}

export function DocsPageView({slug}:{slug:string}){
  const catalog=useDocsCatalog();
  const [page,setPage]=useState<DocsPage>();const [error,setError]=useState('');const [reload,setReload]=useState(0);
  useEffect(()=>{let active=true;setPage(undefined);setError('');loadDocsPage(slug).then(value=>{if(active)setPage(value)}).catch(value=>{if(active)setError(value instanceof Error?value.message:'文档页面加载失败')});return()=>{active=false}},[slug,reload]);
  useEffect(()=>{if(page)document.title=`${page.title} · Content Work OS 文档`},[page]);
  const components=useMemo(()=>({
    a:({href='',children}:{href?:string;children?:React.ReactNode})=>{
      const resolved=resolveMarkdownHref(href,slug);
      if(!resolved)return <span>{children}</span>;
      return resolved.external?<a href={resolved.href} target="_blank" rel="noreferrer">{children}<ExternalLink size={13}/></a>:<Link to={resolved.href}>{children}</Link>;
    },
    img:()=>null,
  }),[slug]);
  if(error)return <div className="docs-state"><Banner kind="error">{error}</Banner><button className="button button-secondary" onClick={()=>setReload(value=>value+1)}>重试</button></div>;
  if(!page)return <div className="docs-state"><Loading/></div>;
  return <div className="docs-page-layout"><article className="docs-article"><header className="docs-article-header"><Link to="/docs"><CircleHelp size={14}/>使用文档</Link><span>/</span><span>{kindLabel(page.kind)}</span><b className={`docs-status is-${page.status}`}>{docsStatusLabel(page.status)}</b></header>{page.status!=='available'&&<aside className={`docs-availability is-${page.status}`}>{page.status==='limited'?<CircleHelp size={19}/>:<Clock3 size={19}/>}<div><strong>{docsStatusLabel(page.status)}</strong><span>{page.status==='limited'?'部分底层能力可用，请以页面列出的边界为准。':'该能力已预留兼容入口，但尚无可执行实现。'}</span></div></aside>}<div className="docs-markdown"><ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml components={components}>{page.markdown}</ReactMarkdown></div></article><aside className="docs-page-meta"><span>页面状态</span><strong>{docsStatusLabel(page.status)}</strong><p>{page.description}</p><code>{page.slug}</code></aside></div>;
}

function useDocsCatalog(){const value=useContext(DocsContext);if(!value)throw new Error('文档目录尚未加载');return value}
function kindLabel(kind:string){return kind==='client'?'客户端':kind==='content_type'?'内容形态':kind==='guide'?'场景指南':kind==='troubleshooting'?'故障排查':kind==='concept'?'概念':'指南'}
