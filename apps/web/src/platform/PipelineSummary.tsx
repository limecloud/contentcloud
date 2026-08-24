import { BarChart3, CheckCircle2, GitBranch, PackageCheck, ShieldCheck } from 'lucide-react';
import './platform.css';

export interface PipelineSummaryValue {
  stageCount:number;
  completedStageCount:number;
  executionCount:number;
  pendingDecisionCount:number;
  approvedVersionCount:number;
  artifactCount:number;
  deliveryPackageCount:number;
  performanceObservationCount:number;
  learningDecisionCount:number;
}

interface PipelineSummaryProps {
  value:PipelineSummaryValue;
}

const cards=[
  {key:'flow',label:'流程与执行',icon:GitBranch,detail:(value:PipelineSummaryValue)=>`${value.completedStageCount}/${value.stageCount} 个阶段已完成 · ${value.executionCount} 次执行`},
  {key:'review',label:'确认版本',icon:ShieldCheck,detail:(value:PipelineSummaryValue)=>`${value.approvedVersionCount} 个批准版本 · ${value.pendingDecisionCount} 个待确认`},
  {key:'delivery',label:'产物与交付',icon:PackageCheck,detail:(value:PipelineSummaryValue)=>`${value.artifactCount} 个产物 · ${value.deliveryPackageCount} 个交付包`},
  {key:'learning',label:'效果与学习',icon:BarChart3,detail:(value:PipelineSummaryValue)=>`${value.performanceObservationCount} 条效果观察 · ${value.learningDecisionCount} 个学习决策`},
] as const;

export function PipelineSummary({value}:PipelineSummaryProps){
  const total=Math.max(value.stageCount,1);
  const progress=Math.min(100,Math.round(value.completedStageCount/total*100));
  return <section className="platform-pipeline-summary" aria-label="平台流程与产物闭环">
    <header className="platform-pipeline-heading"><div><span>平台闭环</span><h3>流程、产物与效果</h3></div><strong>{progress}%</strong></header>
    <div className="platform-pipeline-track" aria-hidden="true"><span style={{width:`${progress}%`}}/></div>
    <div className="platform-pipeline-grid">{cards.map(({key,label,icon:Icon,detail})=><div className={`platform-pipeline-card is-${key}`} key={key}><span className="platform-pipeline-card-icon"><Icon size={15}/></span><div><strong>{label}</strong><small>{detail(value)}</small></div>{key==='review'&&value.pendingDecisionCount>0&&<b>{value.pendingDecisionCount}</b>}{key==='delivery'&&value.deliveryPackageCount>0&&<CheckCircle2 size={14} className="platform-pipeline-check"/>}</div>)}</div>
  </section>;
}
