import { Archive, ArrowDown, ArrowLeft, ArrowRight, ArrowUp, Check, CircleAlert, CircleCheck, Copy, GitBranch, GitCompare, Plus, RotateCcw, Search, ShieldAlert, Trash2, Users, Workflow } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { api, patch, post } from '../api';
import type { AdminWorkOSView, Environment, EnvironmentCapability, GateDefinition, SOPDiffChange, SOPLintReport, SOPSummary, SOPVersion, SOPVersionDiff, SOPVersionImpact, StageDefinition } from '../types';
import { Button, Empty, Field, Modal } from '../components/ui';
import { normalizeEnvironment, normalizeSOPSummary, normalizeSOPVersion } from './operationsData';
import { executionModeLabel, gateModeBlockingLabel, gateModeLabel, statusLabel, workOSTerm } from './workOSLabels';
import { roleLabel, statusLabel as commonStatusLabel, workflowCheckLabel } from '../uiLabels';
import '../styles/workSurface.css';

type Notice = string;

const splitList = (value: string) => value.split(',').map(item => item.trim()).filter(Boolean);
const joinList = (value: string[]) => value.join(', ');
const nextOrder = (stages: StageDefinition[]) => (Math.max(0, ...stages.map(stage => stage.order)) + 10);

export interface AdminProductDraftForm { name:string; description:string; content_type:string; default_execution_mode:string }
export const emptyAdminProductDraft = ():AdminProductDraftForm => ({name:'',description:'',content_type:'video_script',default_execution_mode:'local'});
export const createAdminProduct = async(form:AdminProductDraftForm) => normalizeSOPSummary(await post<SOPSummary>('/api/bff/admin/sops',{name:form.name,description:form.description,content_types:[form.content_type],default_execution_mode:form.default_execution_mode}));

interface PublishedProductVersion extends SOPVersion { label:string }
interface EnvironmentCreateForm { name:string; slug:string; status:string; default_sop_id:string; default_sop_version:number; capabilities:EnvironmentCapability[] }
interface AdminEnvironmentPanelProps {
  environments:Environment[];
  sops:SOPSummary[];
  onSaved:()=>Promise<void>|void;
  initialEnvironmentID?:string;
  initialProductID?:string;
  availableCapabilities?:AdminWorkOSView['capabilities'];
  variant?:'environment'|'enrollment';
  directoryOnly?:boolean;
  scoped?:boolean;
  filterProductID?:string;
  environmentPath?:(environmentID:string)=>string;
  onSelectionChange?:(environmentID:string)=>void;
  onCreated?:(environmentID:string)=>void;
}

function publishedProductVersions(sops:SOPSummary[]):PublishedProductVersion[] {
  return sops.flatMap(summary => summary.versions
    .filter(version => version.status === 'published')
    .map(version => ({...version, label: `${summary.definition.name} · v${version.version}`})));
}

function requiredCapabilityIDs(sops:SOPSummary[], sopID?:string, version?:number):string[] {
  const selected = sops.find(summary => summary.definition.id === sopID)?.versions.find(candidate => candidate.version === version);
  return Array.from(new Set(selected?.stages.flatMap(stage => stage.required_capabilities) || []));
}

function capabilityOptions(available:AdminWorkOSView['capabilities'], existing:EnvironmentCapability[]=[]):EnvironmentCapability[] {
  const byID = new Map<string,EnvironmentCapability>();
  for (const capability of available) byID.set(capability.id, {id: capability.id, version: capability.version, enabled: false});
  for (const capability of existing) byID.set(capability.id, capability);
  return Array.from(byID.values()).sort((left, right) => left.id.localeCompare(right.id));
}

function defaultCapabilityScope(sops:SOPSummary[], available:AdminWorkOSView['capabilities'], sopID?:string, version?:number):EnvironmentCapability[] {
  const required = new Set(requiredCapabilityIDs(sops, sopID, version));
  return capabilityOptions(available).filter(capability => required.has(capability.id)).map(capability => ({...capability, enabled: true}));
}

export function AdminEnvironmentPanel({environments, sops, onSaved, initialEnvironmentID, initialProductID, availableCapabilities=[], variant='environment', directoryOnly=false, scoped=false, filterProductID, environmentPath, onSelectionChange, onCreated}: AdminEnvironmentPanelProps) {
  const publishedSOPs = useMemo(() => publishedProductVersions(sops), [sops]);
  const initialPublished = publishedSOPs.find(version => version.sop_id === initialProductID) || publishedSOPs[0];
  const [selectedID, setSelectedID] = useState(initialEnvironmentID || environments[0]?.id || '');
  const selected = environments.find(value => value.id === selectedID) || environments.find(value => value.id === initialEnvironmentID) || environments[0];
  const [form, setForm] = useState<Environment | undefined>(selected);
  const [query, setQuery] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [createStep, setCreateStep] = useState(1);
  const [createForm, setCreateForm] = useState<EnvironmentCreateForm>({name: '', slug: '', status: 'active', default_sop_id: initialPublished?.sop_id || '', default_sop_version: initialPublished?.version || 0, capabilities: defaultCapabilityScope(sops, availableCapabilities, initialPublished?.sop_id, initialPublished?.version)});
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState<Notice>('');
  const enrollment = variant === 'enrollment';
  const filtered = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return environments.filter(environment => {
      if (filterProductID && environment.default_sop_id !== filterProductID) return false;
      const product = publishedSOPs.find(version => version.sop_id === environment.default_sop_id && version.version === environment.default_sop_version)?.label || '';
      return `${environment.name} ${environment.slug} ${product}`.toLowerCase().includes(normalizedQuery);
    });
  }, [environments, filterProductID, publishedSOPs, query]);

  useEffect(() => {
    if (initialEnvironmentID && environments.some(environment => environment.id === initialEnvironmentID)) setSelectedID(initialEnvironmentID);
  }, [environments, initialEnvironmentID]);
  useEffect(() => { setForm(selected); }, [selectedID, environments]);

  const openCreate = () => {
    const preferred = publishedSOPs.find(version => version.sop_id === initialProductID) || publishedSOPs[0];
    setCreateForm({name: '', slug: '', status: 'active', default_sop_id: preferred?.sop_id || '', default_sop_version: preferred?.version || 0, capabilities: defaultCapabilityScope(sops, availableCapabilities, preferred?.sop_id, preferred?.version)});
    setCreateStep(1);
    setNotice('');
    setCreateOpen(true);
  };
  const closeCreate = () => { setCreateOpen(false); setCreateStep(1); };
  const selectEnvironment = (environmentID:string) => {
    setSelectedID(environmentID);
    if (!environmentPath) onSelectionChange?.(environmentID);
  };
  const selectCreateProduct = (value:string) => {
    const [sopID, versionValue] = value.split(':');
    const version = Number(versionValue);
    setCreateForm(current => ({...current, default_sop_id: sopID, default_sop_version: version, capabilities: defaultCapabilityScope(sops, availableCapabilities, sopID, version)}));
  };
  const updateCapability = (id:string, version:string, enabled:boolean) => {
    if (!form) return;
    const exists = form.capabilities.some(capability => capability.id === id);
    setForm({...form, capabilities: exists ? form.capabilities.map(capability => capability.id === id ? {...capability, enabled} : capability) : [...form.capabilities, {id, version, enabled}]});
  };
  const updateCreateCapability = (id:string, version:string, enabled:boolean) => {
    const exists = createForm.capabilities.some(capability => capability.id === id);
    setCreateForm({...createForm, capabilities: exists ? createForm.capabilities.map(capability => capability.id === id ? {...capability, enabled} : capability) : [...createForm.capabilities, {id, version, enabled}]});
  };
  const save = async () => {
    if (!form) return;
    setSaving(true); setNotice('');
    try {
      await patch<Environment>(`/api/bff/admin/environments/${encodeURIComponent(form.id)}`, {name: form.name, slug: form.slug, status: form.status, default_sop_id: form.default_sop_id || '', default_sop_version: form.default_sop_version || 0, capabilities: form.capabilities});
      setNotice(enrollment ? '客户开通已保存。新配置只影响后续创建的任务，历史任务和结果保持不变。' : '执行环境已保存，后续新任务会读取这份配置。');
      await onSaved();
    } catch (value) { setNotice(value instanceof Error ? value.message : enrollment ? '客户开通保存失败' : '执行环境保存失败'); }
    finally { setSaving(false); }
  };
  const create = async () => {
    setSaving(true); setNotice('');
    try {
      const created = normalizeEnvironment(await post<Environment>('/api/bff/admin/environments', createForm));
      closeCreate(); setSelectedID(created.id);
      setNotice(enrollment ? `客户「${created.name}」已开通。` : `执行环境「${created.name}」已创建。`);
      await onSaved();
      onCreated?.(created.id);
    } catch (value) { setNotice(value instanceof Error ? value.message : enrollment ? '客户开通创建失败' : '执行环境创建失败'); }
    finally { setSaving(false); }
  };

  const selectedProduct = publishedSOPs.find(version => version.sop_id === form?.default_sop_id && version.version === form?.default_sop_version);
  const required = requiredCapabilityIDs(sops, form?.default_sop_id, form?.default_sop_version);
  const enabled = new Set(form?.capabilities.filter(capability => capability.enabled).map(capability => capability.id) || []);
  const missing = required.filter(capabilityID => !enabled.has(capabilityID));
  const editorCapabilities = capabilityOptions(availableCapabilities, form?.capabilities);
  const createRequired = requiredCapabilityIDs(sops, createForm.default_sop_id, createForm.default_sop_version);
  const createEnabled = new Set(createForm.capabilities.filter(capability => capability.enabled).map(capability => capability.id));
  const createMissing = createRequired.filter(capabilityID => !createEnabled.has(capabilityID));
  const emptyAction = <Button onClick={openCreate}><Plus size={15}/>{enrollment ? '新建客户开通' : '新建执行环境'}</Button>;
  const directory = !scoped && <section className="workos-section admin-config-section">
    <div className="admin-config-toolbar"><label className="search-field"><Search size={16}/><input placeholder={enrollment ? '搜索客户或创作产品' : '搜索执行环境'} aria-label={enrollment ? '搜索客户开通' : '搜索执行环境'} value={query} onChange={event => setQuery(event.target.value)}/></label><span className="toolbar-count">{filterProductID ? `${filtered.length}/${environments.length}` : environments.length} 个{enrollment ? '客户开通' : '执行环境'}</span><Button onClick={openCreate}><Plus size={15}/>新建</Button></div>
    <div className="admin-config-table">{filtered.length === 0 ? <Empty title={environments.length ? `没有匹配的${enrollment ? '客户开通' : '执行环境'}` : `还没有${enrollment ? '客户开通' : '执行环境'}`} detail={environments.length ? '换一个搜索词或清除产品筛选。' : enrollment ? '选择已发布的创作产品，为第一个客户建立开通记录。' : '先建立一个运行边界，再为它绑定流程规范和本地能力。'} action={emptyAction}/> : filtered.map(environment => {
      const product = publishedSOPs.find(version => version.sop_id === environment.default_sop_id && version.version === environment.default_sop_version);
      const row = <><span className="admin-row-icon">{enrollment ? <Users size={17}/> : <Workflow size={17}/>}</span><span><strong>{environment.name}</strong><small>{enrollment ? `${product?.label || '尚未绑定已发布产品'} · ${environment.capabilities.filter(capability => capability.enabled).length} 项能力` : `${environment.slug} · ${environment.manifest_digest ? `配置摘要：${environment.manifest_digest.slice(0, 18)}…` : '尚未登记配置摘要'}`}</small></span><span className={`config-state ${environment.status === 'active' ? 'is-success' : 'is-warning'}`}>{environment.status === 'active' ? (enrollment ? '已启用' : '运行中') : '已暂停'}</span><span className="admin-row-chevron">›</span></>;
      return environmentPath ? <Link className={`admin-select-row ${!directoryOnly && environment.id === form?.id ? 'is-selected' : ''}`} key={environment.id} to={environmentPath(environment.id)} onClick={() => selectEnvironment(environment.id)}>{row}</Link> : <button className={`admin-select-row ${!directoryOnly && environment.id === form?.id ? 'is-selected' : ''}`} key={environment.id} onClick={() => selectEnvironment(environment.id)}>{row}</button>;
    })}</div>
    {notice && directoryOnly && <div className="workos-notice is-info operations-directory-notice">{notice}</div>}
  </section>;

  return <>
    {directory}
    {!directoryOnly && form && <section className="workos-section admin-editor-section operations-enrollment-editor">
      <header className="workos-section-header"><div><span>{enrollment ? '客户开通配置' : `${workOSTerm.environment}配置`}</span><h2>{form.name}</h2></div><span className={`config-state ${form.status === 'active' ? 'is-success' : 'is-warning'}`}>{form.status === 'active' ? '可创建新任务' : '暂停新任务'}</span></header>
      {enrollment && <div className="operations-enrollment-summary"><span><small>固定产品版本</small><strong>{selectedProduct?.label || '尚未选择'}</strong></span><span><small>能力覆盖</small><strong>{required.length === 0 ? '产品未声明' : `${required.length - missing.length}/${required.length}`}</strong></span><span><small>当前影响</small><strong>{form.status === 'active' ? '允许新任务' : '阻止新任务'}</strong></span></div>}
      <div className="form-grid"><Field label={enrollment ? '客户名称' : '名称'}><input value={form.name} onChange={event => setForm({...form, name: event.target.value})}/></Field><Field label={enrollment ? '客户标识' : '标识'}><input value={form.slug} onChange={event => setForm({...form, slug: event.target.value})}/></Field><Field label="状态"><select value={form.status} onChange={event => setForm({...form, status: event.target.value})}><option value="active">已启用，可创建新任务</option><option value="paused">已暂停，不再创建新任务</option></select></Field><Field label={enrollment ? '已开通创作产品' : `默认${workOSTerm.sop}`}><select value={`${form.default_sop_id || ''}:${form.default_sop_version || 0}`} onChange={event => {const [sopID, version] = event.target.value.split(':'); setForm({...form, default_sop_id: sopID, default_sop_version: Number(version)});}}><option value=":0" disabled>请选择已发布版本</option>{publishedSOPs.map(version => <option key={`${version.sop_id}:${version.version}`} value={`${version.sop_id}:${version.version}`}>{version.label}</option>)}</select></Field></div>
      <div className="admin-capability-list"><strong>{enrollment ? '客户可用能力范围' : '已声明的本地能力'}</strong>{editorCapabilities.length === 0 ? <Empty title="没有已登记能力" detail="能力目录尚未返回可配置项。"/> : editorCapabilities.map(capability => <label className="toggle-line" key={capability.id}><span><strong>{capability.id}</strong><small>v{capability.version}{required.includes(capability.id) ? ' · 当前产品需要' : ''}</small></span><input type="checkbox" checked={enabled.has(capability.id)} onChange={event => updateCapability(capability.id, capability.version, event.target.checked)}/></label>)}</div>
      {enrollment && <div className={`operations-enrollment-impact ${missing.length ? 'is-warning' : 'is-info'}`}>{missing.length ? <CircleAlert size={17}/> : <CircleCheck size={17}/>}<div><strong>{missing.length ? `缺少 ${missing.length} 项产品所需能力` : '当前能力范围覆盖产品声明'}</strong><span>{missing.length ? `${missing.join('、')} 尚未启用，后续任务可能无法完成对应步骤。` : '保存后只影响新任务；历史任务、审批和创作结果不会被改写。'}</span></div></div>}
      <div className="new-task-actions"><Button onClick={save} disabled={saving || !form.default_sop_id || (form.default_sop_version || 0) < 1}><Check size={15}/>{saving ? '保存中…' : enrollment ? '保存客户开通' : '保存执行环境'}</Button></div>{notice && <div className="workos-notice is-info">{notice}</div>}
    </section>}
    {createOpen && (enrollment ? <Modal title="新建客户开通" onClose={closeCreate}><div className="admin-modal-form operations-enrollment-wizard"><ol className="operations-enrollment-steps" aria-label="客户开通步骤">{['客户','产品版本','能力范围','确认影响'].map((label,index) => <li className={createStep === index + 1 ? 'is-active' : createStep > index + 1 ? 'is-complete' : ''} key={label}><span>{index + 1}</span><small>{label}</small></li>)}</ol>
      {createStep === 1 && <div className="operations-wizard-panel"><Field label="客户名称"><input autoFocus value={createForm.name} onChange={event => setCreateForm({...createForm, name: event.target.value})} placeholder="例如：青云品牌团队"/></Field><Field label="客户标识" hint="用于稳定识别客户开通记录，不会展示给终端客户。"><input value={createForm.slug} onChange={event => setCreateForm({...createForm, slug: event.target.value})} placeholder="例如：qingyun-brand"/></Field></div>}
      {createStep === 2 && <div className="operations-wizard-panel"><Field label="已发布创作产品"><select autoFocus value={`${createForm.default_sop_id}:${createForm.default_sop_version}`} onChange={event => selectCreateProduct(event.target.value)}><option value=":0" disabled>请选择已发布版本</option>{publishedSOPs.map(version => <option key={`${version.sop_id}:${version.version}`} value={`${version.sop_id}:${version.version}`}>{version.label}</option>)}</select></Field>{publishedSOPs.length === 0 && <div className="operations-enrollment-impact is-warning"><CircleAlert size={17}/><div><strong>没有可开通的产品版本</strong><span>先发布一个创作产品版本，再建立客户开通。</span></div></div>}</div>}
      {createStep === 3 && <div className="operations-wizard-panel"><div className="admin-capability-list"><strong>客户可用能力范围</strong>{capabilityOptions(availableCapabilities, createForm.capabilities).length === 0 ? <Empty title="没有已登记能力" detail="可以继续创建，但对应产品步骤可能无法执行。"/> : capabilityOptions(availableCapabilities, createForm.capabilities).map(capability => <label className="toggle-line" key={capability.id}><span><strong>{capability.id}</strong><small>v{capability.version}{requiredCapabilityIDs(sops, createForm.default_sop_id, createForm.default_sop_version).includes(capability.id) ? ' · 当前产品需要' : ''}</small></span><input type="checkbox" checked={createForm.capabilities.some(value => value.id === capability.id && value.enabled)} onChange={event => updateCreateCapability(capability.id, capability.version, event.target.checked)}/></label>)}</div></div>}
      {createStep === 4 && <div className="operations-wizard-panel"><Field label="启用状态"><select autoFocus value={createForm.status} onChange={event => setCreateForm({...createForm, status: event.target.value})}><option value="active">立即启用，允许创建新任务</option><option value="paused">先保存为暂停，不允许创建新任务</option></select></Field><div className="operations-enrollment-review"><span><small>客户</small><strong>{createForm.name}</strong></span><span><small>创作产品</small><strong>{publishedSOPs.find(version => version.sop_id === createForm.default_sop_id && version.version === createForm.default_sop_version)?.label || '尚未选择'}</strong></span><span><small>能力覆盖</small><strong>{createRequired.length===0?'产品未声明':`${createRequired.length-createMissing.length}/${createRequired.length}`}</strong></span></div><div className={`operations-enrollment-impact ${createMissing.length?'is-warning':'is-info'}`}>{createMissing.length?<CircleAlert size={17}/>:<CircleCheck size={17}/>}<div><strong>{createMissing.length?`缺少 ${createMissing.length} 项产品所需能力`:'开通只约束后续新任务'}</strong><span>{createMissing.length?`${createMissing.join('、')} 尚未启用；可以先保存为暂停，补齐能力后再启用。`:'暂停或修改开通不会隐藏历史任务、审批记录和创作结果。'}</span></div></div></div>}
      <footer className="modal-actions operations-wizard-actions"><Button variant="secondary" onClick={createStep === 1 ? closeCreate : () => setCreateStep(step => step - 1)}>{createStep === 1 ? '取消' : <><ArrowLeft size={15}/>上一步</>}</Button>{createStep < 4 ? <Button onClick={() => setCreateStep(step => step + 1)} disabled={(createStep === 1 && (!createForm.name.trim() || !createForm.slug.trim())) || (createStep === 2 && (!createForm.default_sop_id || createForm.default_sop_version < 1))}>下一步<ArrowRight size={15}/></Button> : <Button onClick={create} disabled={saving}><Plus size={15}/>{saving ? '开通中…' : '确认开通'}</Button>}</footer>
    </div></Modal> : <Modal title="新建执行环境" onClose={closeCreate}><div className="admin-modal-form"><Field label="名称"><input autoFocus value={createForm.name} onChange={event => setCreateForm({...createForm, name: event.target.value})} placeholder="例如：内容审核环境"/></Field><Field label="标识"><input value={createForm.slug} onChange={event => setCreateForm({...createForm, slug: event.target.value})} placeholder="例如：review"/></Field><Field label="状态"><select value={createForm.status} onChange={event => setCreateForm({...createForm, status: event.target.value})}><option value="active">运行中，创建后立即可用</option><option value="paused">已暂停，创建后暂不启用</option></select></Field><Field label={`默认${workOSTerm.sop}`}><select value={`${createForm.default_sop_id}:${createForm.default_sop_version}`} onChange={event => selectCreateProduct(event.target.value)}><option value=":0" disabled>请选择默认流程规范</option>{publishedSOPs.map(version => <option key={`${version.sop_id}:${version.version}`} value={`${version.sop_id}:${version.version}`}>{version.label}</option>)}</select></Field><footer className="modal-actions"><Button variant="secondary" onClick={closeCreate}>取消</Button><Button onClick={create} disabled={saving || !createForm.name.trim() || !createForm.slug.trim() || !createForm.default_sop_id || createForm.default_sop_version < 1}><Plus size={15}/>{saving ? '创建中…' : '创建执行环境'}</Button></footer></div></Modal>)}
  </>;
}

interface AdminSOPPanelProps {
  sops:SOPSummary[];
  onChanged:()=>Promise<void>|void;
  onPublished?:(sopID:string,version:number)=>void;
  initialSOPID?:string;
  initialVersion?:number;
  scoped?:boolean;
  onSelectionChange?:(sopID:string,version:number)=>void;
}

export function AdminSOPPanel({sops, onChanged, onPublished, initialSOPID, initialVersion=0, scoped=false, onSelectionChange}: AdminSOPPanelProps) {
  const [selectedSOPID, setSelectedSOPID] = useState(initialSOPID || sops[0]?.definition.id || '');
  const summary = sops.find(value => value.definition.id === selectedSOPID) || sops[0];
  const [selectedVersion, setSelectedVersion] = useState(initialVersion);
  const version = summary?.versions.find(value => value.version === selectedVersion) || summary?.versions[0];
  const [draft, setDraft] = useState<SOPVersion | undefined>(version);
  const [lint, setLint] = useState<SOPLintReport>();
  const [compareVersion, setCompareVersion] = useState(0);
  const [diff, setDiff] = useState<SOPVersionDiff>();
  const [impact, setImpact] = useState<SOPVersionImpact>();
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState(emptyAdminProductDraft);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<Notice>('');
  const selectVersion = (sopID:string, version:number) => { setSelectedSOPID(sopID); setSelectedVersion(version); onSelectionChange?.(sopID, version); };
  useEffect(() => { if (initialSOPID && initialSOPID !== selectedSOPID && sops.some(value => value.definition.id === initialSOPID)) setSelectedSOPID(initialSOPID); }, [initialSOPID, selectedSOPID, sops]);
  useEffect(() => { if (initialVersion > 0 && initialVersion !== selectedVersion && summary?.versions.some(value => value.version === initialVersion)) setSelectedVersion(initialVersion); }, [initialVersion, selectedVersion, summary]);
  useEffect(() => { setDraft(version); setLint(undefined); setDiff(undefined); setImpact(undefined); setCompareVersion(summary?.versions.find(candidate => candidate.version !== version?.version)?.version || 0); }, [selectedSOPID, selectedVersion, sops]);
  const createSOP = async () => {
    setBusy(true); setNotice('');
    try {
      const created = await createAdminProduct(createForm);
      setCreateOpen(false); setCreateForm(emptyAdminProductDraft()); setNotice(`创作产品「${created.definition.name}」已创建为 v1 草稿。`); await onChanged(); selectVersion(created.definition.id, 1);
    } catch (value) { setNotice(value instanceof Error ? value.message : '流程规范创建失败'); }
    finally { setBusy(false); }
  };
  if (!summary || !draft) return <section className="workos-section admin-empty-action"><Empty title="还没有创作产品" detail="建立第一套创作产品后，客户任务就可以使用已发布版本。" action={<Button onClick={() => setCreateOpen(true)}><Plus size={15}/>新建创作产品</Button>}/>{createOpen && <CreateProductModal form={createForm} setForm={setCreateForm} busy={busy} onClose={() => setCreateOpen(false)} onCreate={createSOP}/>}</section>;
  const editable = draft.status === 'draft';
  const updateDraft = (next: SOPVersion) => { setDraft(next); setLint(undefined); };
  const createDraft = async () => { setBusy(true); setNotice(''); try { const created = normalizeSOPVersion(await post<SOPVersion>(`/api/bff/admin/sops/${encodeURIComponent(summary.definition.id)}/versions`, {source_version: draft.version})); setNotice(`已创建 v${created.version} 草稿。`); await onChanged(); selectVersion(created.sop_id, created.version); } catch (value) { setNotice(value instanceof Error ? value.message : '草稿创建失败'); } finally { setBusy(false); } };
  const cloneAsCustom = async () => { setBusy(true); setNotice(''); try { const created = normalizeSOPSummary(await post<SOPSummary>('/api/bff/admin/sops', {name: `${draft.name}（自定义）`, description: draft.description, content_types: draft.content_types, stages: draft.stages, gates: draft.gates, default_execution_mode: draft.default_execution_mode})); setNotice(`已复制为「${created.definition.name}」草稿，可继续修改。`); await onChanged(); selectVersion(created.definition.id, 1); } catch (value) { setNotice(value instanceof Error ? value.message : '复制创作产品失败'); } finally { setBusy(false); } };
  const saveDraft = async () => { if (!editable) return; setBusy(true); setNotice(''); try { const saved = normalizeSOPVersion(await patch<SOPVersion>(`/api/bff/admin/sops/${encodeURIComponent(draft.sop_id)}/versions/${draft.version}`, {name: draft.name, description: draft.description, content_types: draft.content_types, stages: draft.stages, gates: draft.gates, default_execution_mode: draft.default_execution_mode})); setDraft(saved); setNotice('流程规范草稿已保存。'); await onChanged(); } catch (value) { setNotice(value instanceof Error ? value.message : '流程规范草稿保存失败'); } finally { setBusy(false); } };
  const runLint = async () => { setBusy(true); setNotice(''); try { setLint(await api<SOPLintReport>(`/api/bff/admin/sops/${encodeURIComponent(draft.sop_id)}/versions/${draft.version}/lint`)); } catch (value) { setNotice(value instanceof Error ? value.message : '流程规范检查失败'); } finally { setBusy(false); } };
  const publish = async () => { setBusy(true); setNotice(''); try { const report = await api<SOPLintReport>(`/api/bff/admin/sops/${encodeURIComponent(draft.sop_id)}/versions/${draft.version}/lint`); setLint(report); if (!report.valid) { setNotice('发布前检查未通过，请先修正配置。'); return; } const published = normalizeSOPVersion(await post<SOPVersion>(`/api/bff/admin/sops/${encodeURIComponent(draft.sop_id)}/versions/${draft.version}/publish`)); setDraft(published); setNotice(`流程规范 v${published.version} 已发布，配置摘要已固定。`); await onChanged(); onPublished?.(published.sop_id, published.version); } catch (value) { setNotice(value instanceof Error ? value.message : '流程规范发布失败'); } finally { setBusy(false); } };
  const compare = async () => { if (!compareVersion) return; setBusy(true); setNotice(''); try { setDiff(await api<SOPVersionDiff>(`/api/bff/admin/sops/${encodeURIComponent(draft.sop_id)}/versions/${compareVersion}/diff/${draft.version}`)); } catch (value) { setNotice(value instanceof Error ? value.message : '版本比较失败'); } finally { setBusy(false); } };
  const loadImpact = async () => { setBusy(true); setNotice(''); try { setImpact(await api<SOPVersionImpact>(`/api/bff/admin/sops/${encodeURIComponent(draft.sop_id)}/versions/${draft.version}/impact`)); } catch (value) { setNotice(value instanceof Error ? value.message : '影响分析失败'); } finally { setBusy(false); } };
  const rollback = async () => { const target = summary.versions.find(candidate => candidate.version === compareVersion); if (!target || compareVersion >= summary.definition.current_version || !window.confirm(`确认回滚到 v${compareVersion}？历史执行记录不会改变，绑定当前版本的未来执行环境和项目会切换到新版本。`)) return; setBusy(true); setNotice(''); try { const result = await post<{version:SOPVersion; rebound_environments:number; rebound_projects:number}>(`/api/bff/admin/sops/${encodeURIComponent(draft.sop_id)}/rollback`, {target_version: compareVersion}); setSelectedVersion(result.version.version); setNotice(`已创建回滚版本 v${result.version.version}，切换 ${result.rebound_environments} 个执行环境、${result.rebound_projects} 个项目。`); await onChanged(); } catch (value) { setNotice(value instanceof Error ? value.message : '流程规范回滚失败'); } finally { setBusy(false); } };
  const retire = async () => { if (draft.status !== 'published' || draft.version === summary.definition.current_version || !window.confirm(`确认停用流程规范 v${draft.version}？已绑定的历史任务仍可只读完成。`)) return; setBusy(true); setNotice(''); try { await post(`/api/bff/admin/sops/${encodeURIComponent(draft.sop_id)}/versions/${draft.version}/retire`, {}); setNotice(`流程规范 v${draft.version} 已停用，新任务不能再绑定它。`); await onChanged(); } catch (value) { setNotice(value instanceof Error ? value.message : '流程规范停用失败'); } finally { setBusy(false); } };
  const updateStage = (index: number, changes: Partial<StageDefinition>) => updateDraft({...draft, stages: draft.stages.map((stage, stageIndex) => stageIndex === index ? {...stage, ...changes} : stage)});
  const addStage = () => updateDraft({...draft, stages: [...draft.stages, {stage_id: `stage-${draft.stages.length + 1}`, name: '新流程阶段', order: nextOrder(draft.stages), owner_roles: [], input_refs: [], output_schema: 'contentcloud.output/1.0', output_schema_refs: [], accepted_input_types: [], required_output_types: [], required_capabilities: [], execution_modes: [draft.default_execution_mode || 'local'], checks: [], gate_ids: [], retry_max_attempts: 0, retry_policy: {}, cost_policy: {}}]});
  const removeStage = (index: number) => updateDraft({...draft, stages: draft.stages.filter((_, stageIndex) => stageIndex !== index).map((stage, stageIndex) => ({...stage, order: (stageIndex + 1) * 10}))});
  const moveStage = (index: number, direction: -1 | 1) => { const target = index + direction; if (target < 0 || target >= draft.stages.length) return; const stages = [...draft.stages]; [stages[index], stages[target]] = [stages[target], stages[index]]; updateDraft({...draft, stages: stages.map((stage, stageIndex) => ({...stage, order: (stageIndex + 1) * 10}))}); };
  const addGate = () => updateDraft({...draft, gates: [...draft.gates, {gate_id: `gate-${draft.gates.length + 1}`, name: '新检查项', mode: 'advisory', blocking: false, assignee_roles: [], input_refs: [], checks: [], on_reject: 'changes_requested', escalation_hours: 0}]});
  const updateGate = (index: number, changes: Partial<GateDefinition>) => updateDraft({...draft, gates: draft.gates.map((gate, gateIndex) => gateIndex === index ? {...gate, ...changes} : gate)});
  const removeGate = (index: number) => updateDraft({...draft, gates: draft.gates.filter((_, gateIndex) => gateIndex !== index).map(gate => ({...gate, blocking: ['required_check', 'internal_review', 'client_decision'].includes(gate.mode)}))});
  return <>
    <section className="workos-section admin-config-section"><div className="admin-config-toolbar">{scoped?<span className="operations-muted">选择要查看或维护的产品版本</span>:<label className="search-field"><GitBranch size={16}/><select aria-label="选择创作产品" value={summary.definition.id} onChange={event => selectVersion(event.target.value, 0)}>{sops.map(value => <option key={value.definition.id} value={value.definition.id}>{value.definition.name}{value.definition.built_in ? ' · 内置模板' : ''}</option>)}</select></label>}<span className="toolbar-count">{summary.versions.length} 个版本</span>{!scoped&&<Button onClick={() => setCreateOpen(true)}><Plus size={15}/>新建创作产品</Button>}</div><div className="admin-version-tabs">{summary.versions.map(candidate => <button key={candidate.version} className={candidate.version === draft.version ? 'is-active' : ''} onClick={() => selectVersion(summary.definition.id, candidate.version)}>v{candidate.version} <small>{statusLabel(candidate.status)}</small></button>)}</div></section>
    <section className="workos-section admin-editor-section"><header className="workos-section-header"><div><span>创作产品版本</span><h2>{draft.name} · v{draft.version}</h2><small className="config-source-note">{summary.definition.built_in ? `内置产品 · ${summary.definition.source_ref || 'Content Work OS'}` : '平台自定义创作产品'}</small></div>{editable ? <span className="config-state is-warning">可编辑草稿</span> : <span className="config-state is-success">已发布，不可直接修改</span>}</header><div className="form-grid"><Field label="名称"><input value={draft.name} readOnly={!editable} onChange={event => updateDraft({...draft, name: event.target.value})}/></Field><Field label="默认执行方式"><select value={draft.default_execution_mode} disabled={!editable} onChange={event => updateDraft({...draft, default_execution_mode: event.target.value})}><option value="local">{executionModeLabel('local')}</option><option value="agent">{executionModeLabel('agent')}</option></select></Field></div><Field label="说明"><textarea value={draft.description} readOnly={!editable} onChange={event => updateDraft({...draft, description: event.target.value})} rows={3}/></Field><div className="admin-sop-stage-editor"><div className="admin-editor-heading"><strong>客户流程</strong>{editable && <Button variant="secondary" onClick={addStage}><Plus size={14}/>添加流程步骤</Button>}</div>{draft.stages.length === 0 ? <Empty title="还没有客户流程" detail="至少添加一个流程步骤才能发布产品版本。"/> : draft.stages.map((stage, index) => <StageEditor key={`${stage.stage_id}:${index}`} stage={stage} index={index} editable={editable} canMoveUp={index > 0} canMoveDown={index < draft.stages.length - 1} onChange={changes => updateStage(index, changes)} onMove={direction => moveStage(index, direction)} onRemove={() => removeStage(index)} gates={draft.gates}/>)}</div><div className="admin-gate-editor"><div className="admin-editor-heading"><strong>检查与确认</strong>{editable && <Button variant="secondary" onClick={addGate}><Plus size={14}/>添加确认点</Button>}</div>{draft.gates.length === 0 ? <Empty title="没有检查与确认" detail="确认不是必选步骤；不设置确认点也是合法配置。"/> : draft.gates.map((gate, index) => <GateEditor key={`${gate.gate_id}:${index}`} gate={gate} editable={editable} onChange={changes => updateGate(index, changes)} onRemove={() => removeGate(index)}/>)}</div><div className="new-task-actions">{editable ? <><Button variant="secondary" onClick={saveDraft} disabled={busy}><Check size={15}/>保存草稿</Button><Button variant="secondary" onClick={runLint} disabled={busy}><CircleCheck size={15}/>{busy ? '检查中…' : '运行发布检查'}</Button><Button onClick={publish} disabled={busy}><GitBranch size={15}/>发布版本</Button></> : <>{summary.definition.built_in ? <Button variant="secondary" onClick={cloneAsCustom} disabled={busy}><Copy size={15}/>复制为自定义产品</Button> : <Button onClick={createDraft} disabled={busy}><GitBranch size={15}/>创建新版本</Button>}{draft.status === 'published' && draft.version !== summary.definition.current_version && <Button variant="secondary" onClick={retire} disabled={busy}><Archive size={15}/>停用版本</Button>}</>}</div>{lint && <LintPanel report={lint}/>} {notice && <div className="workos-notice is-info">{notice}</div>}</section>
    <section className="workos-section sop-governance-panel"><header className="workos-section-header"><div><span>版本治理</span><h2>比较、影响和回滚</h2><small className="config-source-note">历史任务固定原版本；显式回滚只切换当前绑定的未来工作。</small></div><ShieldAlert size={18}/></header><div className="form-grid"><Field label="比较 / 回滚目标"><select value={compareVersion} onChange={event => {setCompareVersion(Number(event.target.value)); setDiff(undefined);}}><option value={0}>选择历史版本</option>{summary.versions.filter(candidate => candidate.version !== draft.version).map(candidate => <option key={candidate.version} value={candidate.version}>v{candidate.version} · {statusLabel(candidate.status)}</option>)}</select></Field><div className="new-task-actions sop-governance-actions"><Button variant="secondary" onClick={compare} disabled={busy || !compareVersion}><GitCompare size={15}/>比较版本</Button><Button variant="secondary" onClick={loadImpact} disabled={busy}><ShieldAlert size={15}/>查看影响</Button>{draft.status === 'published' && compareVersion > 0 && compareVersion < summary.definition.current_version && <Button onClick={rollback} disabled={busy}><RotateCcw size={15}/>回滚到 v{compareVersion}</Button>}</div></div>{diff && <SOPDiffPanel diff={diff}/>} {impact && <SOPImpactPanel impact={impact}/>}</section>
    {createOpen && <CreateProductModal form={createForm} setForm={setCreateForm} busy={busy} onClose={() => setCreateOpen(false)} onCreate={createSOP}/>}
  </>;
}

function SOPDiffPanel({diff}: {diff: SOPVersionDiff}) { return <div className={`sop-diff-panel ${diff.same ? 'is-same' : ''}`}><div className="sop-governance-heading"><GitCompare size={15}/><strong>v{diff.from_version} → v{diff.to_version}</strong><span>{diff.same ? '没有结构变化' : `${diff.changes.length} 项变化`}</span></div>{diff.changes.length > 0 && <div className="sop-diff-list">{diff.changes.map((change: SOPDiffChange) => <div className="sop-diff-row" key={change.path}><code>{change.path}</code><span>{formatDiffValue(change.before)}</span><ArrowRightGlyph/><span>{formatDiffValue(change.after)}</span></div>)}</div>}</div>; }
function SOPImpactPanel({impact}: {impact: SOPVersionImpact}) { return <div className="sop-impact-panel"><div className="sop-governance-heading"><ShieldAlert size={15}/><strong>v{impact.version} 影响范围</strong><span>{impact.counts.tasks || 0} 个任务 · {impact.counts.projects || 0} 个项目 · {impact.counts.environments || 0} 个执行环境</span></div><div className="sop-impact-grid"><div><strong>执行环境</strong>{impact.environments.length === 0 ? <small>没有默认绑定</small> : impact.environments.slice(0, 4).map(item => <small key={item.environment_id}>{item.name} · {statusLabel(item.status)}</small>)}</div><div><strong>项目</strong>{impact.projects.length === 0 ? <small>没有固定绑定</small> : impact.projects.slice(0, 4).map(item => <small key={item.project_id}>{item.name} · v{item.bound_sop_version}</small>)}</div><div><strong>任务</strong>{impact.tasks.length === 0 ? <small>没有历史任务</small> : impact.tasks.slice(0, 4).map(item => <small key={item.task_id}>{item.title} · {statusLabel(item.status)}</small>)}</div></div></div>; }
function ArrowRightGlyph() { return <span className="sop-diff-arrow" aria-hidden="true">→</span>; }
function formatDiffValue(value: unknown) { if (value === null || value === undefined) return '不存在'; if (typeof value === 'string') return value || '空字符串'; try { const serialized = JSON.stringify(value); return serialized.length > 180 ? `${serialized.slice(0, 177)}…` : serialized; } catch { return String(value); } }

export function CreateProductModal({form,setForm,busy,notice='',onClose,onCreate}:{form:AdminProductDraftForm;setForm:(value:AdminProductDraftForm)=>void;busy:boolean;notice?:string;onClose:()=>void;onCreate:()=>Promise<void>}) {
  return <Modal title="新建创作产品" onClose={onClose}><div className="admin-modal-form"><Field label="名称"><input autoFocus value={form.name} onChange={event=>setForm({...form,name:event.target.value})} placeholder="例如：新品内容生产"/></Field><Field label="用途和客户结果"><textarea value={form.description} onChange={event=>setForm({...form,description:event.target.value})} rows={3} placeholder="说明适用场景，以及客户最终会得到什么。"/></Field><Field label="内容类型"><select value={form.content_type} onChange={event=>setForm({...form,content_type:event.target.value})}><option value="marketing_video">营销视频</option><option value="video_script">视频脚本</option><option value="wechat_article">微信公众号文章</option></select></Field>{notice&&<div className="workos-notice is-info">{notice}</div>}<footer className="modal-actions"><Button variant="secondary" onClick={onClose}>取消</Button><Button onClick={onCreate} disabled={busy||!form.name.trim()}><Plus size={15}/>{busy?'创建中…':'创建创作产品'}</Button></footer></div></Modal>;
}

function StageEditor({stage, index, editable, canMoveUp, canMoveDown, onChange, onMove, onRemove, gates}: {stage: StageDefinition; index: number; editable: boolean; canMoveUp: boolean; canMoveDown: boolean; onChange: (changes: Partial<StageDefinition>) => void; onMove: (direction: -1 | 1) => void; onRemove: () => void; gates: GateDefinition[]}) {
  return <article className="admin-stage-editor-row">
    <div className="admin-stage-editor-top"><span className="stage-order">{index + 1}</span><strong>{stage.name || '未命名流程阶段'}</strong><div className="admin-stage-editor-actions">{editable && <><button className="icon-button" aria-label="上移流程阶段" title="上移流程阶段" disabled={!canMoveUp} onClick={() => onMove(-1)}><ArrowUp size={14}/></button><button className="icon-button" aria-label="下移流程阶段" title="下移流程阶段" disabled={!canMoveDown} onClick={() => onMove(1)}><ArrowDown size={14}/></button><button className="icon-button is-danger" aria-label="删除流程阶段" title="删除流程阶段" onClick={onRemove}><Trash2 size={14}/></button></>}</div></div>
    <div className="form-grid">
      <Field label="阶段标识（ID）"><input value={stage.stage_id} readOnly={!editable} onChange={event => onChange({stage_id: event.target.value})}/></Field>
      <Field label="名称"><input value={stage.name} readOnly={!editable} onChange={event => onChange({name: event.target.value})}/></Field>
      <Field label="输出格式标识"><input value={stage.output_schema} readOnly={!editable} onChange={event => onChange({output_schema: event.target.value})}/></Field>
      <Field label="负责人角色"><input value={editable ? joinList(stage.owner_roles) : stage.owner_roles.map(roleLabel).join('、')} readOnly={!editable} onChange={event => onChange({owner_roles: splitList(event.target.value)})}/></Field>
      <Field label="输入引用"><input value={joinList(stage.input_refs)} readOnly={!editable} onChange={event => onChange({input_refs: splitList(event.target.value)})}/></Field>
      <Field label="所需能力"><input value={joinList(stage.required_capabilities)} readOnly={!editable} onChange={event => onChange({required_capabilities: splitList(event.target.value)})}/></Field>
      <Field label="执行方式"><input value={editable ? joinList(stage.execution_modes) : stage.execution_modes.map(executionModeLabel).join('、')} readOnly={!editable} onChange={event => onChange({execution_modes: splitList(event.target.value)})}/></Field>
      <Field label="检查规则"><input value={editable ? joinList(stage.checks) : stage.checks.map(workflowCheckLabel).join('、')} readOnly={!editable} onChange={event => onChange({checks: splitList(event.target.value)})}/></Field>
      <Field label="重试次数"><input type="number" min={0} max={10} value={stage.retry_max_attempts} readOnly={!editable} onChange={event => onChange({retry_max_attempts: Math.max(0, Number(event.target.value) || 0)})}/></Field>
      <Field label="关联检查项"><select multiple value={stage.gate_ids} disabled={!editable} onChange={event => onChange({gate_ids: Array.from(event.target.selectedOptions, option => option.value)})}>{gates.map(gate => <option key={gate.gate_id} value={gate.gate_id}>{gate.name} · {gateModeLabel(gate.mode)}</option>)}</select></Field>
    </div>
  </article>;
}

function GateEditor({gate, editable, onChange, onRemove}: {gate: GateDefinition; editable: boolean; onChange: (changes: Partial<GateDefinition>) => void; onRemove: () => void}) {
  return <article className="admin-gate-editor-row">
    <div className="admin-editor-heading"><div><strong>{gate.name || '未命名检查项'}</strong><small>{gate.gate_id} · {gateModeBlockingLabel(gate.blocking)}</small></div>{editable && <button className="icon-button is-danger" aria-label="删除检查项" title="删除检查项" onClick={onRemove}><Trash2 size={14}/></button>}</div>
    <div className="form-grid">
      <Field label="检查项标识（ID）"><input value={gate.gate_id} readOnly={!editable} onChange={event => onChange({gate_id: event.target.value})}/></Field>
      <Field label="名称"><input value={gate.name} readOnly={!editable} onChange={event => onChange({name: event.target.value})}/></Field>
      <Field label="处理方式"><select value={gate.mode} disabled={!editable} onChange={event => {const mode = event.target.value as GateDefinition['mode']; onChange({mode, blocking: ['internal_review', 'client_decision'].includes(mode)});}}><option value="none">不设置审批</option><option value="required_check">必做检查</option><option value="advisory">可选建议</option><option value="internal_review">内部审核</option><option value="client_decision">客户确认</option></select></Field>
      <Field label="处理人角色"><input value={editable ? joinList(gate.assignee_roles) : gate.assignee_roles.map(roleLabel).join('、')} readOnly={!editable} onChange={event => onChange({assignee_roles: splitList(event.target.value)})}/></Field>
      <Field label="检查规则"><input value={editable ? joinList(gate.checks) : gate.checks.map(workflowCheckLabel).join('、')} readOnly={!editable} onChange={event => onChange({checks: splitList(event.target.value)})}/></Field>
      <Field label="拒绝后的动作"><input value={editable ? gate.on_reject : commonStatusLabel(gate.on_reject)} readOnly={!editable} onChange={event => onChange({on_reject: event.target.value})}/></Field>
      <Field label="超时提醒（小时）"><input type="number" min={0} max={720} value={gate.escalation_hours} readOnly={!editable} onChange={event => onChange({escalation_hours: Math.max(0, Number(event.target.value) || 0)})}/></Field>
    </div>
  </article>;
}

function LintPanel({report}: {report: SOPLintReport}) {
  return <div className={`admin-lint-panel ${report.valid ? 'is-valid' : 'is-invalid'}`}><div className="admin-lint-title">{report.valid ? <CircleCheck size={16}/> : <CircleAlert size={16}/>}<strong>{report.valid ? '发布检查通过' : '发布检查未通过'}</strong><span>{report.errors.length} 个错误 · {report.warnings.length} 个提醒</span></div>{report.errors.length > 0 && <ul>{report.errors.map(issue => <li key={`${issue.code}:${issue.path}`}><strong>{issue.path}</strong>{issue.message}</li>)}</ul>}{report.warnings.length > 0 && <ul className="is-warning">{report.warnings.map(issue => <li key={`${issue.code}:${issue.path}`}><strong>{issue.path}</strong>{issue.message}</li>)}</ul>}</div>;
}
