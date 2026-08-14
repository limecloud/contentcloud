---
name: contentcloud-douyin-audience-strategy
description: 在已绑定的 ContentCloud 工作区中生成、比较、校验和发布受证据约束的抖音商业受众策略候选。适用于单受众策略、2 至 3 个受众对比、八类受众探索、受众到 Brief 交接或修订 AudienceStrategyVersion；始终分离 Codex 本地生成与 ContentCloud 服务端批准。
---

# ContentCloud 抖音受众策略

将服务端治理的受众分类体系和已批准项目证据转化为本地策略候选。不得把本地文件或模型建议视为已批准策略。

## 执行边界

严格使用以下执行平面：

| 平面 | 允许工作 |
| --- | --- |
| `Codex local` | 读取已拉取快照、搭建候选、比较受众、编辑本地 JSON、执行 lint，并准备发布预检。 |
| `ContentCloud server` | 保存分类治理事实、创建不可变 SubmissionRevision、执行审核、创建 ApprovedSnapshot，并记录审计历史。 |
| `Human` | 选择受众、核验证据、确认发布，并在服务端批准或要求修改。 |

不得在本地创建 `approved` 对象。`publish` 创建的是可审核修订版本，不是批准。只有 `contentcloud pull approved` 返回的快照具有权威性。

## 工作流

1. 检查已绑定工作区和当前已批准输入。仅当用户明确要求刷新时，拉取当前策略快照：

   ```bash
   contentcloud pull approved --type strategy
   ```

2. 要求已拉取不可变缓存中存在未过期且经人工核验的 `AudienceTaxonomySnapshot`。不得根据模型常识推断或静默更新八类受众分类体系。

3. 选择一种模式：

   - `single`：必须且只能提供一个受众代码。
   - `compare`：必须提供两个或三个受众代码和一个共同目标。
   - `explore`：只创建八张轻量策略卡；不得生成八套脚本、分镜、图片或视频。

4. 搭建本地候选：

   ```bash
   contentcloud local audience strategy scaffold \
     --taxonomy <taxonomy-object-id> \
     --mode <single|compare|explore> \
     --audience <code> \
     --objective <objective>
   ```

5. 为每个候选填写需求时刻、受证据约束的洞察、钩子假设、证明顺序、异议、CTA 策略、证据引用、实验类型、主变量、控制变量、目标指标和约束。

6. 分离证据与假设。仅由模型提出的断言保持 `candidate` 状态并设置低置信度。不得从受众标签推断敏感属性、收入、家庭结构、健康状况或购买力。

7. 校验每个选中候选：

   ```bash
   contentcloud local audience strategy lint <strategy.json>
   ```

8. 运行 `contentcloud publish strategy --file <strategy.json> --dry-run`。展示准确预检，并在云端写入前等待用户明确确认其 `plan_id`。发布会从 `Codex local` 跨越到 `ContentCloud server`。

9. 发布后停止，除非用户明确要求执行服务端审核动作且具备相应权限。不得代替用户批准。

10. 人工批准后，运行 `contentcloud pull approved --type strategy`。生成 Brief 或 ContentBatch 时，只使用已拉取的 ApprovedSnapshot。

## 实验规则

- 仅当受众是唯一主变量，且创意、Offer、预算逻辑、时间、落地页和观察窗口均受控时，使用 `strict_ab`。
- 有意将受众与表达配对时，使用 `audience_expression_fit_test`。
- 广泛探索时使用 `exploration_batch`。不得将其报告为受众因果测试。
- 创建 Brief 前，要求选中策略明确决策指标和测量窗口。

## 停止条件

当分类体系缺失或过期、证据引用缺失、Offer 无效、策略包含无依据的产品断言、实验类型与变更变量冲突，或工作区尚未拉取所需 ApprovedSnapshot 时，以结构化阻断信息停止。

报告失败门禁、涉及的本地文件和下一个有效命令。不得通过编辑已拉取缓存文件来修复正式事实。
