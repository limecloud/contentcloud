import { BookOpen, CheckCircle2, FileText, Quote } from 'lucide-react';
import type { BusinessTaskCanvasProps } from '../../workbench/contract/businessTaskTypes';

export function ArticleTaskCanvas({task,step,results}:BusinessTaskCanvasProps){
  return <section className="workbench-task-canvas workbench-article-canvas" aria-label="文章创作工作面">
    <header className="workbench-canvas-heading"><span><FileText size={15}/>文章创作</span><h3>{step.title}</h3><p>{step.outcome_description}</p></header>
    <div className="workbench-article-grid">
      <aside className="workbench-outline"><header><BookOpen size={15}/><strong>文章目录</strong></header><ol><li className="is-active"><span>01</span><div><strong>{task.title}</strong><small>{task.project.brand_name}</small></div></li><li><span>02</span><div><strong>资料与引用</strong><small>{task.asset_count} 项输入</small></div></li><li><span>03</span><div><strong>校对与确认</strong><small>等待当前版本</small></div></li></ol></aside>
      <article className="workbench-article-editor"><header><small>当前文章草稿</small><h4>{task.title}</h4><p>{task.intent||'文章主题与业务目标将在这里形成可审阅版本。'}</p></header><div className="workbench-editor-lines"><span/><span/><span className="is-short"/></div><blockquote><Quote size={15}/><span>当前阶段：{step.title}<small>{step.outcome_description}</small></span></blockquote></article>
      <aside className="workbench-proofing"><header><CheckCircle2 size={15}/><strong>引用与校对</strong></header><div><span>资料完整性</span><b>待检查</b></div><div><span>品牌表达</span><b>待确认</b></div><div><span>已生成版本</span><b>{results.length} 个</b></div></aside>
    </div>
  </section>;
}
