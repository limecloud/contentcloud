---
name: contentcloud-seedance-export
description: 将 ContentCloud 服务端已批准且经本地 digest 校验的 StoryboardPackage 编译为确定性、可直接复制使用的 Seedance 上传清单和中文提示词包。适用于向 Seedance 导出分镜帧、映射 @图片/@视频/@音频 引用、拆分镜头提示词、校验供应商限制或诊断过期包；不得代替用户上传到 Seedance 或批准内容。
---

# ContentCloud Seedance 导出

将已锁定分镜投影为供应商专用操作指令。不得更改受众策略、脚本事实、产品断言、分镜媒体或批准状态。

## 执行边界

| 平面 | 允许工作 |
| --- | --- |
| `Codex local` | 读取已拉取的分镜 ApprovedSnapshot、校验本地 digest、编译稳定的上传编号与提示词、校验限制/权利/Offer，并写入 `60-delivery`。 |
| `ContentCloud server` | 提供权威分镜 ApprovedSnapshot，并可选择保存单独发布的交付清单；它不执行 Seedance 生成。 |
| `User in Seedance` | 登录、检查披露内容、按顺序上传文件、核对 UI 引用编号/设置、粘贴提示词、启动生成并下载结果。 |

不得将原始本地 `review_ready` 清单作为权威。必须使用已拉取且 `submission_type` 为 `storyboard` 的 ApprovedSnapshot，并要求快照对象的 `locked_digest` 与每个本地输入文件匹配。

## 工作流

1. 解析已绑定工作区并展示选中的分镜 ApprovedSnapshot。拒绝可变缓存文件、Project/Workspace 不匹配、合格对象 ID 缺失或非分镜快照。

2. 读取有效且经人工核验的 Seedance 供应商 Profile。将模型标签、支持模式、文件格式、引用数量、时长范围、大小限制、声音行为、人脸策略和过期时间视为版本化事实。不得从未固定版本的上游 `master` 分支复制限制。

3. 重新计算分镜清单和媒体 digest。出现任何漂移时以 `STORYBOARD_LOCKED_DIGEST_MISMATCH` 停止；不得针对已变更媒体静默重新编号。

4. 只选择模型输入：身份锚点、首帧/尾帧、已批准参考视频和已批准参考音频。除非已验证供应商 Profile 明确定义分镜板模式，否则排除 `review_sheet`。

5. 确定性分配引用：先处理公共锚点，再按片段和镜头顺序处理；对相同 Artifact ID 去重；图片、视频和音频分别独立编号为 `@图片N`、`@视频N` 和 `@音频N`。

6. 沿叙事边界编译一个或多个片段。每个片段只保留一个可观察动作或转场。保留片段间的传出/传入状态。片段超过有效供应商 Profile 时拒绝，不得机械截断。

7. 按以下顺序编写每条中文提示词：模式与设置、引用用途、传入状态、带时间的可观察动作、构图/镜头/运动、声音意图、传出状态、产品与连续性锁定，最后是负向约束。避免无依据的质量形容词和相互冲突的镜头指令。

8. 不在生成底片中包含价格、优惠券、库存、准确包装文字、字幕、标志、CTA、法律文字和倒计时。将它们放入 `post_production_plan`；使用动态条款时，最终渲染或抖音发布前必须存在仍有效的 CommerceOfferSnapshot。

9. 校验每个 `@引用` 准确映射一个上传项、每个复制文件匹配 SHA-256、所有限制与权利检查通过、不包含绝对路径或凭据，且供应商 Profile 未过期。

10. 使用从选中供应商 Profile 读取的限制运行本地导出器，不得使用猜测的默认值：

   ```bash
   contentcloud local seedance export \
     --snapshot <storyboard-approved-snapshot-id> \
     --storyboard <storyboard-package-id> \
     --profile-version <verified-profile-version> \
     --adapter-digest sha256:<adapter-digest> \
     --sound <profile-sound-setting> \
     --min-duration <profile-min> \
     --max-duration <profile-max> \
     --max-images <profile-image-limit> \
     --max-videos <profile-video-limit> \
     --max-audios <profile-audio-limit>
   contentcloud local seedance lint <generated-package.json>
   ```

11. 生成自包含目录：

   ```text
   60-delivery/packages/<package-id>/providers/seedance/
     package.json
     README.md
     prompts/segment-01.txt
     media/image-01.<ext>
   ```

12. 向用户展示上传顺序和提示词文件。除非用户另行执行外部平台动作，否则在打开、上传、生成、下载或发布前停止。

## 必需的操作员交接

确保 `README.md` 在没有聊天历史时也足够使用。包含已锁定分镜快照和 digest、Adapter/Profile 版本、平台设置、准确上传顺序与 `@引用` 映射、每个片段的复制文本、预期传入/传出状态、验收检查、重试范围和后期制作检查表。

生成后，将下载结果视为新的本地候选产物。人工选择、本地 QA、后期制作、最终交付发布、抖音发布和服务端创意绑定是权限各自独立的不同阶段。
