import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { PipelineSummary } from './PipelineSummary';

describe('platform pipeline summary',()=>{
  it('renders the shared orchestration and closure facts for any workbench',()=>{
    const markup=renderToStaticMarkup(<PipelineSummary value={{stageCount:6,completedStageCount:3,executionCount:4,pendingDecisionCount:1,approvedVersionCount:2,artifactCount:5,deliveryPackageCount:1,performanceObservationCount:3,learningDecisionCount:1}}/>);
    expect(markup).toContain('流程、产物与效果');
    expect(markup).toContain('3/6 个阶段已完成');
    expect(markup).toContain('5 个产物 · 1 个交付包');
    expect(markup).toContain('3 条效果观察 · 1 个学习决策');
  });
});
