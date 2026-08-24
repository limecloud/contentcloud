# Business Workbenches

每个业务工作台拥有独立的目录和信息架构：

```text
workbenches/
├── marketing-video/  导航、阶段、镜头/候选画布、视频上下文
├── article/          目录、编辑器、引用、校对和文章交付
├── commerce/         商品事实、内容变体、渠道预览和交付
└── serialized-novel/ Canon、卷章、章节编辑、连续性和章节交付
```

工作台 feature 只通过 `../workbench/` 宿主取得平台契约；不得直接读取 Runtime、Provider、数据库 DTO 或 Agent Plugin 安装状态。它可以使用共享 UI 原语，但不能把业务页面搬进 `platform/` 或 `shared/`。

## Feature 组成

每个业务目录按真实需求逐步形成自己的边界，推荐使用：

```text
workbenches/<business>/
├── definition.ts       业务导航、阶段、面板和入口文案
├── navigation.ts       业务导航和入口文案（规模较大时再拆分）
├── stages.ts            阶段、状态和主要动作映射（规模较大时再拆分）
├── *BriefFields.tsx     业务简报字段和输入布局
├── views/              业务主画布、编辑器或比较面
├── components/         业务对象组件
├── api.ts              业务 BFF 查询与命令
└── *.test.ts(x)        页面、契约和响应式验收
```

不要为了目录整齐预建完整模板；只有已实现并有测试的职责才创建文件。不同业务可以使用完全不同的页面结构、对象语言和信息密度，统一的是平台边界，不是视觉骨架。

外部 `workbench-plugins/<id>` 只提供 manifest 和静态预览，不能携带任意 React bundle。宿主在 Registry 返回已批准版本后，才把它映射到对应的业务 feature；未知插件进入安全回退页，不静默复用另一个业务页面。
