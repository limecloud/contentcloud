import { BookOpen, CheckCircle2, FileCheck2, FileText, PackageCheck, ScrollText } from 'lucide-react';
import type { BusinessTaskCanvasProps } from '../../workbench/contract/businessTaskTypes';

const stageLabels=['世界观 Canon','卷章规划','章节候选','连续性校验','编辑与确认','章节交付'];

export function NovelTaskCanvas({task,step,results}:Pick<BusinessTaskCanvasProps,'task'|'step'|'results'>){
  return <section className="workbench-task-canvas workbench-novel-canvas" aria-label="连载小说工作面">
    <header className="workbench-canvas-heading"><span><ScrollText size={15}/>连载小说</span><h3>{step.title}</h3><p>{step.outcome_description}</p></header>
    <div className="workbench-novel-grid">
      <aside className="workbench-novel-rail"><header><BookOpen size={15}/><strong>章节流程</strong></header><ol>{stageLabels.map((label,index)=><li key={label} className={label===step.title?'is-active':''}><span>{String(index+1).padStart(2,'0')}</span><div><strong>{label}</strong><small>{label===step.title?'当前工作':'由统一流程推进'}</small></div></li>)}</ol></aside>
      <article className="workbench-novel-editor"><header><small>当前章节任务</small><h4>{task.title}</h4><p>{task.intent||'从固定 Canon、卷章规划和资料继续形成可审阅章节。'}</p></header><div className="workbench-novel-manuscript"><span/><span/><span/><span className="is-short"/></div><blockquote><CheckCircle2 size={15}/><span>本阶段产出：{step.outcome_description}<small>章节只有经过连续性检查、内审和客户批准后才会进入交付。</small></span></blockquote></article>
      <aside className="workbench-novel-context"><header><FileCheck2 size={15}/><strong>章节上下文</strong></header><dl><div><dt>项目</dt><dd>{task.project.brand_name}</dd></div><div><dt>输入资料</dt><dd>{task.asset_count} 项</dd></div><div><dt>已保存结果</dt><dd>{results.length} 个</dd></div></dl><footer><PackageCheck size={14}/><span>交付引用批准章节快照</span></footer><div className="workbench-novel-facts"><FileText size={14}/><span>角色、时间线和伏笔引用保持可追溯</span></div></aside>
    </div>
  </section>;
}
