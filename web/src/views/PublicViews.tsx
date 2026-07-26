import { CheckCircle2, KeyRound, ShieldCheck, Undo2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { api, post } from '../api';
import type { ReviewProjection, ScriptPackageV2, Shot } from '../types';
import { Banner, Button, Field, Loading, Status } from '../components/ui';

export function DeviceAuthView() {
  const [code,setCode]=useState(new URLSearchParams(window.location.search).get('user_code') || '');
  const [done,setDone]=useState(false);const [busy,setBusy]=useState(false);const [error,setError]=useState('');
  const approve=async()=>{setBusy(true);setError('');try{await post('/api/bff/device-auth/approve',{user_code:code.trim().toUpperCase()});setDone(true)}catch(e){setError(e instanceof Error?e.message:'授权失败')}finally{setBusy(false)}};
  return <main className="public-page"><section className="public-panel"><div className="public-brand"><span>CC</span><strong>ContentCloud</strong></div>{done?<><div className="public-success"><CheckCircle2 size={28}/></div><h1>客户端已授权</h1><p>可以返回终端继续完成登录，此页面现在可以关闭。</p></>:<><div className="public-icon"><KeyRound size={24}/></div><h1>授权 ContentCloud CLI</h1><p>输入终端显示的一次性用户代码。授权只建立当前用户的 CLI 会话，不会授权本地 Agent 凭据。</p><Field label="用户代码"><input autoFocus value={code} onChange={e=>setCode(e.target.value.toUpperCase())} placeholder="ABCD-EFGH" /></Field>{error&&<Banner kind="error">{error}</Banner>}<Button disabled={busy||code.trim().length<6} onClick={approve}>{busy?'验证中…':'确认授权'}</Button></>}</section></main>
}

export function PublicReviewView({token}:{token:string}) {
  const [projection,setProjection]=useState<ReviewProjection>();const [otp,setOTP]=useState('');const [reason,setReason]=useState('');const [busy,setBusy]=useState(false);const [error,setError]=useState('');const [done,setDone]=useState('');
  const load=()=>api<ReviewProjection>(`/api/review/${encodeURIComponent(token)}/projection`).then(setProjection).catch(e=>setError(e.message));
  useEffect(()=>{load()},[token]);
  const verify=async()=>{setBusy(true);setError('');try{setProjection(await post(`/api/review/${encodeURIComponent(token)}/verify`,{otp}))}catch(e){setError(e instanceof Error?e.message:'验证失败')}finally{setBusy(false)}};
  const decide=async(decision:'approve'|'return')=>{setBusy(true);setError('');try{await post(`/api/review/${encodeURIComponent(token)}/decision`,{decision,reason});setDone(decision)}catch(e){setError(e instanceof Error?e.message:'提交失败')}finally{setBusy(false)}};
  if(!projection&&!error)return <main className="public-page"><Loading/></main>;
  if(!projection)return <main className="public-page"><section className="public-panel"><Banner kind="error">{error}</Banner></section></main>;
  if(done)return <main className="public-page"><section className="public-panel"><div className="public-success"><CheckCircle2 size={28}/></div><h1>{done==='approve'?'剧本已批准':'修改意见已提交'}</h1><p>本次决定已绑定到剧本内容哈希并写入审计记录。</p></section></main>;
  const subject=reviewSubject(projection);
  return <main className="review-page"><header className="review-public-header"><div className="public-brand"><span>CC</span><strong>ContentCloud 审批</strong></div><div><strong>{projection.project.brand_name}</strong><span>{projection.project.product_name}</span></div></header><div className="review-public-content">{error&&<Banner kind="error" onClose={()=>setError('')}>{error}</Banner>}{!projection.verified?<section className="verify-panel"><ShieldCheck size={26}/><h1>验证审批身份</h1><p>输入与该链接分开发送的六位验证码。</p><Field label="验证码"><input inputMode="numeric" maxLength={6} value={otp} onChange={e=>setOTP(e.target.value.replace(/\D/g,''))}/></Field><Button disabled={busy||otp.length!==6} onClick={verify}>{busy?'验证中…':'查看指定剧本'}</Button></section>:!subject?<section className="verify-panel"><Banner kind="error">审批对象不是可展示的剧本包。</Banner></section>:<><section className="review-summary"><div><span>{subject.versionLabel}</span><h1>{subject.title}</h1><p>{subject.objective}</p></div><div><Status value={subject.status}/><code>{subject.hash.slice(0,16)}</code></div></section><section className="review-shot-list">{subject.shots.map((shot,index)=><article key={shot.shot_id}><header><span>{String(index+1).padStart(2,'0')} · {Math.max(1,Math.round((shot.end_ms-shot.start_ms)/1000))}s</span><Status value={shot.role}/></header><h2>{shot.narrative_purpose}</h2><dl><div><dt>画面</dt><dd>{shot.visual_intent}</dd></div><div><dt>动作</dt><dd>{shot.subject_action}</dd></div><div><dt>口播 / 字幕</dt><dd>{shot.voiceover||shot.on_screen_text||'无'}</dd></div><div><dt>验收</dt><dd>{shot.acceptance_criteria.join('；')||'无'}</dd></div></dl>{projection.comments.filter(c=>c.shot_id===shot.shot_id).map(c=><blockquote key={c.id}>{c.body}</blockquote>)}</article>)}</section><section className="review-decision"><Field label="审批意见"><textarea rows={3} value={reason} onChange={e=>setReason(e.target.value)} placeholder="批准时可选；退回时必填具体修改原因"/></Field><div><Button variant="secondary" disabled={busy||!reason.trim()} onClick={()=>decide('return')}><Undo2 size={16}/>退回修改</Button><Button disabled={busy} onClick={()=>decide('approve')}><CheckCircle2 size={16}/>批准此版本</Button></div></section></>}</div></main>
}

type ReviewShot = Pick<Shot,'shot_id'|'start_ms'|'end_ms'|'role'|'narrative_purpose'|'visual_intent'|'subject_action'|'voiceover'|'on_screen_text'|'acceptance_criteria'>;
type ReviewSubject = {versionLabel:string;title:string;objective:string;status:string;hash:string;shots:ReviewShot[]};

function reviewSubject(projection:ReviewProjection):ReviewSubject|undefined {
  if(projection.submission){
    const value=projection.submission.objects.find(object=>Array.isArray(object.shots)) as unknown as ScriptPackageV2|undefined;
    if(!value)return undefined;
    return {versionLabel:`Script Package 2.0 · ${Math.round(value.duration_ms/1000)}s`,title:value.title,objective:value.direction?.title||value.direction?.angle||value.experiment?.hypothesis||'',status:value.status,hash:projection.submission.subject_hash,shots:value.shots};
  }
  const script=projection.script;
  if(!script)return undefined;
  return {versionLabel:`AI 视频剧本 V${script.version}`,title:script.package.title,objective:script.package.creative_strategy.objective,status:script.status,hash:script.content_hash,shots:script.package.shots};
}
