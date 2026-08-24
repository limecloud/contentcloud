import { Archive, BookOpen, BriefcaseBusiness, ChevronDown, ClipboardCheck, Layers3, LayoutDashboard, ListTodo, LogOut, Menu, MonitorUp, PackageCheck, Sparkles, X } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { Link, Navigate, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { BrandLockup, BrandMark } from '../components/Brand';
import { IconButton } from '../components/ui';
import { ALL_EXPERIENCES, useStudio } from './StudioContext';
import { scopedStudioProjects } from './studioScope';
import './studio.css';

const navItems=[
  {to:'/studio',end:true,label:'今天',icon:Sparkles},
  {to:'/studio/connect',end:true,label:'连接工作电脑',icon:MonitorUp},
  {to:'/studio/tasks',end:false,label:'我的任务',icon:ListTodo},
  {to:'/studio/assets',end:false,label:'我的资料',icon:Archive},
  {to:'/studio/knowledge',end:false,label:'知识库',icon:BookOpen},
  {to:'/studio/deliveries',end:false,label:'交付',icon:PackageCheck},
] as const;

function businessTypeLabel(value:string):string {
  const labels:Record<string,string>={marketing_video:'营销视频',video_script:'视频脚本',article:'文章',wechat_article:'公众号文章',article_content:'文章',commerce:'电商内容',ecommerce:'电商内容',douyin_commerce:'抖音电商',product_content:'商品内容',serialized_novel:'连载小说'};
  return labels[value.trim().toLowerCase()]||value||'内容业务';
}

export function CustomerStudioShell(){
  const {bootstrap,switchTenant,selectedExperienceID,selectExperience,logout}=useStudio();
  const {session,tenants}=bootstrap;
  const navigate=useNavigate();
  const location=useLocation();
  const [mobileOpen,setMobileOpen]=useState(false);
  const [accountOpen,setAccountOpen]=useState(false);
  const [businessOpen,setBusinessOpen]=useState(false);
  const businessMenuRef=useRef<HTMLDivElement>(null);
  const canOperate=session.can_view_operations&&Boolean(session.operations_path);
  const signOut=async()=>{await logout();navigate('/login',{replace:true})};
  const selectedExperience=bootstrap.experiences.find(item=>item.id===selectedExperienceID);
  const activeProjects=scopedStudioProjects(bootstrap.projects,selectedExperienceID,selectedExperience);
  const needsConnection=session.can_create&&activeProjects.length>0&&activeProjects.every(project=>!project.execution_client_connected);
  const businessLabel=selectedExperience?.name||(selectedExperienceID===ALL_EXPERIENCES?'全部业务':'未开通业务');
  const chooseExperience=(experienceID:string)=>{
    selectExperience(experienceID);
    setBusinessOpen(false);
    navigate('/studio');
  };
  useEffect(()=>{
    if(!businessOpen)return;
    const closeOnOutside=(event:PointerEvent)=>{if(!businessMenuRef.current?.contains(event.target as Node))setBusinessOpen(false)};
    const closeOnEscape=(event:KeyboardEvent)=>{if(event.key==='Escape')setBusinessOpen(false)};
    document.addEventListener('pointerdown',closeOnOutside);
    document.addEventListener('keydown',closeOnEscape);
    return()=>{document.removeEventListener('pointerdown',closeOnOutside);document.removeEventListener('keydown',closeOnEscape)};
  },[businessOpen]);
  if(location.pathname==='/studio'&&needsConnection)return <Navigate to="/studio/connect" replace/>;
  return <div className="studio-shell">
    <header className="studio-mobile-header"><BrandMark/><strong>创作台</strong><IconButton label={mobileOpen?'关闭导航':'打开导航'} onClick={()=>setMobileOpen(value=>!value)}>{mobileOpen?<X size={20}/>:<Menu size={20}/>}</IconButton></header>
    <aside className={`studio-sidebar ${mobileOpen?'is-open':''}`}>
      <div className="studio-brand"><BrandLockup subtitle="客户创作台"/></div>
      <nav aria-label="客户创作台导航">{navItems.map(({to,end,label,icon:Icon})=><NavLink key={to} to={to} end={end} onClick={()=>setMobileOpen(false)} className={({isActive})=>isActive?'is-active':''}><Icon size={18}/><span>{label}</span></NavLink>)}</nav>
      {canOperate&&<Link className="studio-operations-link" to={session.operations_path||'/admin/dashboard'}><LayoutDashboard size={16}/><span>运营与管理</span></Link>}
      <div className="studio-sidebar-footer"><span>{session.user.display_name.slice(0,1).toUpperCase()}</span><div><strong>{session.user.display_name}</strong><small>{session.tenant.name}</small></div><IconButton label="退出登录" onClick={signOut}><LogOut size={16}/></IconButton></div>
    </aside>
    {mobileOpen&&<button className="studio-scrim" type="button" aria-label="关闭导航" onClick={()=>setMobileOpen(false)}/>}
    <main className="studio-main">
      <header className="studio-topbar"><div className="studio-topbar-context"><label><span>当前客户</span><select value={session.tenant.id} onChange={async event=>{if(await switchTenant(event.target.value))navigate('/studio')}}>{tenants.map(tenant=><option key={tenant.id} value={tenant.id}>{tenant.name}</option>)}</select><ChevronDown size={14}/></label><div className="studio-business-switcher" ref={businessMenuRef}><button className="studio-business-trigger" type="button" onClick={()=>setBusinessOpen(value=>!value)} aria-expanded={businessOpen} aria-haspopup="menu" aria-controls="studio-business-menu"><BriefcaseBusiness size={15}/><span><small>当前业务</small><strong>{businessLabel}</strong></span><ChevronDown size={14}/></button>{businessOpen&&<div className="studio-business-menu" id="studio-business-menu" role="menu" aria-label="选择业务"><button type="button" className={selectedExperienceID===ALL_EXPERIENCES?'is-selected':''} onClick={()=>chooseExperience(ALL_EXPERIENCES)} role="menuitem"><Layers3 size={16}/><span><strong>全部业务</strong><small>查看当前客户所有已开通的工作台</small></span>{selectedExperienceID===ALL_EXPERIENCES&&<span className="studio-menu-check">当前</span>}</button>{bootstrap.experiences.map(experience=><button type="button" className={experience.id===selectedExperienceID?'is-selected':''} key={experience.id} onClick={()=>chooseExperience(experience.id)} role="menuitem"><BriefcaseBusiness size={16}/><span><strong>{experience.name}</strong><small>{businessTypeLabel(experience.content_type)} · {experience.project_ids.length} 个项目</small></span>{experience.id===selectedExperienceID&&<span className="studio-menu-check">当前</span>}</button>)}{bootstrap.experiences.length===0&&<div className="studio-business-empty"><Layers3 size={16}/><span><strong>还没有可用业务</strong><small>请联系管理员开通内容类型、流程和工作台。</small></span></div>}</div>}</div></div><div className="studio-topbar-note"><ClipboardCheck size={16}/><span>确认后会保留当前成果</span></div><button type="button" onClick={()=>setAccountOpen(value=>!value)} aria-expanded={accountOpen}><span>{session.user.display_name.slice(0,1).toUpperCase()}</span><strong>{session.user.display_name}</strong><ChevronDown size={14}/></button>{accountOpen&&<div className="studio-account-menu">{session.can_manage_team&&<Link to="/studio/team">团队成员</Link>}{canOperate&&<Link to={session.operations_path||'/admin/dashboard'}>运营与管理</Link>}<button type="button" onClick={signOut}>退出登录</button></div>}</header>
      <div className="studio-page"><Outlet/></div>
    </main>
  </div>;
}
