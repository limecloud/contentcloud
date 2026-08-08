import { renderToStaticMarkup } from 'react-dom/server';
import { Route, Routes, StaticRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AdminWorkOSView, OperationsExecutorDirectory, OperationsSkillDirectory, PlatformOverview, Session } from '../types';

const adminState=vi.hoisted(()=>({value:{} as Record<string,unknown>}));

vi.mock('./context',()=>({useAdmin:()=>adminState.value}));

import { AdminShell } from './AdminShell';
import { AdminCapabilityCatalogPage, AdminCapabilityDetailPage, AdminCustomerDetailPage, AdminCustomersPage, AdminExecutorDetailPage, AdminExecutorsPage, AdminOperationsOverview, AdminProductDetailPage, AdminProductReleasesPage, AdminProductsPage, AdminReleaseResultPage, AdminSkillDetailPage, AdminSkillsPage } from './views/AdminOperationsPages';

const session:Session={user:{id:'user-1',email:'operator@example.com',display_name:'运营人员'},tenant:{id:'tenant-1',name:'平台运营',slug:'platform',status:'active',created_at:'2026-08-01T08:00:00Z'},role:'admin',is_platform_admin:true};
const overview:PlatformOverview={counts:{tenants:1,active_tenants:1,users:1,projects:1,online_devices:1,active_runs:1},tenants:[],users:[],generated_at:'2026-08-08T02:30:00Z'};

const workOS:AdminWorkOSView={
  environments:[{id:'environment-1',tenant_id:'tenant-1',name:'果木食品创作端',slug:'guomu',status:'active',manifest_digest:'digest',default_sop_id:'ip-video',default_sop_version:2,capabilities:[{id:'storyboard_generation',version:'1.0.0',enabled:true}],created_at:'2026-08-01T08:00:00Z',updated_at:'2026-08-08T02:00:00Z'},{id:'environment-2',tenant_id:'tenant-1',name:'洞察研究创作端',slug:'insight',status:'paused',manifest_digest:'digest-2',default_sop_id:'other-product',default_sop_version:1,capabilities:[{id:'inspiration_collection',version:'1.0.0',enabled:true}],created_at:'2026-08-02T08:00:00Z',updated_at:'2026-08-08T01:00:00Z'}],
  sops:[{definition:{id:'ip-video',tenant_id:'platform',name:'IP 人设视频',description:'从人物原型到视频交付',content_types:['marketing_video'],current_version:2,created_by:'operator',created_at:'2026-08-01T08:00:00Z',updated_at:'2026-08-08T02:00:00Z'},versions:[{id:'ip-video-v2',tenant_id:'platform',sop_id:'ip-video',version:2,schema_version:'1',name:'IP 人设视频',description:'正式发布版本',content_types:['marketing_video'],stages:[{stage_id:'inspiration',name:'灵感采集',order:10,owner_roles:[],input_refs:[],output_schema:'contentcloud.output/1.0',output_schema_refs:[],required_capabilities:['inspiration_collection'],execution_modes:['managed'],checks:[],gate_ids:[],retry_max_attempts:0,accepted_input_types:[],required_output_types:[],retry_policy:{},cost_policy:{}}],gates:[],default_execution_mode:'managed',digest:'version-digest',status:'published',created_by:'operator',published_by:'operator',created_at:'2026-08-08T01:00:00Z',published_at:'2026-08-08T02:00:00Z'}]}],
  gates:[{sop_id:'ip-video',sop_name:'IP 人设视频',sop_version:2,gate_id:'client-review',name:'客户确认',mode:'client_decision',blocking:true,usage_count:1}],
  capabilities:[{id:'inspiration_collection',version:'1.0.0',kind:'search',input_schema:'contentcloud.inspiration-query/1.0',output_schema:'contentcloud.inspiration-result/1.0',presentation_profiles:['candidate-list'],local_only:false,digest:'capability-digest'}],
  audit:[{id:'audit-1',action:'sop_version_published',subject_type:'sop_version',subject_id:'ip-video-v2',summary:{},created_at:'2026-08-08T02:00:00Z'}],
  usage:{task_count:12,running_count:1,waiting_gate_count:2,by_execution_mode:{managed:12}},
  generated_at:'2026-08-08T02:30:00Z'
};

const executorDirectory:OperationsExecutorDirectory={executors:[{id:'executor-1',tenant_id:'tenant-1',display_name:'分镜工作站',executor_type:'contentcloud_device',status:'online',status_reason:'heartbeat_recent',hostname:'storyboard.local',platform:'darwin',arch:'arm64',version:'0.20.0',capabilities:[{id:'inspiration_collection',version:'1.0.0',kind:'business_capability',input_schema:'contentcloud.inspiration-query/1.0',output_schema:'contentcloud.inspiration-result/1.0',presentation_profiles:['candidate-list'],local_only:true,digest:'capability-digest'}],projects:[{id:'project-1',brand_name:'果木食品',product_name:'品牌短片',status:'active'}],last_seen_at:'2026-08-08T02:29:00Z'}],generated_at:'2026-08-08T02:30:00Z',online_window_seconds:120};
const skillDirectory:OperationsSkillDirectory={configured:true,source:'verified_plugin_registry',registry_schema_version:'1.0',generated_at:'2026-08-08T02:30:00Z',skills:[{id:'contentcloud-script-writing',version:'1.2.0',digest:'sha256:skill',kind:'skill_pack',lifecycle:'published',available_for_new_runs:true,source:{repository:'https://github.com/limecloud/contentcloud',ref:'v1.2.0',license:'Apache-2.0'},signature:{status:'verified',algorithm:'ed25519',key_id:'plugin-release'},compatible_profiles:['contentcloud.video-production'],permissions:['workspace:read'],data_flow:{local_by_default:true,cloud_actions:[]},cost:{model:'included',notice:'Included in subscription.'},output_schemas:['contracts/content-item-3.0.schema.json'],evaluation:{status:'passed',report:'.agents/plugins/evaluations/script.json',digest:'sha256:evaluation',evidence:['contract-tests']},revocation:{status:'active'}}]};

function setAdminView(nextWorkOS:AdminWorkOSView=workOS,nextExecutors:OperationsExecutorDirectory=executorDirectory,nextSkills:OperationsSkillDirectory=skillDirectory){
  adminState.value={session,data:overview,workOS:nextWorkOS,executorDirectory:nextExecutors,skillDirectory:nextSkills,executorDirectoryError:'',skillDirectoryError:'',loading:false,refreshing:false,error:'',clearError:()=>{},refresh:async()=>{},setTenantStatus:async()=>session.tenant,setTenantContentCapability:async()=>{throw new Error('not used')}};
}

const render=(node:React.ReactNode,location='/admin/dashboard')=>renderToStaticMarkup(<StaticRouter location={location}>{node}</StaticRouter>);

describe('operations control plane pages',()=>{
  beforeEach(()=>setAdminView());

  it('renders the operations overview from real server data',()=>{
    const markup=render(<AdminOperationsOverview/>);
    expect(markup).toContain('运营总览');
    expect(markup).toContain('IP 人设视频');
    expect(markup).toContain('12');
    expect(markup).not.toContain('运行基础设施');
  });

  it('shows an explicit empty state when no creative product exists',()=>{
    setAdminView({...workOS,sops:[]});
    const markup=render(<AdminProductsPage/>,'/admin/products');
    expect(markup).toContain('还没有创作产品');
    expect(markup).not.toContain('演示产品');
  });

  it('keeps creative products available when optional operations directories fail',()=>{
    setAdminView();
    adminState.value.executorDirectoryError='接口 /api/bff/operations/executors 返回了无效 JSON（HTTP 404）';
    adminState.value.skillDirectoryError='接口 /api/bff/operations/skills 返回了无效 JSON（HTTP 404）';
    const productMarkup=render(<AdminProductsPage/>,'/admin/products');
    expect(productMarkup).toContain('IP 人设视频');
    expect(productMarkup).not.toContain('创作产品暂不可用');
    const executorMarkup=render(<AdminExecutorsPage/>,'/admin/executors');
    expect(executorMarkup).toContain('执行端目录暂不可用');
    expect(executorMarkup).toContain('HTTP 404');
    const skillMarkup=render(<AdminSkillsPage/>,'/admin/skills');
    expect(skillMarkup).toContain('技能包目录暂不可用');
    expect(skillMarkup).toContain('重新加载');
  });

  it('renders registered capabilities and verified skill package entries',()=>{
    const capabilityMarkup=render(<AdminCapabilityCatalogPage/>,'/admin/capabilities');
    expect(capabilityMarkup).toContain('灵感采集');
    expect(capabilityMarkup).toContain('1 个能力已登记');
    expect(capabilityMarkup).toContain('href="/admin/capabilities/inspiration_collection/versions/1.0.0"');
    const skillMarkup=render(<AdminSkillsPage/>,'/admin/skills');
    expect(skillMarkup).toContain('1 个技能包版本已登记');
    expect(skillMarkup).toContain('contentcloud-script-writing');
    expect(skillMarkup).toContain('href="/admin/skills/contentcloud-script-writing/versions/1.2.0"');
    expect(skillMarkup).toContain('可进入候选');
  });

  it('keeps an unconfigured skill registry explicit and empty',()=>{
    setAdminView(workOS,executorDirectory,{configured:false,skills:[],generated_at:'2026-08-08T02:30:00Z'});
    const markup=render(<AdminSkillsPage/>,'/admin/skills');
    expect(markup).toContain('技能包 Registry 未配置');
    expect(markup).not.toContain('演示技能包');
  });

  it('opens a skill version with registry evidence and honest manifest gaps',()=>{
    const markup=render(<Routes><Route path="/admin/skills/:skillID/versions/:skillVersion" element={<AdminSkillDetailPage/>}/></Routes>,'/admin/skills/contentcloud-script-writing/versions/1.2.0');
    expect(markup).toContain('contentcloud-script-writing');
    expect(markup).toContain('plugin-release');
    expect(markup).toContain('contracts/content-item-3.0.schema.json');
    expect(markup).toContain('满足 Registry 的新任务使用门槛');
    expect(markup).toContain('没有能力引用、输入 Schema、负责人');
    expect(markup).not.toContain('已被客户使用');
  });

  it('opens a capability version with technical facts and reverse references',()=>{
    const markup=render(<Routes><Route path="/admin/capabilities/:capabilityID/versions/:capabilityVersion" element={<AdminCapabilityDetailPage/>}/></Routes>,'/admin/capabilities/inspiration_collection/versions/1.0.0');
    expect(markup).toContain('灵感采集');
    expect(markup).toContain('contentcloud.inspiration-query/1.0');
    expect(markup).toContain('contentcloud.inspiration-result/1.0');
    expect(markup).toContain('candidate-list');
    expect(markup).toContain('href="/admin/products/ip-video/versions/2"');
    expect(markup).toContain('href="/admin/customers/environment-2"');
    expect(markup).toContain('不代表技能包已经批准');
  });

  it('renders executors from the independent device projection',()=>{
    const markup=render(<AdminExecutorsPage/>,'/admin/executors');
    expect(markup).toContain('分镜工作站');
    expect(markup).toContain('0.20.0');
    expect(markup).toContain('在线');
    expect(markup).toContain('href="/admin/executors/executor-1"');
    expect(markup).not.toContain('果木食品创作端');
    expect(markup).not.toContain('打开旧配置入口');
  });

  it('opens executor details with heartbeat, capabilities and project scope',()=>{
    const markup=render(<Routes><Route path="/admin/executors/:executorID" element={<AdminExecutorDetailPage/>}/></Routes>,'/admin/executors/executor-1');
    expect(markup).toContain('storyboard.local');
    expect(markup).toContain('darwin / arm64');
    expect(markup).toContain('果木食品 / 品牌短片');
    expect(markup).toContain('href="/admin/capabilities/inspiration_collection/versions/1.0.0"');
    expect(markup).toContain('24 小时失败率');
  });

  it('opens a product version workspace with real coverage and enrollment data',()=>{
    const markup=render(<Routes><Route path="/admin/products/:productID/versions/:version" element={<AdminProductDetailPage/>}/></Routes>,'/admin/products/ip-video/versions/2');
    expect(markup).toContain('IP 人设视频');
    expect(markup).toContain('当前发布');
    expect(markup).toContain('1/1');
    expect(markup).toContain('创建新版本');
    expect(markup).toContain('版本治理');
    expect(markup).toContain('href="/admin/customers?product=ip-video"');
  });

  it('filters customer enrollments by product context and links to stable details',()=>{
    const markup=render(<AdminCustomersPage/>,'/admin/customers?product=ip-video');
    expect(markup).toContain('正在查看使用「IP 人设视频」的客户');
    expect(markup).toContain('果木食品创作端');
    expect(markup).toContain('href="/admin/customers/environment-1"');
    expect(markup).toContain('新建');
  });

  it('renders real enrollment coverage and an explicit future-task impact',()=>{
    const markup=render(<Routes><Route path="/admin/customers/:environmentID" element={<AdminCustomerDetailPage/>}/></Routes>,'/admin/customers/environment-1');
    expect(markup).toContain('客户开通配置');
    expect(markup).toContain('IP 人设视频 · v2');
    expect(markup).toContain('0/1');
    expect(markup).toContain('缺少 1 项产品所需能力');
    expect(markup).toContain('后续任务可能无法完成对应步骤');
    expect(markup).toContain('href="/admin/products/ip-video/versions/2"');
  });

  it('rejects an unknown customer enrollment deep link',()=>{
    const markup=render(<Routes><Route path="/admin/customers/:environmentID" element={<AdminCustomerDetailPage/>}/></Routes>,'/admin/customers/missing');
    expect(markup).toContain('没有找到这条客户开通记录');
    expect(markup).toContain('href="/admin/customers"');
  });

  it('links release records to the exact product version and rejects unknown versions',()=>{
    const releaseMarkup=render(<AdminProductReleasesPage/>,'/admin/releases');
    expect(releaseMarkup).toContain('href="/admin/releases/ip-video/versions/2"');
    const missingMarkup=render(<Routes><Route path="/admin/products/:productID/versions/:version" element={<AdminProductDetailPage/>}/></Routes>,'/admin/products/ip-video/versions/99');
    expect(missingMarkup).toContain('没有找到这个产品版本');
    expect(missingMarkup).toContain('href="/admin/products/ip-video"');
  });

  it('renders a server-grounded release result without invented canary facts',()=>{
    const markup=render(<Routes><Route path="/admin/releases/:productID/versions/:version" element={<AdminReleaseResultPage/>}/></Routes>,'/admin/releases/ip-video/versions/2');
    expect(markup).toContain('IP 人设视频 v2 已发布');
    expect(markup).toContain('version-digest');
    expect(markup).toContain('1 个客户开通当前固定使用该版本');
    expect(markup).toContain('进行中的任务继续使用任务创建时固定的版本');
    expect(markup).toContain('href="/admin/customers/environment-1"');
    expect(markup).not.toContain('Canary 已通过');
  });

  it('uses the independent Chinese operations shell without compatibility navigation',()=>{
    const markup=render(<AdminShell/>);
    expect(markup).toContain('平台运营后台');
    expect(markup).toContain('创作产品');
    expect(markup).toContain('能力目录');
    expect(markup).toContain('任务记录');
    expect(markup).toContain('任务用量');
    expect(markup).not.toContain('数据连接');
    expect(markup).not.toContain('创作结果');
    expect(markup).not.toContain('用户与角色');
    expect(markup).not.toContain('工作面正在建设');
    expect(markup).not.toContain('旧配置入口');
    expect(markup).not.toContain('兼容入口');
    expect(markup).not.toContain('运行基础设施');
  });
});
