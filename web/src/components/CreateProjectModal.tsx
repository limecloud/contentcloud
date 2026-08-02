import { Plus, Save } from 'lucide-react';
import { useEffect, useState } from 'react';
import { api, post } from '../api';
import type { Project, ProjectTemplate } from '../types';
import { Button, Field, Modal } from './ui';

export function CreateProjectModal({role,onClose,onCreated}:{role:string;onClose:()=>void;onCreated:(project:Project)=>void}) {
  const [form,setForm]=useState({template_id:'',brand_name:'',product_name:'',channel:'douyin',stage_objective:'',owner_name:'',reviewer_name:'',client_approver:''});
  const [templates,setTemplates]=useState<ProjectTemplate[]>([]);
  const [templateLoading,setTemplateLoading]=useState(true);
  const [templateError,setTemplateError]=useState('');
  const [templateOpen,setTemplateOpen]=useState(false);
  const [template,setTemplate]=useState({name:'',channel:'douyin',stage_objective:''});
  const [busy,setBusy]=useState('');const [error,setError]=useState('');
  const set=(key:string,value:string)=>setForm(prev=>({...prev,[key]:value}));
  const loadTemplates=async()=>{
    setTemplateLoading(true);
    setTemplateError('');
    try { setTemplates(await api<ProjectTemplate[]>('/api/bff/project-templates')); }
    catch (value) { setTemplates([]); setTemplateError(value instanceof Error ? value.message : '模板加载失败'); }
    finally { setTemplateLoading(false); }
  };
  useEffect(()=>{loadTemplates()},[]);
  const selectTemplate=(id:string)=>{const selected=templates.find(item=>item.id===id);setForm(prev=>({...prev,template_id:id,channel:selected?.channel||prev.channel,stage_objective:selected?.stage_objective||prev.stage_objective}))};
  const createTemplate=async()=>{setBusy('template');setError('');try{const created=await post<ProjectTemplate>('/api/bff/project-templates',template);setTemplates(prev=>[...prev,created]);setTemplateOpen(false);setTemplate({name:'',channel:'douyin',stage_objective:''});setForm(prev=>({...prev,template_id:created.id,channel:created.channel,stage_objective:created.stage_objective}))}catch(e){setError(message(e,'模板创建失败'))}finally{setBusy('')}};
  const submit=async()=>{setBusy('project');setError('');try{const project=await post<Project>('/api/bff/projects',form);onCreated(project)}catch(e){setError(message(e,'创建失败'))}finally{setBusy('')}};
  return <Modal title="新建品牌项目" onClose={onClose}>
    <div className="project-template-picker"><Field label="项目模板" hint={templateLoading?'正在读取默认模板…':templateError?`模板加载失败：${templateError}`:templates.length?'默认模板可直接套用，品牌和阶段目标仍可覆盖。':'当前租户还没有可用模板。'}><select value={form.template_id} disabled={templateLoading} onChange={e=>selectTemplate(e.target.value)}><option value="">{templateLoading?'正在读取模板…':'不使用模板'}</option>{templates.map(item=><option key={item.id} value={item.id}>{item.name}</option>)}</select></Field>{role==='tenant_admin'&&<Button variant="secondary" onClick={()=>setTemplateOpen(value=>!value)}><Plus size={15}/>{templateOpen?'收起':'新建模板'}</Button>}</div>
    {templateOpen&&<section className="template-create-inline"><header><strong>新建可复用模板</strong><span>模板只保存渠道和阶段目标，不包含客户事实或素材。</span></header><div className="form-grid"><Field label="模板名称"><input value={template.name} onChange={e=>setTemplate({...template,name:e.target.value})}/></Field><Field label="默认渠道"><select value={template.channel} onChange={e=>setTemplate({...template,channel:e.target.value})}><option value="douyin">抖音</option><option value="xiaohongshu">小红书</option><option value="wechat_channels">视频号</option></select></Field><Field label="默认阶段目标"><input value={template.stage_objective} onChange={e=>setTemplate({...template,stage_objective:e.target.value})}/></Field></div><Button variant="secondary" disabled={!template.name.trim()||busy==='template'} onClick={createTemplate}><Save size={15}/>{busy==='template'?'保存中…':'保存模板'}</Button></section>}
    <div className="form-grid"><Field label="品牌"><input value={form.brand_name} onChange={e=>set('brand_name',e.target.value)} placeholder="金陵古都香" /></Field><Field label="主攻单品"><input value={form.product_name} onChange={e=>set('product_name',e.target.value)} placeholder="古法线香" /></Field><Field label="首发渠道"><select value={form.channel} onChange={e=>set('channel',e.target.value)}><option value="douyin">抖音</option><option value="xiaohongshu">小红书</option><option value="wechat_channels">视频号</option></select></Field><Field label="阶段目标"><input value={form.stage_objective} onChange={e=>set('stage_objective',e.target.value)} placeholder="验证文化内容与收藏转化" /></Field><Field label="项目负责人"><input value={form.owner_name} onChange={e=>set('owner_name',e.target.value)} /></Field><Field label="内部审核人"><input value={form.reviewer_name} onChange={e=>set('reviewer_name',e.target.value)} /></Field><Field label="客户审批方"><input value={form.client_approver} onChange={e=>set('client_approver',e.target.value)} /></Field></div>
    {error&&<p className="form-error">{error}</p>}<footer className="modal-actions"><Button variant="secondary" onClick={onClose}>取消</Button><Button disabled={busy!==''||!form.brand_name.trim()||!form.product_name.trim()} onClick={submit}>{busy==='project'?'创建中…':'创建项目'}</Button></footer>
  </Modal>
}

const message=(error:unknown,fallback:string)=>error instanceof Error?error.message:fallback;
