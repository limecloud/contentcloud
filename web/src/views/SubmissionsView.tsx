import { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Check, ChevronRight, Download, FileCheck2, FileJson2, FileSpreadsheet, FileText, GitCompareArrows, History, Link2, MessageSquareText, PackageCheck, RefreshCw, ShieldCheck, X, XCircle } from 'lucide-react';
import { api, download, post } from '../api';
import type { ApprovedSnapshot, Artifact, DeliveryPackage, Project, ReviewGrant, Submission, SubmissionDetails, SubmissionRevision } from '../types';
import { Banner, Button, Empty, Field, IconButton, Loading, Modal, Status } from '../components/ui';
import { ProjectPage } from './OverviewView';

export function SubmissionsView({project,role}:{project:Project;role:string}) {
  const [items,setItems]=useState<Submission[]>([]);
  const [snapshots,setSnapshots]=useState<ApprovedSnapshot[]>([]);
  const [packages,setPackages]=useState<DeliveryPackage[]>([]);
  const [selectedID,setSelectedID]=useState('');
  const [details,setDetails]=useState<SubmissionDetails>();
  const [revisionID,setRevisionID]=useState('');
  const [grants,setGrants]=useState<ReviewGrant[]>([]);
  const [showGrants,setShowGrants]=useState(false);
  const [loading,setLoading]=useState(true);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');
  const [decision,setDecision]=useState<'approve'|'changes'>('approve');
  const [reason,setReason]=useState('');
  const [pointer,setPointer]=useState('');
  const canReview=['tenant_admin','project_manager','reviewer'].includes(role);
  const canManage=role==='tenant_admin'||role==='project_manager';
  const canDeliver=['tenant_admin','project_manager','editor'].includes(role);

  const load=async()=>{
    setLoading(true);setError('');
    try{
      const [submissions,approved,deliveries]=await Promise.all([
        api<Submission[]>(`/api/bff/projects/${project.id}/submissions`),
        api<ApprovedSnapshot[]>(`/api/bff/projects/${project.id}/approved-snapshots`),
        api<DeliveryPackage[]>(`/api/bff/projects/${project.id}/delivery-packages`),
      ]);
      setItems(submissions);setSnapshots(approved);setPackages(deliveries);
      setSelectedID(previous=>submissions.some(item=>item.id===previous)?previous:submissions[0]?.id||'');
    }catch(cause){setError(cause instanceof Error?cause.message:'提交审核加载失败')}finally{setLoading(false)}
  };
  const loadDetails=async(submissionID:string)=>{
    const value=await api<SubmissionDetails>(`/api/bff/submissions/${submissionID}`);
    setDetails(value);
    setRevisionID(previous=>value.revisions.some(item=>item.id===previous)?previous:value.submission.current_revision_id||value.revisions[0]?.id||'');
  };
  const loadGrants=async(id:string)=>{
    try{setGrants(await api<ReviewGrant[]>(`/api/bff/submission-revisions/${id}/review-grants`))}catch{setGrants([])}
  };
  useEffect(()=>{setDetails(undefined);setSelectedID('');load()},[project.id]);
  useEffect(()=>{if(!selectedID){setDetails(undefined);return}loadDetails(selectedID).catch(cause=>setError(cause instanceof Error?cause.message:'提交详情加载失败'))},[selectedID]);
  useEffect(()=>{if(!revisionID){setGrants([]);return}loadGrants(revisionID)},[revisionID]);

  const revision=details?.revisions.find(item=>item.id===revisionID);
  const previous=details?.revisions.find(item=>item.revision_no===(revision?.revision_no||1)-1);
  const current=details?.submission.current_revision_id===revision?.id;
  const actionable=current&&['submitted','in_review'].includes(details?.submission.status||'');
  const clientActionable=current&&details?.submission.submission_type==='script'&&['internally_approved','client_review'].includes(details?.submission.status||'');
  const disclosureCounts=useMemo(()=>countDisclosures(revision),[revision]);

  const refreshCurrent=async()=>{
    await load();
    if(selectedID)await loadDetails(selectedID);
    if(revisionID)await loadGrants(revisionID);
  };
  const decide=async()=>{
    if(!revision||!reason.trim())return;
    setBusy(true);setError('');
    try{
      if(decision==='approve')await post(`/api/bff/submission-revisions/${revision.id}/approve`,{reason});
      else await post(`/api/bff/submission-revisions/${revision.id}/request-changes`,{reason,json_pointer:pointer});
      setReason('');setPointer('');await refreshCurrent();
    }catch(cause){setError(cause instanceof Error?cause.message:'审核决定失败')}finally{setBusy(false)}
  };
  const exportSnapshot=async(snapshot:ApprovedSnapshot,format:'json'|'markdown'|'xlsx')=>{
    setBusy(true);setError('');
    try{
      const artifact=await post<Artifact>(`/api/bff/approved-snapshots/${snapshot.id}/exports`,{format});
      await saveArtifact(artifact);
    }catch(cause){setError(cause instanceof Error?cause.message:'快照导出失败')}finally{setBusy(false)}
  };
  const createPackage=async(snapshot:ApprovedSnapshot)=>{
    setBusy(true);setError('');
    try{
      await post<DeliveryPackage>(`/api/bff/approved-snapshots/${snapshot.id}/delivery-packages`,{});
      await load();
    }catch(cause){setError(cause instanceof Error?cause.message:'交付包生成失败')}finally{setBusy(false)}
  };

  return <ProjectPage project={project} kicker="云端治理" title="提交审核" actions={<IconButton label="刷新提交" onClick={load}><RefreshCw size={17}/></IconButton>}>
    {error&&<Banner kind="error" onClose={()=>setError('')}>{error}</Banner>}
    {loading?<div className="submission-loading"><Loading/></div>:items.length===0?<Empty title="暂无待审核提交" detail="本地客户端 publish 后会在这里形成不可变版本"/>:<>
      <div className="submission-summary"><div><FileCheck2 size={18}/><span>提交</span><strong>{items.length}</strong></div><div><MessageSquareText size={18}/><span>待处理</span><strong>{items.filter(item=>['submitted','in_review','changes_requested','internally_approved','client_review'].includes(item.status)).length}</strong></div><div><ShieldCheck size={18}/><span>批准快照</span><strong>{snapshots.length}</strong></div><div><AlertTriangle size={18}/><span>证据受限</span><strong>{details?.revisions.filter(item=>item.evidence_limited).length||0}</strong></div></div>
      <div className="submission-workspace">
        <section className="section submission-list"><header className="section-header"><div><span className="section-kicker">检查点</span><h2>{items.length} 个提交</h2></div></header><div>{items.map(item=><button key={item.id} className={selectedID===item.id?'active':''} onClick={()=>setSelectedID(item.id)}><span className="submission-type">{typeLabel(item.submission_type)}</span><div><strong>{typeLabel(item.submission_type)}检查点</strong><small>{formatDate(item.updated_at)} · {item.current_revision_id.slice(0,8)}</small></div><Status value={item.status}/><ChevronRight size={15}/></button>)}</div></section>
        <section className="section submission-detail">{!details||!revision?<div className="submission-loading"><Loading/></div>:<>
          <header className="section-header"><div><span className="section-kicker">不可变版本</span><h2>{typeLabel(details.submission.submission_type)} · V{revision.revision_no}</h2></div><Status value={details.submission.status}/></header>
          <div className="revision-strip">{details.revisions.map(item=><button key={item.id} className={item.id===revision.id?'active':''} onClick={()=>setRevisionID(item.id)}><span>V{item.revision_no}</span><small>{item.content_hash.slice(0,10)}</small>{item.id===details.submission.current_revision_id&&<i>当前</i>}</button>)}</div>
          <div className="revision-facts"><div><span>内容摘要</span><code>{revision.content_hash.slice(0,24)}</code></div><div><span>Schema</span><strong>{revision.schema_version}</strong></div><div><span>对象</span><strong>{revision.objects.length}</strong></div><div><span>披露</span><strong>{revision.source_disclosures.length}</strong></div></div>
          {revision.evidence_limited&&<Banner kind="error">高风险内容的证据披露不足，当前版本不能远程批准。</Banner>}
          {previous&&<div className="revision-compare"><GitCompareArrows size={16}/><div><strong>与 V{previous.revision_no} 比较</strong><span>对象 {signed(revision.objects.length-previous.objects.length)} · 披露 {signed(revision.source_disclosures.length-previous.source_disclosures.length)} · hash 已变化</span></div></div>}
          <div className="submission-tabs"><section><header><strong>审核对象</strong><span>{revision.objects.length}</span></header><div className="object-list">{revision.objects.map((object,index)=><article key={String(object.id||index)}><div><strong>{String(object.title||object.name||object.id||`对象 ${index+1}`)}</strong><span>{String(object.kind||object.deliverability||object.status||'content')}</span></div><code>{object.id?String(object.id).slice(0,12):`#${index+1}`}</code><details><summary>字段</summary><pre>{JSON.stringify(object,null,2)}</pre></details></article>)}</div></section>
            <aside><div className="disclosure-summary"><header><strong>来源披露</strong></header><dl><div><dt>仅元数据</dt><dd>{disclosureCounts.metadata_only}</dd></div><div><dt>证据包</dt><dd>{disclosureCounts.evidence_pack}</dd></div><div><dt>完整原件</dt><dd>{disclosureCounts.full_source}</dd></div></dl>{revision.source_disclosures.map(item=><div className="disclosure-row" key={item.id||item.source_ref}><span>{item.source_ref}</span><strong>{disclosureLabel(item.level)}</strong><code>{item.sha256.replace('sha256:','').slice(0,10)}</code></div>)}</div>
            <div className="feedback-list"><header><strong>审核记录</strong><span>{details.comments.filter(item=>item.subject_id===revision.id).length}</span></header>{details.comments.filter(item=>item.subject_id===revision.id).map(item=><article key={item.id}><span>{item.json_pointer||'整版'}</span><p>{item.body}</p><small>{formatDate(item.created_at)}</small></article>)}</div></aside></div>
          {canReview&&actionable&&<div className="submission-decision"><div className="decision-segment"><button className={decision==='approve'?'active':''} onClick={()=>setDecision('approve')}><Check size={15}/>批准</button><button className={decision==='changes'?'active':''} onClick={()=>setDecision('changes')}><X size={15}/>提出修改</button></div><div className="decision-fields">{decision==='changes'&&<Field label="字段位置"><input value={pointer} onChange={event=>setPointer(event.target.value)} placeholder="/0/shots/2/voiceover"/></Field>}<Field label={decision==='approve'?'整版结论':'修改要求'}><textarea value={reason} onChange={event=>setReason(event.target.value)} rows={3}/></Field><Button disabled={busy||!reason.trim()} onClick={decide}>{busy?'提交中…':decision==='approve'?(details.submission.submission_type==='script'?'通过内审':'批准并生成快照'):'发送修改要求'}</Button></div></div>}
          {clientActionable&&<div className="submission-client-review"><div><Link2 size={16}/><div><strong>客户 OTP 审批</strong><span>{grants.length?`${grants.length} 个授权 · ${grantSummary(grants)}`:'尚未创建客户授权'}</span></div></div>{canManage&&<Button disabled={busy} onClick={()=>setShowGrants(true)}><Link2 size={15}/>{details.submission.status==='client_review'?'管理审批授权':'发起客户审批'}</Button>}</div>}
        </>}</section>
      </div>
    </>}

    {snapshots.length>0&&<section className="section snapshot-delivery"><header className="section-header"><div><span className="section-kicker">ApprovedSnapshot</span><h2>批准快照与交付</h2></div><span className="section-count">{snapshots.length} 个</span></header><div className="snapshot-list">{snapshots.map(snapshot=>{const deliverable=snapshot.submission_type==='script'&&snapshot.origin==='current';return <article key={snapshot.id}><div><PackageCheck size={17}/><span><strong>{typeLabel(snapshot.submission_type)} · {snapshot.eligible_ids[0]||snapshot.id.slice(0,8)}</strong><small>{formatDate(snapshot.created_at)} · {snapshot.origin==='v1_import'?'V1 历史影子':'客户批准'} · {snapshot.content_hash.slice(0,12)}</small></span></div>{deliverable&&canDeliver?<div className="snapshot-actions"><Button variant="ghost" disabled={busy} title="下载 JSON" onClick={()=>exportSnapshot(snapshot,'json')}><FileJson2 size={15}/></Button><Button variant="ghost" disabled={busy} title="下载 Markdown" onClick={()=>exportSnapshot(snapshot,'markdown')}><FileText size={15}/></Button><Button variant="ghost" disabled={busy} title="下载 XLSX" onClick={()=>exportSnapshot(snapshot,'xlsx')}><FileSpreadsheet size={15}/></Button><Button variant="secondary" disabled={busy} onClick={()=>createPackage(snapshot)}><PackageCheck size={15}/>生成交付包</Button></div>:<Status value={snapshot.origin}/>}</article>})}</div></section>}

    {packages.length>0&&<section className="section delivery-packages"><header className="section-header"><div><span className="section-kicker">DeliveryPackage</span><h2>三格式交付包</h2></div><span className="section-count">{packages.length} 个</span></header><div>{packages.map(value=><article key={value.id}><header><div><PackageCheck size={17}/><span><strong>{value.script_id}</strong><small>{formatDate(value.created_at)} · {value.approved_snapshot_ids[0]?.slice(0,8)}</small></span></div><Status value={value.status}/></header><div>{value.manifest.map(artifact=><Button key={artifact.id} variant="ghost" onClick={()=>saveArtifact(artifact)}><Download size={14}/>{artifact.file_name}</Button>)}</div></article>)}</div></section>}

    {showGrants&&revision&&<ReviewGrantModal revision={revision} grants={grants} onClose={()=>setShowGrants(false)} onChanged={()=>loadGrants(revision.id)}/>}
  </ProjectPage>
}

function ReviewGrantModal({revision,grants,onClose,onChanged}:{revision:SubmissionRevision;grants:ReviewGrant[];onClose:()=>void;onChanged:()=>void}) {
  const [email,setEmail]=useState('');const [created,setCreated]=useState<ReviewGrant>();const [busy,setBusy]=useState(false);const [error,setError]=useState('');
  const create=async()=>{setBusy(true);setError('');try{const value=await post<ReviewGrant>(`/api/bff/submission-revisions/${revision.id}/review-grants`,{reviewer_email:email});setCreated(value);onChanged()}catch(cause){setError(cause instanceof Error?cause.message:'创建失败')}finally{setBusy(false)}};
  const revoke=async(grant:ReviewGrant)=>{if(!window.confirm(`确认撤销发给 ${grant.reviewer_email} 的审批链接？撤销后会立即失效。`))return;setBusy(true);setError('');try{await post(`/api/bff/review-grants/${grant.id}/revoke`);onChanged()}catch(cause){setError(cause instanceof Error?cause.message:'撤销失败')}finally{setBusy(false)}};
  const url=created?.review_token?`${window.location.origin}/review/${created.review_token}`:'';
  return <Modal title="客户审批授权" onClose={onClose}>{created&&url?<div className="grant-result"><Banner kind="success">链接与验证码仅显示本次，请通过不同渠道分别发送给客户。</Banner><Field label="审批链接"><input readOnly value={url}/></Field><Field label="六位验证码"><input readOnly value={created.dev_otp||''}/></Field><div className="row-actions"><Button onClick={()=>navigator.clipboard.writeText(url)}>复制审批链接</Button><Button variant="secondary" onClick={()=>setCreated(undefined)}>继续管理</Button></div></div>:<><p className="modal-copy">新链接固定绑定当前 SubmissionRevision 和内容哈希，不会自动切换到后续版本。</p><div className="grant-create-row"><Field label="客户审批邮箱"><input type="email" value={email} onChange={event=>setEmail(event.target.value)} placeholder="reviewer@example.com"/></Field><Button disabled={busy||!email.includes('@')} onClick={create}><Link2 size={15}/>生成安全链接</Button></div><div className="grant-history"><header><History size={15}/><strong>授权历史</strong><span>{grants.length}</span></header>{grants.length===0?<p>尚未创建审批授权</p>:grants.map(grant=><div key={grant.id}><div><strong>{grant.reviewer_email}</strong><span>{formatDate(grant.created_at)} · {grantStatusLabel(grant)}</span></div>{isActiveGrant(grant)&&<Button variant="danger" disabled={busy} onClick={()=>revoke(grant)}><XCircle size={14}/>撤销</Button>}</div>)}</div></>}{error&&<p className="form-error">{error}</p>}<footer className="modal-actions"><Button variant="secondary" onClick={onClose}>关闭</Button></footer></Modal>
}

async function saveArtifact(artifact:Artifact){const result=await download(`/api/bff/artifacts/${artifact.id}/download`);const href=URL.createObjectURL(result.blob);const link=document.createElement('a');link.href=href;link.download=result.fileName;link.click();URL.revokeObjectURL(href)}
const typeLabel=(value:string)=>({knowledge:'知识',research:'研究',strategy:'策略',brief:'Brief',script:'剧本',delivery:'交付',performance:'投放结果'}[value]||value);
const disclosureLabel=(value:string)=>({metadata_only:'仅元数据',evidence_pack:'证据包',full_source:'完整原件'}[value]||value);
const formatDate=(value:string)=>new Intl.DateTimeFormat('zh-CN',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}).format(new Date(value));
const signed=(value:number)=>value>0?`+${value}`:`${value}`;
const isActiveGrant=(grant:ReviewGrant)=>!grant.revoked_at&&!grant.decision_at&&new Date(grant.expires_at)>new Date();
const grantStatusLabel=(grant:ReviewGrant)=>grant.revoked_at?'已撤销':grant.decision_at?'已决定':new Date(grant.expires_at)<=new Date()?'已过期':grant.verified_at?'已验证':'待验证';
const grantSummary=(grants:ReviewGrant[])=>grants.some(isActiveGrant)?'等待客户决定':grants.some(item=>item.decision_at)?'客户已决定':'无有效授权';
function countDisclosures(revision?:SubmissionRevision){const result={metadata_only:0,evidence_pack:0,full_source:0};revision?.source_disclosures.forEach(item=>result[item.level]++);return result}
