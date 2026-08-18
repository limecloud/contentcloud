import { CheckCircle2, KeyRound, ShieldCheck, Undo2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { api, post } from '../api';
import type { ReviewProjection } from '../types';
import { Banner, Button, Field, Loading } from '../components/ui';
import { BrandLockup } from '../components/Brand';
import { ReviewContent } from './ReviewContent';
import { reviewSubject } from './reviewSubject';

export function DeviceAuthView() {
  const [code,setCode]=useState(new URLSearchParams(window.location.search).get('user_code') || '');
  const [done,setDone]=useState(false);const [busy,setBusy]=useState(false);const [error,setError]=useState('');
  const approve=async()=>{setBusy(true);setError('');try{await post('/api/bff/device-auth/approve',{user_code:code.trim().toUpperCase()});setDone(true)}catch(e){setError(e instanceof Error?e.message:'授权失败')}finally{setBusy(false)}};
  return <main className="public-page"><section className="public-panel"><div className="public-brand"><BrandLockup/></div>{done?<><div className="public-success"><CheckCircle2 size={28}/></div><h1>电脑已连接</h1><p>回到电脑上的连接工具继续登录即可，此页面可以关闭。</p></>:<><div className="public-icon"><KeyRound size={24}/></div><h1>确认电脑连接</h1><p>输入电脑上显示的一次性代码。这里只会把这台电脑连接到当前账号，不会授予其他权限。</p><Field label="一次性代码"><input autoFocus value={code} onChange={e=>setCode(e.target.value.toUpperCase())} placeholder="ABCD-EFGH" /></Field>{error&&<Banner kind="error">{error}</Banner>}<Button disabled={busy||code.trim().length<6} onClick={approve}>{busy?'验证中…':'确认连接'}</Button></>}</section></main>
}

export function PublicReviewView({token}:{token:string}) {
  const [projection,setProjection]=useState<ReviewProjection>();const [otp,setOTP]=useState('');const [reason,setReason]=useState('');const [busy,setBusy]=useState(false);const [error,setError]=useState('');const [done,setDone]=useState('');
  const load=()=>api<ReviewProjection>(`/api/review/${encodeURIComponent(token)}/projection`).then(setProjection).catch(e=>setError(e.message));
  useEffect(()=>{load()},[token]);
  const verify=async()=>{setBusy(true);setError('');try{setProjection(await post(`/api/review/${encodeURIComponent(token)}/verify`,{otp}))}catch(e){setError(e instanceof Error?e.message:'验证失败')}finally{setBusy(false)}};
  const decide=async(decision:'approve'|'return')=>{setBusy(true);setError('');try{await post(`/api/review/${encodeURIComponent(token)}/decision`,{decision,reason});setDone(decision)}catch(e){setError(e instanceof Error?e.message:'提交失败')}finally{setBusy(false)}};
  if(!projection&&!error)return <main className="public-page"><Loading/></main>;
  if(!projection)return <main className="public-page"><section className="public-panel"><Banner kind="error">{error}</Banner></section></main>;
  if(done)return <main className="public-page"><section className="public-panel"><div className="public-success"><CheckCircle2 size={28}/></div><h1>{done==='approve'?'内容已确认':'修改意见已提交'}</h1><p>你的决定已保存到这份内容的当前版本，并留下记录。</p></section></main>;
  const subject=reviewSubject(projection);
  return <main className="review-page"><header className="review-public-header"><div className="public-brand"><BrandLockup subtitle="内容确认"/></div><div><strong>{projection.project.brand_name}</strong><span>{projection.project.product_name}</span></div></header><div className="review-public-content">{error&&<Banner kind="error" onClose={()=>setError('')}>{error}</Banner>}{!projection.verified?<section className="verify-panel"><ShieldCheck size={26}/><h1>确认你的身份</h1><p>输入与该链接分开发送的六位验证码。</p><Field label="验证码"><input inputMode="numeric" maxLength={6} value={otp} onChange={e=>setOTP(e.target.value.replace(/\D/g,''))}/></Field><Button disabled={busy||otp.length!==6} onClick={verify}>{busy?'验证中…':'查看内容'}</Button></section>:!subject?<section className="verify-panel"><Banner kind="error">这份内容暂时无法展示，请联系发送链接的人。</Banner></section>:<><ReviewContent subject={subject} comments={projection.comments}/><section className="review-decision"><Field label="你的意见"><textarea rows={3} value={reason} onChange={e=>setReason(e.target.value)} placeholder="确认时可不填；退回时请写清楚需要修改的地方"/></Field><div><Button variant="secondary" disabled={busy||!reason.trim()} onClick={()=>decide('return')}><Undo2 size={16}/>退回修改</Button><Button disabled={busy} onClick={()=>decide('approve')}><CheckCircle2 size={16}/>确认此版本</Button></div></section></>}</div></main>
}
