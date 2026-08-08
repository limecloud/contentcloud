# ContentCloud 产品需求索引

状态：`目标产品需求索引；不代表相关能力已经上线`。

更新时间：2026-08-07。

本目录保存建立在 [ContentCloud 平台基线](../foundation/README.md) 之上的场景产品需求。平台基线定义产品平面、事实所有权、Runtime、代码边界、契约、安全和迁移规范；本目录只定义具体客户如何使用这些能力，不建立第二套领域模型或执行状态。

## 当前产品需求

| 文档或目录 | 定位 | 状态 |
| --- | --- | --- |
| [00-product-narrative.md](./00-product-narrative.md) | 客户叙事图、平台架构图、工具示例和分层表达规范 | 目标产品叙事 |
| [customer-creation-studio](./customer-creation-studio/README.md) | 简单客户创作台、运营流水线产品层和首个灵感采集纵向切片 | 客户首切片已实现，完整分层持续迁移 |
| [creative-asset-library](./creative-asset-library/README.md) | 客户工作区资料、创作结果、复用引用与运营治理 | “我的资产”上传首切片与客户结果目录已实现，连接器导入和结果持久化 Projector 待完成 |
| [operations-control-plane](./operations-control-plane/README.md) | 平台运营后台、中文页面蓝图、创作产品发布、能力与执行方式、绑定规则、运行诊断和创作结果治理 | 目标设计；按 O0-O7 阶段建设 |

## 文档规则

1. 新场景需求必须先引用 `docs/foundation`，再描述客户目标、输入、结果、决定和交付。
2. 客户文档不得直接定义 JobRun、NodeRun、Lease、Effect 或执行者状态机。
3. 需要新增平台原语时，先用两条真实业务流证明现有契约不足，再通过 ADR 修改基线。
4. 目标、Preview、已实现和已投产必须明确区分；当前能力以代码、迁移、测试和 `docs/content` 为准。
5. 客户资产入口必须组合工作区资料与创作结果两个专用投影；跨任务引用固定现有事实对象版本，不复制正文或创建第二套处理、批准、权利和交付状态。
6. 官网、销售、客户文档和工程架构使用同一事实但不同信息密度；不得用“万能 Agent”或供应商 Logo 替代产品与能力边界。
