import { CheckCircle2, CloudCog, KeyRound, Plus, Send, RefreshCw, ShieldCheck, Video, WalletCards } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import { Banner, Button, Empty, Status } from '../../components/ui';
import type { ProviderBinding, ProviderProfile } from '../../types';
import { isMissingProviderBinding, providerApi, SEEDANCE_PROVIDER_ID, type ProviderBindingInput } from '../../providerApi';
import { useAdmin } from '../context';

const dateTime = (value:string) => value ? new Intl.DateTimeFormat('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).format(new Date(value)) : '未记录';
const dateInput = (value:Date) => value.toISOString().slice(0,16);

type ProfileDraft = {
  provider_id:string; version:string; digest:string; adapter_version:string; model:string; region:string;
  modes:string; input_media_types:string; output_media_type:string; limits:string; data_retention:string;
  pricing:string; verified_at:string; expires_at:string;
};

type BindingDraft = {
  profile_version:string; state:'active'|'disabled'; credential_ref:string; egress_policy:string;
  monthly_budget_minor:string; max_job_cost_minor:string; max_concurrency:string; max_retries:string;
};

function newProfileDraft():ProfileDraft {
  const now=new Date();
  const expires=new Date(now.getTime()+30*24*60*60*1000);
  return {provider_id:SEEDANCE_PROVIDER_ID,version:'',digest:'',adapter_version:'modelark/1.0.0',model:'dreamina-seedance-2-5-260628',region:'cn-beijing',modes:'text_to_video, image_to_video',input_media_types:'image/png, image/jpeg, application/json',output_media_type:'video/mp4',limits:'{\n  "max_duration_seconds": 12,\n  "max_reference_images": 30\n}',data_retention:'provider_policy',pricing:'{\n  "currency": "CNY",\n  "per_second_minor": 0\n}',verified_at:dateInput(now),expires_at:dateInput(expires)};
}

function bindingDraft(binding:ProviderBinding|undefined, profiles:ProviderProfile[]):BindingDraft {
  const profileVersion=binding?.profile_version||profiles[0]?.version||'';
  return {profile_version:profileVersion,state:binding?.state==='active'?'active':'disabled',credential_ref:'',egress_policy:binding?.egress_policy||'provider-only',monthly_budget_minor:String(binding?.monthly_budget_minor||0),max_job_cost_minor:String(binding?.max_job_cost_minor||0),max_concurrency:String(binding?.max_concurrency||1),max_retries:String(binding?.max_retries||0)};
}

export function AdminProviderPage(){
  const {session,workOS}=useAdmin();
  const tenantOptions=useMemo(()=>Array.from(new Map((workOS?.environments||[]).filter(item=>item.tenant_id).map(item=>[item.tenant_id,{id:item.tenant_id,name:item.name}])).values()),[workOS?.environments]);
  const [targetTenantID,setTargetTenantID]=useState(()=>tenantOptions[0]?.id||session.tenant.id);
  const [profiles,setProfiles]=useState<ProviderProfile[]>([]);
  const [availableProfiles,setAvailableProfiles]=useState<ProviderProfile[]>([]);
  const [binding,setBinding]=useState<ProviderBinding>();
  const [bindingSupported,setBindingSupported]=useState(true);
  const [loading,setLoading]=useState(true);
  const [refreshing,setRefreshing]=useState(false);
  const [error,setError]=useState('');
  const [notice,setNotice]=useState('');
  const [createOpen,setCreateOpen]=useState(false);
  const [profileDraft,setProfileDraft]=useState<ProfileDraft>(newProfileDraft);
  const [bindingForm,setBindingForm]=useState<BindingDraft>(()=>bindingDraft(undefined,[]));
  const [busy,setBusy]=useState('');
  const loadSequence=useRef(0);

  const load=useCallback(async(silent=false)=>{
    const sequence=++loadSequence.current;
    silent?setRefreshing(true):setLoading(true);setError('');
    try{
      const [managed,bindingResult]=await Promise.all([
        session.is_platform_admin?providerApi.adminProfiles():providerApi.availableProfiles(),
        (session.is_platform_admin&&targetTenantID?providerApi.adminBinding(targetTenantID):providerApi.binding()).catch(value=>{if(isMissingProviderBinding(value))return undefined;throw value})
      ]);
      if(sequence!==loadSequence.current)return;
      const profilesValue=managed||[];
      const available= session.is_platform_admin ? profilesValue.filter(isAvailableProfile) : profilesValue;
      setProfiles(profilesValue);setAvailableProfiles(available);setBinding(bindingResult);setBindingSupported(true);setBindingForm(bindingDraft(bindingResult,available));
    }catch(value){
      if(sequence!==loadSequence.current)return;
      if((value as {status?:number}).status===403){setBindingSupported(false)}
      setError(value instanceof Error?value.message:'视频服务配置加载失败');
    }finally{if(sequence===loadSequence.current){setLoading(false);setRefreshing(false)}}
  },[session.is_platform_admin,targetTenantID]);
  useEffect(()=>{void load()},[load]);
  useEffect(()=>{if(session.is_platform_admin&&tenantOptions.length&&!tenantOptions.some(item=>item.id===targetTenantID))setTargetTenantID(tenantOptions[0].id)},[session.is_platform_admin,targetTenantID,tenantOptions]);

  const publish=async(profile:ProviderProfile)=>{
    setBusy(`publish:${profile.version}`);setError('');setNotice('');
    try{await providerApi.publishProfile(profile.provider_id,profile.version);setNotice(`服务版本 ${profile.version} 已发布，客户可以连接使用。`);await load(true)}
    catch(value){setError(value instanceof Error?value.message:'服务版本发布失败')}finally{setBusy('')}
  };
  const create=async(event:FormEvent)=>{
    event.preventDefault();setBusy('create');setError('');setNotice('');
    try{
      const limits=parseJSON(profileDraft.limits,'限制');const pricing=parseJSON(profileDraft.pricing,'价格');
      await providerApi.createProfile({provider_id:profileDraft.provider_id.trim(),version:profileDraft.version.trim(),digest:profileDraft.digest.trim(),adapter_version:profileDraft.adapter_version.trim(),model:profileDraft.model.trim(),region:profileDraft.region.trim(),modes:splitList(profileDraft.modes),input_media_types:splitList(profileDraft.input_media_types),output_media_type:profileDraft.output_media_type.trim(),limits,data_retention:profileDraft.data_retention.trim(),pricing,verified_at:new Date(profileDraft.verified_at).toISOString(),expires_at:new Date(profileDraft.expires_at).toISOString()});
      setCreateOpen(false);setProfileDraft(newProfileDraft());setNotice(`服务版本 ${profileDraft.version} 已创建为草稿。`);await load(true);
    }catch(value){setError(value instanceof Error?value.message:'服务版本创建失败')}finally{setBusy('')}
  };
  const saveBinding=async(event:FormEvent)=>{
    event.preventDefault();
    if(!bindingForm.profile_version){setError('请先选择已发布的服务版本');return}
    setBusy('binding');setError('');setNotice('');
    try{
      const input:ProviderBindingInput={profile_version:bindingForm.profile_version,state:bindingForm.state,egress_policy:bindingForm.egress_policy.trim(),monthly_budget_minor:toNonNegativeInt(bindingForm.monthly_budget_minor),max_job_cost_minor:toNonNegativeInt(bindingForm.max_job_cost_minor),max_concurrency:Math.max(1,toNonNegativeInt(bindingForm.max_concurrency)||1),max_retries:toNonNegativeInt(bindingForm.max_retries)};
      if(bindingForm.credential_ref.trim())input.credential_ref=bindingForm.credential_ref.trim();
      const saved=session.is_platform_admin&&targetTenantID?await providerApi.saveAdminBinding(targetTenantID,input):await providerApi.saveBinding(input);setBinding(saved);setBindingForm(bindingDraft(saved,availableProfiles));setNotice('客户连接设置已保存，后续任务会使用平台检查后的设置。');
    }catch(value){setError(value instanceof Error?value.message:'客户连接设置保存失败')}finally{setBusy('')}
  };

  const publishedCount=profiles.filter(item=>item.status==='published').length;
  const bindingProfile=availableProfiles.find(item=>item.version===bindingForm.profile_version);
  const isConfigured=Boolean(binding?.credential_configured)||(binding?.provider_id==='fake');
  const profileOptions=availableProfiles;

  if(loading)return <div className="operations-page"><ProviderIntro platformAdmin={session.is_platform_admin}/><section className="operations-section"><div className="admin-loading"><RefreshCw className="is-spinning" size={18}/><span>正在读取视频服务配置…</span></div></section></div>;
  return <div className="operations-page provider-page">
    <ProviderIntro platformAdmin={session.is_platform_admin} action={<Button variant="secondary" disabled={refreshing} onClick={()=>void load(true)}><RefreshCw className={refreshing?'is-spinning':''} size={15}/>刷新</Button>}/>
    {error&&<Banner kind="error" onClose={()=>setError('')}>{error}</Banner>}{notice&&<Banner kind="success" onClose={()=>setNotice('')}>{notice}</Banner>}
    {session.is_platform_admin&&tenantOptions.length>0&&<label className="provider-tenant-select"><span>正在管理的客户</span><select value={targetTenantID} onChange={event=>setTargetTenantID(event.target.value)}>{tenantOptions.map(item=><option key={item.id} value={item.id}>{item.name} · {item.id}</option>)}</select></label>}
    <section className="provider-summary" aria-label="Seedance 视频服务状态"><SummaryItem icon={<Video size={17}/>} label="服务名称" value={SEEDANCE_PROVIDER_ID}/><SummaryItem icon={<CloudCog size={17}/>} label="已发布版本" value={`${publishedCount} 个`}/><SummaryItem icon={<WalletCards size={17}/>} label="客户连接" value={binding?binding.state==='active'?'已启用':providerStateLabel(binding.state):'未配置'}/><SummaryItem icon={<KeyRound size={17}/>} label="服务密钥" value={binding?isConfigured?'已配置':'未配置':'未配置'}/></section>
    <div className="provider-layout">
      <section className="operations-section provider-profiles"><SectionTitle kicker="平台能力" title={session.is_platform_admin?`服务版本（${profiles.length}）`:'可用服务版本'} action={session.is_platform_admin?<Button onClick={()=>setCreateOpen(value=>!value)}><Plus size={15}/>新建版本</Button>:<span className="operations-muted">仅显示已发布且未过期版本</span>}/>{profiles.length===0?<Empty title="还没有可用版本" detail={session.is_platform_admin?'先创建一个待核验的服务版本草稿。':'平台尚未发布可连接的 Seedance 服务版本。'}/>:<div className="provider-profile-list">{profiles.map(profile=><ProfileRow key={`${profile.provider_id}:${profile.version}`} profile={profile} canPublish={session.is_platform_admin&&profile.status==='draft'} busy={busy===`publish:${profile.version}`} onPublish={()=>void publish(profile)}/>)}</div>}</section>
      {bindingSupported&&<BindingPanel form={bindingForm} setForm={setBindingForm} profileOptions={profileOptions} selectedProfile={bindingProfile} binding={binding} isConfigured={isConfigured} busy={busy==='binding'} onSave={saveBinding}/>}
    </div>
    {session.is_platform_admin&&createOpen&&<ProfileCreatePanel draft={profileDraft} setDraft={setProfileDraft} busy={busy==='create'} onSubmit={create} onCancel={()=>setCreateOpen(false)}/>}
  </div>;
}

function ProviderIntro({platformAdmin,action}:{platformAdmin:boolean;action?:React.ReactNode}){return <header className="operations-page-intro"><div><span className="operations-eyebrow">功能设置 / 视频制作</span><h1>Seedance 视频制作</h1><p>{platformAdmin?'维护视频制作服务的版本、发布状态和客户连接设置。':'查看当前客户可以使用的视频制作服务，并维护连接设置。'}</p></div>{action&&<div className="operations-page-actions">{action}</div>}</header>}
function SectionTitle({kicker,title,action}:{kicker:string;title:string;action?:React.ReactNode}){return <header className="operations-section-title"><div><span>{kicker}</span><h2>{title}</h2></div>{action}</header>}
function SummaryItem({icon,label,value}:{icon:React.ReactNode;label:string;value:string}){return <div className="provider-summary-item"><span>{icon}</span><div><small>{label}</small><strong>{value}</strong></div></div>}
function ProfileRow({profile,canPublish,busy,onPublish}:{profile:ProviderProfile;canPublish:boolean;busy:boolean;onPublish:()=>void}){return <article className="provider-profile-row"><header><div className="provider-profile-title"><span className="provider-icon"><CloudCog size={16}/></span><div><strong>{profile.provider_id}</strong><small>v{profile.version} · {profile.model}</small></div></div><Status value={profile.status}/></header><dl><div><dt>支持方式</dt><dd>{profile.modes.map(mode=>mode==='text_to_video'?'文生视频':mode==='image_to_video'?'图生视频':mode).join('、')}</dd></div><div><dt>服务所在区域 / 连接工具版本</dt><dd>{profile.region} · {profile.adapter_version}</dd></div><div><dt>使用限制</dt><dd>{limitSummary(profile.limits)}</dd></div><div><dt>费用</dt><dd>{priceSummary(profile.pricing)}</dd></div><div><dt>有效期</dt><dd>{dateTime(profile.expires_at)}</dd></div><div><dt>版本记录</dt><dd><code>{profile.digest}</code></dd></div></dl>{canPublish&&<footer><span>已核验于 {dateTime(profile.verified_at)}，发布后客户才能使用。</span><Button variant="secondary" disabled={busy} onClick={onPublish}><Send size={14}/>{busy?'发布中…':'发布版本'}</Button></footer>}</article>}

function BindingPanel({form,setForm,profileOptions,selectedProfile,binding,isConfigured,busy,onSave}:{form:BindingDraft;setForm:React.Dispatch<React.SetStateAction<BindingDraft>>;profileOptions:ProviderProfile[];selectedProfile?:ProviderProfile;binding?:ProviderBinding;isConfigured:boolean;busy:boolean;onSave:(event:FormEvent)=>void}){return <section className="operations-section provider-binding"><SectionTitle kicker="客户设置" title="客户连接设置" action={binding?<Status value={binding.state}/>:<span className="operations-muted">尚未保存</span>}/><form onSubmit={onSave} className="provider-form"><div className="provider-form-grid"><label className="provider-field"><span>服务版本</span><select value={form.profile_version} onChange={event=>setForm(value=>({...value,profile_version:event.target.value}))} disabled={!profileOptions.length}>{profileOptions.length===0?<option value="">暂无已发布版本</option>:profileOptions.map(profile=><option value={profile.version} key={profile.version}>v{profile.version} · {profile.model}</option>)}</select></label><label className="provider-field"><span>状态</span><select value={form.state} onChange={event=>setForm(value=>({...value,state:event.target.value as BindingDraft['state']}))}><option value="active">启用</option><option value="disabled">停用</option></select></label><label className="provider-field"><span>网络访问方式</span><select value={form.egress_policy} onChange={event=>setForm(value=>({...value,egress_policy:event.target.value}))}><option value="provider-only">仅视频服务出口</option><option value="public">允许公共网络</option><option value="private">私有网络</option></select></label><label className="provider-field"><span>服务密钥引用</span><input type="password" value={form.credential_ref} onChange={event=>setForm(value=>({...value,credential_ref:event.target.value}))} placeholder={isConfigured?'已保存，留空以继续使用':'secret://providers/modelark-seedance25'} autoComplete="new-password"/><small>填写已保存的服务密钥地址，不要直接填写密钥原文；平台不会显示原文。</small></label><label className="provider-field"><span>每月预算（分）</span><input type="number" min="0" value={form.monthly_budget_minor} onChange={event=>setForm(value=>({...value,monthly_budget_minor:event.target.value}))}/></label><label className="provider-field"><span>单次任务上限（分）</span><input type="number" min="0" value={form.max_job_cost_minor} onChange={event=>setForm(value=>({...value,max_job_cost_minor:event.target.value}))}/></label><label className="provider-field"><span>同时处理上限</span><input type="number" min="1" value={form.max_concurrency} onChange={event=>setForm(value=>({...value,max_concurrency:event.target.value}))}/></label><label className="provider-field"><span>失败后最多重试</span><input type="number" min="0" value={form.max_retries} onChange={event=>setForm(value=>({...value,max_retries:event.target.value}))}/></label></div><div className="provider-binding-review"><span><small>当前视频模型</small><strong>{selectedProfile?.model||'未选择'}</strong></span><span><small>支持方式</small><strong>{selectedProfile?.modes.map(mode=>mode==='text_to_video'?'文生视频':mode==='image_to_video'?'图生视频':mode).join('、')||'未返回'}</strong></span><span><small>服务密钥</small><strong>{isConfigured?'平台已配置':'需要配置服务密钥'}</strong></span></div><div className="provider-form-actions"><span><ShieldCheck size={15}/>状态、预算、服务版本和服务密钥会由平台最终检查。</span><Button type="submit" disabled={busy||!profileOptions.length}><CheckCircle2 size={15}/>{busy?'保存中…':'保存连接设置'}</Button></div></form></section>}

function ProfileCreatePanel({draft,setDraft,busy,onSubmit,onCancel}:{draft:ProfileDraft;setDraft:React.Dispatch<React.SetStateAction<ProfileDraft>>;busy:boolean;onSubmit:(event:FormEvent)=>void;onCancel:()=>void}){const update=(key:keyof ProfileDraft)=>(event:ChangeEvent<HTMLInputElement|HTMLTextAreaElement>)=>setDraft(value=>({...value,[key]:event.target.value}));return <section className="operations-section provider-create"><SectionTitle kicker="平台配置" title="新建服务版本草稿" action={<Button variant="ghost" onClick={onCancel}>取消</Button>}/><form className="provider-form" onSubmit={onSubmit}><div className="provider-form-grid provider-form-grid-wide"><label className="provider-field"><span>服务名称（系统标识）</span><input value={draft.provider_id} onChange={update('provider_id')} required/></label><label className="provider-field"><span>版本</span><input value={draft.version} onChange={update('version')} placeholder="例如 1.0.0" required/></label><label className="provider-field provider-field-wide"><span>版本记录</span><input value={draft.digest} onChange={update('digest')} placeholder="sha256:..." required/></label><label className="provider-field"><span>连接工具版本</span><input value={draft.adapter_version} onChange={update('adapter_version')} required/></label><label className="provider-field"><span>视频模型</span><input value={draft.model} onChange={update('model')} required/></label><label className="provider-field"><span>服务所在区域</span><input value={draft.region} onChange={update('region')} required/></label><label className="provider-field"><span>输出视频格式</span><input value={draft.output_media_type} onChange={update('output_media_type')} required/></label><label className="provider-field provider-field-wide"><span>支持的制作方式（用逗号分隔）</span><input value={draft.modes} onChange={update('modes')} required/></label><label className="provider-field provider-field-wide"><span>可接收的素材类型（用逗号分隔）</span><input value={draft.input_media_types} onChange={update('input_media_types')} required/></label><label className="provider-field"><span>数据保留方式</span><input value={draft.data_retention} onChange={update('data_retention')} required/></label><label className="provider-field"><span>核验时间</span><input type="datetime-local" value={draft.verified_at} onChange={update('verified_at')} required/></label><label className="provider-field"><span>失效时间</span><input type="datetime-local" value={draft.expires_at} onChange={update('expires_at')} required/></label><label className="provider-field"><span>使用限制</span><textarea value={draft.limits} onChange={update('limits')} rows={4} required/></label><label className="provider-field"><span>费用信息</span><textarea value={draft.pricing} onChange={update('pricing')} rows={4} required/></label></div><div className="provider-form-actions"><span>服务版本本身不包含服务密钥；创建后仍需核验并发布。</span><Button type="submit" disabled={busy}><Plus size={15}/>{busy?'创建中…':'创建草稿'}</Button></div></form></section>}

function parseJSON(value:string,label:string):Record<string,unknown>{try{const parsed=JSON.parse(value);if(!parsed||Array.isArray(parsed)||typeof parsed!=='object')throw new Error('invalid object');return parsed as Record<string,unknown>}catch{throw new Error(`${label}配置格式不正确，请按示例检查。`)}}
function splitList(value:string){return value.split(',').map(item=>item.trim()).filter(Boolean)}
function toNonNegativeInt(value:string){const parsed=Number(value);return Number.isFinite(parsed)&&parsed>=0?Math.floor(parsed):0}
function providerStateLabel(value:string){return ({active:'已启用',disabled:'已停用',misconfigured:'配置有误',budget_blocked:'预算阻断'} as Record<string,string>)[value]||value}
function isAvailableProfile(profile:ProviderProfile):boolean{
  if(profile.status!=='published')return false;
  const verifiedAt=Date.parse(profile.verified_at);const expiresAt=Date.parse(profile.expires_at);const now=Date.now();
  return Number.isFinite(verifiedAt)&&Number.isFinite(expiresAt)&&verifiedAt<=now&&expiresAt>now;
}
function limitSummary(value:Record<string,unknown>){const entries=Object.entries(value||{}).slice(0,3);return entries.length?entries.map(([key,item])=>`${key.replace(/_/g,' ')}=${String(item)}`).join(' · '):'未声明'}
function priceSummary(value:Record<string,unknown>){const currency=String(value?.currency||'');const perSecond=value?.per_second_minor;const perJob=value?.per_job_minor;if(perSecond!==undefined)return `${currency} ${Number(perSecond)/100}/秒`;if(perJob!==undefined)return `${currency} ${Number(perJob)/100}/任务`;return currency||'按平台规则计费'}
