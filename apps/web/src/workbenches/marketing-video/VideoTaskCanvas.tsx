import { CheckCircle2, Clapperboard, FileText, Image, PackageCheck, Play, Users, Video } from 'lucide-react';
import type { BusinessTaskCanvasProps } from '../../workbench/contract/businessTaskTypes';

const stageLabels=['目标与资料','营销剧本','视频分镜','候选成片','确认成片','交付准备'];

export function VideoTaskCanvas({task,step,results}:Pick<BusinessTaskCanvasProps,'task'|'step'|'results'>){
  return <section className="workbench-task-canvas workbench-video-canvas" aria-label="视频生产工作面">
    <header className="workbench-canvas-heading"><span><Video size={15}/>视频生产</span><h3>{step.title}</h3><p>{step.outcome_description}</p></header>
    <div className="workbench-video-grid">
      <aside className="workbench-video-rail"><header><Clapperboard size={15}/><strong>生产阶段</strong></header><ol>{stageLabels.map((label,index)=><li key={label} className={label===step.title?'is-active':''}><span>{String(index+1).padStart(2,'0')}</span><div><strong>{label}</strong><small>{label===step.title?'当前工作':'按批准流程推进'}</small></div></li>)}</ol></aside>
      <section className="workbench-video-board"><header><small>当前视频任务</small><h4>{task.title}</h4><p>{task.intent||'从资料、剧本和已确认的镜头开始，形成可审阅的视频版本。'}</p></header><div className="workbench-shot-strip"><div className="workbench-shot-card is-active"><span><Image size={17}/></span><strong>镜头与素材</strong><small>{task.asset_count} 项输入</small></div><div className="workbench-shot-card"><span><FileText size={17}/></span><strong>剧本与旁白</strong><small>随当前版本固定</small></div><div className="workbench-shot-card"><span><Play size={17}/></span><strong>候选成片</strong><small>{results.length} 个结果</small></div></div><blockquote><CheckCircle2 size={15}/><span>本阶段产出：{step.outcome_description}<small>结果只有通过平台审核后才会进入交付。</small></span></blockquote></section>
      <aside className="workbench-video-context"><header><Users size={15}/><strong>任务上下文</strong></header><dl><div><dt>项目</dt><dd>{task.project.brand_name}</dd></div><div><dt>输入资料</dt><dd>{task.asset_count} 项</dd></div><div><dt>创作结果</dt><dd>{results.length} 个</dd></div></dl><footer><PackageCheck size={14}/><span>交付引用批准版本</span></footer></aside>
    </div>
  </section>;
}
