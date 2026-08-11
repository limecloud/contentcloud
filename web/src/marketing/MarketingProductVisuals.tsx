import {
  Archive,
  ArrowDown,
  ArrowRight,
  Boxes,
  CheckCircle2,
  Cloud,
  Cog,
  FileText,
  FolderOpen,
  Images,
  Laptop,
  PackageCheck,
  Search,
  ShieldCheck,
  Sparkles,
  UserCheck,
  Users,
  Video,
  Workflow,
} from 'lucide-react';
import { Link } from 'react-router-dom';
import { BrandMark } from '../components/Brand';

const stageLabels = ['项目参考', '人物原型', '剧本与分镜', '媒体结果', '交付'] as const;

const assets = [
  [Users, '核心人物原型', '人物原型', '已确认'],
  [FileText, '高互动脚本 03', '剧本', '已确认'],
  [Boxes, '第 02 版镜头表', '分镜', '待确认'],
  [Images, '春日产品视觉', '图片', '已确认'],
  [Video, '春日品牌片候选', '视频', '已交付'],
] as const;

const capabilities = [
  {
    title: '本地工作区与内容生产',
    status: '本地可用',
    tone: 'local',
    facts: 'LocalRun · EvidenceBundle · KnowledgePack · Brief · ContentBatch',
    boundary: '本地候选仍需提交审核，不能直接成为批准事实。',
  },
  {
    title: '搜索与受控网页采集',
    status: '服务端可用',
    tone: 'server',
    facts: 'source.search / source.fetch · 白名单 · SSRF 与大小限制',
    boundary: '搜索 Provider、站点授权、robots 与配额由外部提供。',
  },
  {
    title: 'Agent Harness 与 Durable Runtime',
    status: '执行框架已接通',
    tone: 'runtime',
    facts: 'Detect · Start · Resume · Interrupt · Effect · Outbox · Reconcile',
    boundary: '具体 Agent 宿主、SaaS 账号、模型和进程能力按环境接通。',
  },
  {
    title: '服务端审核与批准快照',
    status: '服务端可用',
    tone: 'server',
    facts: 'Submission · Review · ApprovedSnapshot · Artifact · Projection',
    boundary: '批准是内容事实，外部渠道发布仍是独立动作。',
  },
  {
    title: '渠道交付与外部回执',
    status: '按连接状态开放',
    tone: 'external',
    facts: 'DeliveryPackage · ChannelPublication · Callback · Receipt',
    boundary: '公众号当前为人工发布；真实渠道账号与媒体服务属于外部依赖。',
  },
] as const;

export function HeroWorkCanvas() {
  return <div className="marketing-home-scene" role="img" aria-label="Content Work OS 客户创作任务，展示固定输入、人工确认与后续创作结果">
    <div className="marketing-hero-visual">
      <div className="marketing-hero-photo" aria-hidden="true"/>
      <div className="marketing-hero-window">
        <header>
          <span><BrandMark/><strong>春日线香品牌片</strong></span>
          <small>草稿已保存</small>
        </header>
        <div className="marketing-hero-window-body">
          <aside>
            <strong>创作任务</strong>
            {stageLabels.map((label,index)=><span className={index===1?'is-current':''} key={label}><i>{index<1?'✓':String(index+1).padStart(2,'0')}</i>{label}</span>)}
          </aside>
          <section>
            <span>内容策略 / 等待确认</span>
            <h3>确认这版核心受众</h3>
            <p>系统已固定 8 项资料与项目参考。确认后继续生成剧本方案。</p>
            <div className="marketing-hero-choice is-selected"><UserCheck size={16}/><span><strong>都市独居女性</strong><small>关注睡前放松与空间氛围</small></span><CheckCircle2 size={16}/></div>
            <div className="marketing-hero-choice"><Users size={16}/><span><strong>新中式生活爱好者</strong><small>关注器物、仪式感与文化表达</small></span></div>
            <footer><Archive size={15}/><span><strong>结果会进入创作资产</strong><small>固定版本，下一次任务可继续使用</small></span><ArrowRight size={15}/></footer>
          </section>
        </div>
      </div>
      <div className="marketing-hero-proof is-source"><Archive size={17}/><span><strong>8 项输入已固定</strong><small>来源与版本可追溯</small></span></div>
      <div className="marketing-hero-proof is-result"><PackageCheck size={17}/><span><strong>下一步明确</strong><small>确认后继续生成剧本</small></span></div>
    </div>
  </div>;
}

function AssetCatalogVisual() {
  return <section className="marketing-evidence-panel marketing-catalog-panel">
    <header className="marketing-evidence-heading">
      <span>01 / 03</span>
      <h3>创作资产目录，不是另一个文件夹</h3>
      <p>人物、剧本、分镜、图片和视频结果按版本、确认状态与使用关系统一呈现。</p>
    </header>
    <div className="marketing-asset-browser" aria-label="创作资产目录界面示意">
      <aside>
        <div><BrandMark/><strong>Content Work OS</strong></div>
        <span><Sparkles size={14}/>创作</span>
        <span className="is-current"><Archive size={14}/>资产</span>
        <span><Workflow size={14}/>任务</span>
        <span><PackageCheck size={14}/>交付</span>
      </aside>
      <div className="marketing-asset-browser-main">
        <header><div><small>资产</small><strong>资料与创作结果</strong></div><button type="button" tabIndex={-1}><Search size={14}/>搜索资产</button></header>
        <nav aria-label="资产视图示意"><b>我的资产</b><span>创作结果</span><span>最近使用</span></nav>
        <div className="marketing-asset-tiles">
          {assets.map(([Icon,title,type,status])=><article key={title}>
            <div><Icon size={20}/><small>{type}</small></div>
            <strong>{title}</strong>
            <span><CheckCircle2 size={12}/>{status}</span>
          </article>)}
        </div>
        <footer><FolderOpen size={15}/><span><strong>加入本次创作</strong><small>固定版本、摘要和使用范围</small></span><ArrowRight size={14}/></footer>
      </div>
    </div>
  </section>;
}

function AssetReuseVisual() {
  return <section className="marketing-evidence-panel marketing-reuse-panel">
    <header className="marketing-evidence-heading">
      <span>02 / 03</span>
      <h3>确认结果，持续服务后续创作</h3>
      <p>客户资料与生成结果使用独立投影；加入创作时固定版本、摘要和来源关系，不复制正文。</p>
    </header>
    <div className="marketing-reuse-map" aria-label="创作资产复用关系示意">
      <div className="marketing-reuse-side is-input">
        <span><Archive size={15}/><b>项目参考</b><small>任务输入</small></span>
        <span><Users size={15}/><b>人物原型</b><small>已确认结果</small></span>
        <span><FileText size={15}/><b>剧本方案</b><small>已确认结果</small></span>
      </div>
      <ArrowRight className="marketing-reuse-arrow" size={18}/>
      <div className="marketing-reuse-core"><BrandMark/><strong>创作任务</strong><small>固定引用与版本</small></div>
      <ArrowRight className="marketing-reuse-arrow" size={18}/>
      <div className="marketing-reuse-side is-output">
        <span><FileText size={15}/><b>分镜方案</b><small>等待确认</small></span>
        <span><Video size={15}/><b>图片与视频</b><small>继续制作</small></span>
        <span><PackageCheck size={15}/><b>交付结果</b><small>回到交付</small></span>
      </div>
    </div>
    <div className="marketing-reuse-loop"><Archive size={16}/><span>任务输入与项目参考</span><ArrowRight size={14}/><span>新创作任务</span><ArrowRight size={14}/><span>已确认结果</span><ArrowRight size={14}/><strong>下一次复用</strong></div>
  </section>;
}

function PlatformArchitectureVisual() {
  return <section className="marketing-evidence-panel marketing-architecture-panel" id="architecture">
    <header className="marketing-evidence-heading">
      <span>03 / 03</span>
      <h3>三种执行平面协作，但不混淆事实所有权</h3>
      <p>普通交互创作留在本地，服务端负责提交、审核与批准快照，Runtime 负责长时自动化、恢复和外部副作用对账。</p>
    </header>
    <div className="marketing-architecture-map" aria-label="Content Work OS 平台分层架构示意">
      <div className="marketing-architecture-plane is-customer"><Laptop size={18}/><span><strong>本地交互生产平面</strong><small>Plugin · Skill · MCP · LocalRun · Handoff</small></span></div>
      <div className="marketing-architecture-plane is-operations"><Cloud size={18}/><span><strong>服务端治理与审核平面</strong><small>Submission · Review · ApprovedSnapshot · Artifact</small></span></div>
      <div className="marketing-architecture-down is-left"><ArrowDown size={17}/><small>客户体验投影</small></div>
      <div className="marketing-architecture-down is-right"><ArrowDown size={17}/><small>运营投影</small></div>
      <div className="marketing-architecture-contract"><Boxes size={18}/><span><strong>版本化内容与治理契约</strong><small>Source · Knowledge · Brief · ContentBatch · Delivery</small></span></div>
      <ArrowDown className="marketing-architecture-main-arrow" size={18}/>
      <div className="marketing-architecture-runtime"><Cog size={20}/><span><strong>服务端自动化执行平面</strong><small>JobRun · NodeRun · RuntimeAttempt · Effect · Outbox</small></span><b>Durable Runtime</b></div>
      <div className="marketing-executor-grid">
        <span><Cog size={16}/><b>确定性 Worker</b><small>校验 · 转换 · 渲染 · 打包</small></span>
        <span><Laptop size={16}/><b>开放 Agent Harness</b><small>本地 Agent · 远程 Agent · Agent SaaS</small></span>
        <span><Cloud size={16}/><b>Provider 与 Channel</b><small>搜索 · 模型 · 媒体 · 发布回执</small></span>
      </div>
      <div className="marketing-fact-gate"><ShieldCheck size={16}/><span>候选结果</span><ArrowRight size={14}/><UserCheck size={16}/><strong>人工确认</strong><ArrowRight size={14}/><PackageCheck size={16}/><b>正式事实</b></div>
    </div>
  </section>;
}

function CapabilityStatus() {
  return <section className="marketing-capability-status" aria-labelledby="marketing-capability-status-title">
    <header>
      <div>
        <span>当前实现状态</span>
        <h3 id="marketing-capability-status-title">代码已经具备什么，外部还需要接通什么</h3>
      </div>
      <Link to="/docs">查看完整能力地图 <ArrowRight size={15}/></Link>
    </header>
    <div className="marketing-capability-table" role="list">
      {capabilities.map((capability,index)=><article key={capability.title} role="listitem">
        <span className="marketing-capability-index">{String(index+1).padStart(2,'0')}</span>
        <div className="marketing-capability-name">
          <strong>{capability.title}</strong>
          <b className={`is-${capability.tone}`}>{capability.status}</b>
        </div>
        <code>{capability.facts}</code>
        <p>{capability.boundary}</p>
      </article>)}
    </div>
    <footer><ShieldCheck size={16}/><span>状态依据 2026-08-11 Infra 文档与公开内容目录；“底层已实现”不等于外部账号、配额或自动发布已经可用。</span></footer>
  </section>;
}

export function MarketingSystemShowcase() {
  return <section className="marketing-section marketing-system-showcase" id="system">
    <div className="marketing-shell">
      <header className="marketing-section-heading">
        <span>内容创作 AI Infra</span>
        <h2>连接生产所需的一切，<br/>但不让任何执行者越过治理边界</h2>
        <p>资料、证据、知识、Agent、模型、Worker、人工和发布渠道通过版本化契约协作；候选结果经过权利、质量和人工门禁后才成为正式事实。</p>
      </header>
      <div className="marketing-evidence-grid">
        <AssetCatalogVisual/>
        <AssetReuseVisual/>
        <PlatformArchitectureVisual/>
      </div>
      <CapabilityStatus/>
    </div>
  </section>;
}
