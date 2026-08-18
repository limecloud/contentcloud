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

const assets = [
  [Users, '核心人物原型', '人物原型', '已确认'],
  [FileText, '高互动脚本 03', '剧本', '已确认'],
  [Boxes, '第 02 版镜头表', '分镜', '待确认'],
  [Images, '春日产品视觉', '图片', '已确认'],
  [Video, '春日品牌片候选', '视频', '已交付'],
] as const;

const capabilities = [
  {
    title: '项目文件夹与内容生产',
    status: '本地可用',
    tone: 'local',
    facts: '本地资料整理 · 来源记录 · 知识整理 · 创作简报',
    boundary: '电脑上的草稿仍需提交审核，不能直接作为最终内容。',
  },
  {
    title: '搜索与网页采集',
    status: '平台可用',
    tone: 'server',
    facts: '平台搜索 · 网页采集 · 来源记录 · 访问限制',
    boundary: '搜索范围、网站授权和使用额度由外部服务决定。',
  },
  {
    title: '自动处理与长任务恢复',
    status: '基础功能已准备',
    tone: 'runtime',
    facts: '开始 · 继续 · 暂停 · 恢复 · 状态记录',
    boundary: '具体使用哪些工具、账号和模型，要看当前客户的连接情况。',
  },
  {
    title: '平台审核与确认版本',
    status: '平台可用',
    tone: 'server',
    facts: '提交 · 审核 · 已确认版本 · 成果文件',
    boundary: '确认内容只代表审核通过，外部平台发布仍需单独完成。',
  },
  {
    title: '发布与交付结果',
    status: '按连接状态开放',
    tone: 'external',
    facts: '交付文件 · 发布记录 · 外部发布结果',
    boundary: '公众号目前需要人工发布；真实账号和视频服务由外部平台提供。',
  },
] as const;

function AssetCatalogVisual() {
  return <section className="marketing-evidence-panel marketing-catalog-panel" data-reveal>
    <header className="marketing-evidence-heading">
      <span>01 / 03</span>
      <h3>创作资料目录，不只是一个文件夹</h3>
      <p>人物、剧本、分镜、图片和视频结果按版本、确认状态与使用关系统一呈现。</p>
    </header>
    <div className="marketing-asset-browser" aria-label="创作资料目录界面示意">
      <aside>
        <div><BrandMark/><strong>Content Work OS</strong></div>
        <span><Sparkles size={14}/>创作</span>
        <span className="is-current"><Archive size={14}/>资料</span>
        <span><Workflow size={14}/>任务</span>
        <span><PackageCheck size={14}/>交付</span>
      </aside>
      <div className="marketing-asset-browser-main">
        <header><div><small>资料</small><strong>资料与创作结果</strong></div><button type="button" tabIndex={-1}><Search size={14}/>搜索资料</button></header>
        <nav aria-label="资料视图示意"><b>我的资料</b><span>创作结果</span><span>最近使用</span></nav>
        <div className="marketing-asset-tiles">
          {assets.map(([Icon,title,type,status])=><article key={title}>
            <div><Icon size={20}/><small>{type}</small></div>
            <strong>{title}</strong>
            <span><CheckCircle2 size={12}/>{status}</span>
          </article>)}
        </div>
        <footer><FolderOpen size={15}/><span><strong>加入本次创作</strong><small>固定版本、使用说明和范围</small></span><ArrowRight size={14}/></footer>
      </div>
    </div>
  </section>;
}

function AssetReuseVisual() {
  return <section className="marketing-evidence-panel marketing-reuse-panel" data-reveal data-reveal-order="2">
    <header className="marketing-evidence-heading">
      <span>02 / 03</span>
      <h3>确认结果，持续服务后续创作</h3>
      <p>团队资料和创作结果会分别整理；加入任务时会记住使用的版本和来源，不会悄悄改动原文。</p>
    </header>
    <div className="marketing-reuse-map" aria-label="创作资料复用关系示意">
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
  return <section className="marketing-evidence-panel marketing-architecture-panel" id="architecture" data-reveal>
    <header className="marketing-evidence-heading">
      <span>03 / 03</span>
      <h3>电脑、平台和外部服务各司其职</h3>
      <p>电脑负责需要本地处理的内容，平台负责提交、审核和保存确认结果，外部服务负责搜索、生成或发布；每一步的责任都清楚可查。</p>
    </header>
    <div className="marketing-architecture-map" aria-label="Content Work OS 平台分层架构示意">
      <div className="marketing-architecture-plane is-customer"><Laptop size={18}/><span><strong>你的工作电脑</strong><small>整理资料 · 补充灵感 · 继续创作</small></span></div>
      <div className="marketing-architecture-plane is-operations"><Cloud size={18}/><span><strong>平台记录与审核</strong><small>保存版本 · 检查内容 · 记录确认</small></span></div>
      <div className="marketing-architecture-down is-left"><ArrowDown size={17}/><small>你看到的任务进度</small></div>
      <div className="marketing-architecture-down is-right"><ArrowDown size={17}/><small>管理人员看到的状态</small></div>
      <div className="marketing-architecture-contract"><Boxes size={18}/><span><strong>资料和版本记录</strong><small>来源 · 知识 · 简报 · 内容 · 交付</small></span></div>
      <ArrowDown className="marketing-architecture-main-arrow" size={18}/>
      <div className="marketing-architecture-runtime"><Cog size={20}/><span><strong>平台自动处理</strong><small>开始 · 继续 · 恢复 · 记录结果</small></span><b>自动处理</b></div>
      <div className="marketing-executor-grid">
        <span><Cog size={16}/><b>固定步骤</b><small>检查 · 转换 · 生成 · 整理文件</small></span>
        <span><Laptop size={16}/><b>电脑上的创作工具</b><small>本地工具 · 远程工具</small></span>
        <span><Cloud size={16}/><b>外部服务</b><small>搜索 · 生成 · 发布状态</small></span>
      </div>
      <div className="marketing-fact-gate"><ShieldCheck size={16}/><span>候选结果</span><ArrowRight size={14}/><UserCheck size={16}/><strong>人工确认</strong><ArrowRight size={14}/><PackageCheck size={16}/><b>最终确认内容</b></div>
    </div>
  </section>;
}

function CapabilityStatus() {
  return <section className="marketing-capability-status" aria-labelledby="marketing-capability-status-title" data-reveal>
    <header>
      <div>
        <span>当前可用情况</span>
        <h3 id="marketing-capability-status-title">哪些功能现在能用，哪些还需要连接</h3>
      </div>
      <Link to="/docs">查看完整说明 <ArrowRight size={15}/></Link>
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
    <footer><ShieldCheck size={16}/><span>状态以平台当前记录和公开目录为准；已有基础功能不等于外部账号、额度或自动发布已经可用。</span></footer>
  </section>;
}

export function MarketingSystemShowcase() {
  return <section className="marketing-section marketing-system-showcase" id="system">
    <div className="marketing-shell">
      <header className="marketing-section-heading" data-reveal>
        <span>内容创作平台能力</span>
        <h2>把资料、步骤和确认放在一起，<br/>每一步都清楚可控</h2>
        <p>资料、参考内容、制作步骤和人工确认会一起协作；候选结果经过来源、质量和使用权限检查后，才会成为可以交付的内容。</p>
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
