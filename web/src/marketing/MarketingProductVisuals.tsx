import {
  Archive,
  ArrowDown,
  ArrowRight,
  BookOpen,
  Boxes,
  CheckCircle2,
  Cloud,
  Cog,
  FileSearch,
  FileText,
  FolderOpen,
  Images,
  Laptop,
  Lightbulb,
  PackageCheck,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  UserCheck,
  Users,
  Video,
  Workflow,
} from 'lucide-react';
import { BrandMark } from '../components/Brand';

const stageLabels = ['资料准备', '内容策略', '脚本方案', '分镜素材', '审核交付'] as const;

const assets = [
  [Images, '产品视觉规范', '品牌资产', '已批准'],
  [Users, '核心人物原型', '人物资产', 'v3'],
  [FileText, '高互动脚本 03', '历史成果', '已批准'],
  [Video, '春日品牌片', '交付成片', '可复用'],
] as const;

export function HeroWorkCanvas() {
  return <div className="marketing-home-scene" role="img" aria-label="资料和已有资产进入一次创作任务，经过确认后形成可交付结果">
    <div className="marketing-scene-source is-brand"><Archive size={17}/><span><strong>品牌资产</strong><small>产品 · 规范 · 人物</small></span></div>
    <div className="marketing-scene-source is-research"><FileSearch size={17}/><span><strong>可信资料</strong><small>搜索 · 知识 · 本地文件</small></span></div>
    <div className="marketing-scene-source is-idea"><Lightbulb size={17}/><span><strong>灵感观察</strong><small>主题 · 场景 · 业务判断</small></span></div>

    <span className="marketing-scene-link is-link-left"/>
    <span className="marketing-scene-link is-link-right"/>

    <div className="marketing-scene-task">
      <BrandMark/>
      <span><strong>一次创作任务</strong><small>流水线 · 版本 · 人工确认</small></span>
    </div>

    <div className="marketing-scene-output is-content"><FileText size={17}/><span><strong>内容方案</strong><small>策略 · 人物 · 剧本</small></span></div>
    <div className="marketing-scene-output is-media"><Video size={17}/><span><strong>制作素材</strong><small>分镜 · 图片 · 视频</small></span></div>
    <div className="marketing-scene-output is-delivery"><PackageCheck size={17}/><span><strong>交付成果</strong><small>批准版本 · 交付包</small></span></div>

    <div className="marketing-scene-stages">
      {stageLabels.map((label,index)=><span className={index<2?'is-complete':index===2?'is-current':''} key={label}><i>{String(index+1).padStart(2,'0')}</i><b>{label}</b></span>)}
    </div>
  </div>;
}

function AssetCatalogVisual() {
  return <section className="marketing-evidence-panel marketing-catalog-panel">
    <header className="marketing-evidence-heading">
      <span>01 / 03</span>
      <h3>创作资产目录，不是另一个文件夹</h3>
      <p>品牌、人物、灵感、脚本和已批准成果按来源、版本、权利与使用状态统一呈现。</p>
    </header>
    <div className="marketing-asset-browser" aria-label="创作资产目录界面示意">
      <aside>
        <div><BrandMark/><strong>Content Work OS</strong></div>
        <span><Sparkles size={14}/>创作</span>
        <span className="is-current"><Archive size={14}/>资产</span>
        <span><Workflow size={14}/>任务</span>
        <span><BookOpen size={14}/>知识</span>
      </aside>
      <div className="marketing-asset-browser-main">
        <header><div><small>资产库</small><strong>团队创作资产</strong></div><button type="button" tabIndex={-1}><Search size={14}/>搜索资产</button></header>
        <nav aria-label="资产类型示意"><b>全部</b><span>品牌</span><span>人物</span><span>脚本</span><span>媒体</span></nav>
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
      <h3>一次批准，持续服务后续创作</h3>
      <p>资产库同时是创作起点和结果归档点，复用的是固定版本和来源关系，不是复制粘贴正文。</p>
    </header>
    <div className="marketing-reuse-map" aria-label="创作资产复用关系示意">
      <div className="marketing-reuse-side is-input">
        <span><Archive size={15}/><b>品牌规范</b><small>来源资产</small></span>
        <span><Users size={15}/><b>人物原型</b><small>批准版本</small></span>
        <span><Lightbulb size={15}/><b>历史灵感</b><small>人工观察</small></span>
      </div>
      <ArrowRight className="marketing-reuse-arrow" size={18}/>
      <div className="marketing-reuse-core"><BrandMark/><strong>创作任务</strong><small>固定引用与版本</small></div>
      <ArrowRight className="marketing-reuse-arrow" size={18}/>
      <div className="marketing-reuse-side is-output">
        <span><FileText size={15}/><b>剧本方案</b><small>等待确认</small></span>
        <span><Video size={15}/><b>分镜素材</b><small>继续制作</small></span>
        <span><PackageCheck size={15}/><b>批准成果</b><small>回到资产库</small></span>
      </div>
    </div>
    <div className="marketing-reuse-loop"><Archive size={16}/><span>已有资产</span><ArrowRight size={14}/><span>新创作任务</span><ArrowRight size={14}/><span>批准结果</span><ArrowRight size={14}/><strong>下一次复用</strong></div>
  </section>;
}

function PlatformArchitectureVisual() {
  return <section className="marketing-evidence-panel marketing-architecture-panel" id="architecture">
    <header className="marketing-evidence-heading">
      <span>03 / 03</span>
      <h3>客户页面保持简单，复杂能力留在平台底层</h3>
      <p>客户创作台与运营控制台完全分离，Runtime 通过版本化契约组织不同执行者，候选结果经过人工门禁才成为正式事实。</p>
    </header>
    <div className="marketing-architecture-map" aria-label="Content Work OS 平台分层架构示意">
      <div className="marketing-architecture-plane is-customer"><Users size={18}/><span><strong>客户创作台与资产库</strong><small>输入 · 进度 · 预览 · 确认 · 交付</small></span></div>
      <div className="marketing-architecture-plane is-operations"><SlidersHorizontal size={18}/><span><strong>平台运营控制台</strong><small>流水线 · 能力 · 租户 · 诊断 · 回退</small></span></div>
      <div className="marketing-architecture-down is-left"><ArrowDown size={17}/><small>客户体验投影</small></div>
      <div className="marketing-architecture-down is-right"><ArrowDown size={17}/><small>运营投影</small></div>
      <div className="marketing-architecture-contract"><Boxes size={18}/><span><strong>业务域与版本化契约</strong><small>WorkTask · AssetRef · Gate · Artifact</small></span></div>
      <ArrowDown className="marketing-architecture-main-arrow" size={18}/>
      <div className="marketing-architecture-runtime"><Cog size={20}/><span><strong>ContentCloud Agentic Job Runtime</strong><small>调度 · 状态 · 恢复 · 费用 · 审计</small></span><b>内部基座</b></div>
      <div className="marketing-executor-grid">
        <span><Cog size={16}/><b>确定性 Worker</b><small>校验 · 转换 · 渲染</small></span>
        <span><Laptop size={16}/><b>本地智能客户端</b><small>Codex · Claude Code · MCP</small></span>
        <span><Cloud size={16}/><b>外部服务与工具</b><small>搜索 · 图片 · 视频 · API</small></span>
      </div>
      <div className="marketing-fact-gate"><ShieldCheck size={16}/><span>候选结果</span><ArrowRight size={14}/><UserCheck size={16}/><strong>人工确认</strong><ArrowRight size={14}/><PackageCheck size={16}/><b>正式事实</b></div>
    </div>
  </section>;
}

export function MarketingSystemShowcase() {
  return <section className="marketing-section marketing-system-showcase" id="system">
    <div className="marketing-shell">
      <header className="marketing-section-heading">
        <span>产品系统全景</span>
        <h2>资产、任务与执行能力<br/>在同一个系统持续积累</h2>
        <p>首页不再只讲抽象能力。下面三张产品图直接对应文档中的资产目录、客户创作闭环和平台分层架构。</p>
      </header>
      <div className="marketing-evidence-grid">
        <AssetCatalogVisual/>
        <AssetReuseVisual/>
        <PlatformArchitectureVisual/>
      </div>
    </div>
  </section>;
}
