import { describe, expect, it } from 'vitest';
import {
  artifactKindLabel,
  articleCalloutKindLabel,
  auditActionLabel,
  auditSubjectLabel,
  contentTypeLabel,
  deliveryDestinationLabel,
  evidenceLocatorLabel,
  gateModeLabel,
  knowledgeBlockReasonLabel,
  knowledgeLayerLabel,
  knowledgeNextActionLabel,
  knowledgeObjectTypeLabel,
  knowledgeObjectTypeValue,
  knowledgePackPurposeLabel,
  mediaModelLabel,
  mediaProviderLabel,
  mediaRequestIDLabel,
  outputRoleLabel,
  rendererLabel,
  roleLabel,
  shotRoleLabel,
  sourceTypeLabel,
  stageLabel,
  statusLabel,
  storyboardAssetRoleLabel,
  submissionTypeLabel,
  taskTypeLabel,
  workflowCheckLabel,
} from './uiLabels';

describe('用户界面中文标签', () => {
  it('覆盖内容生产主流程中的常用枚举', () => {
    expect(statusLabel('waiting_gate')).toBe('待确认');
    expect(statusLabel('script_only')).toBe('仅剧本');
    expect(statusLabel('online')).toBe('在线');
    expect(statusLabel('offline')).toBe('离线');
    expect(roleLabel('project_manager')).toBe('项目负责人');
    expect(contentTypeLabel('marketing_video')).toBe('营销视频');
    expect(submissionTypeLabel('content_batch')).toBe('内容批次');
    expect(knowledgeObjectTypeLabel('Claim')).toBe('营销主张');
    expect(knowledgeLayerLabel('compliance')).toBe('合规要求');
    expect(sourceTypeLabel('workspace_file')).toBe('项目文件');
    expect(evidenceLocatorLabel('sheet_cell')).toBe('表格单元格');
    expect(stageLabel('postproduction')).toBe('后期制作');
    expect(gateModeLabel('client_decision')).toBe('客户确认');
    expect(outputRoleLabel('selected_take')).toBe('选定的候选成片');
    expect(artifactKindLabel('final_render')).toBe('最终成片');
    expect(auditActionLabel('media.job_created')).toBe('创建视频生成任务');
    expect(auditActionLabel('device.attached')).toBe('关联本地设备');
    expect(auditSubjectLabel('submission_revision')).toBe('提交内容版本');
    expect(auditSubjectLabel('device')).toBe('本地设备');
    expect(knowledgeObjectTypeLabel('Scenario')).toBe('使用场景');
    expect(shotRoleLabel('product_intro')).toBe('产品引入');
    expect(storyboardAssetRoleLabel('review_sheet')).toBe('分镜审核总览');
    expect(articleCalloutKindLabel('conclusion')).toBe('小结');
    expect(workflowCheckLabel('rights.references')).toBe('权利引用完整');
    expect(deliveryDestinationLabel('workspace')).toBe('项目文件夹');
    expect(knowledgePackPurposeLabel('content_production')).toBe('内容生产');
    expect(taskTypeLabel('knowledge_extract')).toBe('品牌知识提取');
    expect(mediaProviderLabel('fake')).toBe('本地演示生成服务');
    expect(mediaModelLabel('fixture-video')).toBe('演示视频模型');
    expect(mediaRequestIDLabel('fake', 'fake-request-1234')).toBe('本地演示请求 1234');
    expect(mediaRequestIDLabel('other', 'request-1234')).toBe('request-1234');
    expect(mediaRequestIDLabel('fake', '')).toBe('未返回请求编号');
    expect(rendererLabel('passthrough')).toBe('原样封装');
  });

  it('将中文知识类型输入转换为服务端协议值', () => {
    expect(knowledgeObjectTypeValue('营销主张')).toBe('Claim');
    expect(knowledgeObjectTypeValue('品牌规则')).toBe('BrandRule');
    expect(knowledgeObjectTypeValue('Scenario')).toBe('Scenario');
  });

  it('把知识查询的阻断原因和下一步转换为中文', () => {
    expect(knowledgeBlockReasonLabel('EVIDENCE_REQUIRED')).toBe('缺少可核对的原文依据');
    expect(knowledgeBlockReasonLabel('STATUS_PENDING')).toBe('当前状态不符合要求：待处理');
    expect(knowledgeBlockReasonLabel('CONFLICT_OPEN:conflict:price')).toBe('仍有知识冲突未解决：conflict:price');
    expect(knowledgeNextActionLabel('REQUEST_SOURCE')).toBe('补充可信来源');
  });

  it('未识别枚举回退到普通用户能理解的中文', () => {
    expect(statusLabel('future_state')).toBe('当前状态');
    expect(roleLabel('future_role')).toBe('团队成员');
    expect(stageLabel('future_stage')).toBe('其他步骤');
    expect(evidenceLocatorLabel('future_locator')).toBe('原文位置');
  });
});
