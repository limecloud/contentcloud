import { AlertTriangle, CheckCircle2, CircleDollarSign, KeyRound, PlugZap, RefreshCw, Video } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import type { ProviderBinding, ProviderProfile } from '../types';
import { isMissingProviderBinding, providerApi } from '../providerApi';

export function StudioProviderStatus(){
  const [profiles,setProfiles]=useState<ProviderProfile[]>([]);
  const [binding,setBinding]=useState<ProviderBinding>();
  const [loading,setLoading]=useState(true);
  const [error,setError]=useState('');
  const loadSequence=useRef(0);
  const load=useCallback(async()=>{
    const sequence=++loadSequence.current;
    setLoading(true);setError('');
    try{
      const [available,nextBinding]=await Promise.all([providerApi.availableProfiles(),providerApi.binding().catch(value=>{if(isMissingProviderBinding(value))return undefined;throw value})]);
      if(sequence!==loadSequence.current)return;
      setProfiles(available||[]);setBinding(nextBinding)
    }catch(value){
      if(sequence!==loadSequence.current)return;
      setError(value instanceof Error?value.message:'视频制作服务暂时无法读取')
    }finally{if(sequence===loadSequence.current)setLoading(false)}
  },[]);
  useEffect(()=>{void load()},[load]);
  if(loading)return <section className="studio-provider-status is-loading" aria-label="视频制作服务状态" role="status" aria-live="polite"><RefreshCw className="is-spinning" size={15}/><span>正在读取视频制作服务状态…</span></section>;
  if(error)return <section className="studio-provider-status is-warning" aria-label="视频制作服务状态" role="alert"><AlertTriangle size={17}/><div><strong>视频制作服务暂时无法读取</strong><span>{error}</span></div><button type="button" onClick={()=>void load()} aria-label="重试读取视频制作服务状态" title="重试"><RefreshCw size={15}/></button></section>;
  const currentProfile=profiles.find(profile=>profile.version===binding?.profile_version);
  const enabled=binding?.state==='active';
  const ready=enabled&&Boolean(currentProfile)&&((binding?.credential_configured??false)||binding?.provider_id==='fake');
  return <section className={`studio-provider-status ${ready?'is-ready':enabled?'is-warning':'is-disabled'}`} aria-label="视频制作服务状态" role="status" aria-live="polite"><div className="studio-provider-status-icon">{ready?<CheckCircle2 size={18}/>:enabled?<AlertTriangle size={18}/>:<PlugZap size={18}/>}</div><div className="studio-provider-status-main"><header><strong>视频制作服务</strong><span>{ready?'已准备好':enabled?'需要补充设置':'尚未启用'}</span></header><p>{currentProfile?`当前服务版本 v${currentProfile.version} · ${modeLabel(currentProfile.modes)} · 服务已连接`:'暂无可使用的视频制作服务'}</p></div><div className="studio-provider-status-facts"><span><Video size={14}/><b>{currentProfile?modeLabel(currentProfile.modes):'未设置'}</b></span><span><CircleDollarSign size={14}/><b>{binding?(binding.monthly_budget_minor>0?`每月上限 ¥${(binding.monthly_budget_minor/100).toFixed(2)}`:'未设每月上限'):'未设预算'}</b></span><span><KeyRound size={14}/><b>{binding?.credential_configured?'连接设置已完成':'还未完成连接设置'}</b></span></div></section>;
}

function modeLabel(modes:string[]){return modes.map(mode=>mode==='text_to_video'?'文生视频':mode==='image_to_video'?'图生视频':mode).join('、')||'未声明'}
