import { CheckCircle2, Image, ShoppingBag, Tags } from 'lucide-react';
import type { BusinessTaskCanvasProps } from '../../workbench/contract/businessTaskTypes';

export function CommerceTaskCanvas({task,step,results}:BusinessTaskCanvasProps){
  return <section className="workbench-task-canvas workbench-commerce-canvas" aria-label="电商内容工作面">
    <header className="workbench-canvas-heading"><span><ShoppingBag size={15}/>电商内容</span><h3>{step.title}</h3><p>{step.outcome_description}</p></header>
    <div className="workbench-commerce-grid">
      <aside className="workbench-product-facts"><header><ShoppingBag size={15}/><strong>商品事实</strong></header><dl><div><dt>商品</dt><dd>{task.project.brand_name}</dd></div><div><dt>任务</dt><dd>{task.title}</dd></div><div><dt>输入</dt><dd>{task.asset_count} 项资料</dd></div></dl></aside>
      <section className="workbench-variant-matrix"><header><Tags size={15}/><strong>卖点与内容变体</strong></header><div className="workbench-variant-row is-active"><span>核心卖点</span><strong>{task.intent||'待整理商品卖点'}</strong><b>当前</b></div><div className="workbench-variant-row"><span>短视频口播</span><strong>等待内容版本</strong><b>待生成</b></div><div className="workbench-variant-row"><span>图文渠道</span><strong>等待渠道变体</strong><b>待生成</b></div></section>
      <aside className="workbench-channel-preview"><header><Image size={15}/><strong>渠道预览</strong></header><div className="workbench-preview-frame"><Image size={24}/><span>预览将在变体确认后生成</span></div><footer><CheckCircle2 size={14}/><span>{results.length} 个结果版本</span></footer></aside>
    </div>
  </section>;
}
