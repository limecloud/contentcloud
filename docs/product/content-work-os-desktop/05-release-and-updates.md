# Desktop 发布、签名与更新

状态：`GitHub Actions 矩阵发布与产物门禁已建立；正式签名、真实安装升级和自动更新服务仍未通过外部门禁`。

更新时间：2026-08-18。

## 1. 发布事实源

Electron Forge 是唯一打包工具，配置位于 `apps/desktop/forge.config.ts`。更新通道和目标平台的机器可读事实位于 [`apps/desktop/update-channels.json`](../../../apps/desktop/update-channels.json)，由 `pnpm desktop:release:check` 校验。

当前目标矩阵：

| 目标        | 打包格式 | 更新格式         | CI runner        |
| ----------- | -------- | ---------------- | ---------------- |
| macOS arm64 | DMG、ZIP | ZIP              | `macos-14`       |
| macOS x64   | DMG、ZIP | ZIP              | `macos-15-intel` |
| Windows x64 | Squirrel | Squirrel         | `windows-2022`   |
| Linux x64   | DEB、RPM | 暂不提供自动更新 | `ubuntu-22.04`   |

## 2. 更新通道

`stable` 和 `beta` 都要求签名产物、禁止降级，并分别从以下元数据路径读取：

```text
desktop/stable/latest.json
desktop/beta/latest.json
```

元数据必须绑定应用 ID、版本、目标平台、架构、产物摘要、下载地址、发布时间、更新通道和签名验证信息。客户端不能因为 HTTP 200、版本号更大或文件名匹配就安装更新；签名和摘要校验失败必须停在可诊断状态。

## 3. 安全与职责

- Electron Main 负责更新检查、下载、校验、重启提示和生命周期；Renderer 只显示状态。
- 更新服务不能收到 Workspace 正文、审批正文、设备 Token 或 Cloud 凭据。
- Daemon 的同步队列和 SQLite 传输状态必须在 Desktop 重启或升级后可恢复。
- `beta` 只用于内部验证；没有签名和安装升级记录时，不能把包发布到 `stable`。
- Linux 预览包不宣称支持正式自动更新。

## 4. 当前门禁

已经自动验证：Forge makers、应用 ID、目标平台声明、通道元数据约束、Linux CI package 和本地 package。

仍必须在持有发布凭据和真实机器的环境完成：

1. macOS arm64/x64 Developer ID 签名、notarization、Gatekeeper 安装。
2. Windows x64 Authenticode 签名、SmartScreen 安装和卸载。
3. stable/beta 元数据签名、更新下载、失败回滚和重启恢复。
4. Desktop 关闭后 Daemon 继续同步，升级后 outbox、cursor、分片状态不丢失。

这些项目不能由 CI 中的未签名预览包或模拟响应代替。

## 5. GitHub Actions 发布

桌面端发布 workflow 位于 `.github/workflows/desktop-release.yml`，借鉴 Lime 的发布分层，但只保留 ContentCloud 当前需要的链路：

```text
vX.Y.Z(-beta.N)
        |
        v
resolve (tag / version / channel / source ref)
        |
        +--> macOS arm64/x64   --+
        +--> Windows x64          +--> stage + SHA-256 + latest.json --> artifact
        +--> Linux x64           --+
                                      |
                                      v
                         aggregate --> existing/new GitHub Release --> publish
```

触发约定：

- 项目只有一套 `v*` 版本标签。推送 `v0.28.0` 后，桌面安装包与 CLI 资产进入同一个 `v0.28.0` Release。
- 推送 `v0.28.0-beta.1` 自动构建并发布 `beta` prerelease。
- `workflow_dispatch` 默认生成未签名的 Actions preview artifact，供内部安装验证；只有显式打开 `publish` 才会启用签名、创建或更新 GitHub Release。
- 手动运行可填写 `source_ref`；留空时构建对应 `v*` 标签。发布模式强制从不可变的同名标签构建，只有 `publish=false` 的预览 artifact 才允许使用其他 ref。`apps/desktop/package.json` 的版本必须与输入版本完全一致。
- 对已经存在的 GitHub Release，workflow 只追加或重建带桌面前缀的资产，不会改写 Release 标题、正文或 CLI 资产。
- 同一标签的 publish job 使用标签级并发锁串行上传，防止自动触发和手动回填交错覆盖资产。

每个平台 job 都执行 `electron-forge make`，随后由 `scripts/stage-desktop-release.mjs`：

1. 只接收 `update-channels.json` 声明的格式，并强制要求 DMG+ZIP、Squirrel installer+NUPKG+RELEASES、DEB+RPM 各自完整。
2. 生成带目标前缀的稳定文件名、目标级 `latest.json` 和 `*-checksums.sha256`。
3. 正式发布在 macOS 上执行 `codesign` 与 Gatekeeper assessment，在 Windows 上验证 Authenticode 状态；`publish=false` 时跳过签名，只生成不得进入更新通道的预览包。
4. 在 publish job 复验四个目标、版本和每个文件摘要，汇总为 `desktop-<channel>-latest.json` 和 `desktop-checksums.txt`，再上传 GitHub Release。桌面端使用独立校验文件名，避免覆盖 CLI 的 `checksums.txt`。

macOS 的 DMG maker 间接依赖 `macos-alias` 和 `fs-xattr` 原生模块。release job 使用 pnpm hoisted 布局，并由 pnpm 的 `onlyBuiltDependencies` 在安装阶段构建这些原生依赖，不读取 `.pnpm` 内部目录。Linux DEB/RPM maker 直接使用 `apps/desktop/package.json` 中的 `description`、`license` 和统一的可执行文件名，不在 workflow 中修补历史标签源码。

正式 Release 所需 GitHub Actions secrets：

| Secret                                     | 用途                              |
| ------------------------------------------ | --------------------------------- |
| `APPLE_CERTIFICATE`                        | base64 编码的 Developer ID `.p12` |
| `APPLE_CERTIFICATE_PASSWORD`               | `.p12` 密码                       |
| `APPLE_ID` / `APPLE_APP_SPECIFIC_PASSWORD` | notarization 账号                 |
| `APPLE_SIGNING_IDENTITY` / `APPLE_TEAM_ID` | macOS 签名身份与团队              |
| `KEYCHAIN_PASSWORD`                        | CI 临时 keychain                  |
| `WINDOWS_SIGNING_CERTIFICATE`              | base64 编码的 Authenticode `.pfx` |
| `WINDOWS_SIGNING_CERTIFICATE_PASSWORD`     | `.pfx` 密码                       |

`publish=true` 时缺少任一 macOS/Windows 签名 secret，构建 job 必须失败，不会上传可被误认为正式版本的未签名安装包。`publish=false` 的预览构建不读取签名 secret，产物元数据标记为 `unverified-preview`，不包含 Release 下载地址，不能聚合或发布到 stable/beta 更新通道；用户手动安装时也可能看到系统安全提示。GitHub token 只用于 Release 元数据和 asset 上传；不会传入 Electron Renderer，也不会接收 Workspace 正文、设备 token 或 Cloud 凭据。

## 6. 发布后的外部门禁与回滚

GitHub Release 成功只表示资产已生成并完成摘要记录，不表示客户端自动更新服务已经上线。发布前后仍需在真实设备完成：

1. macOS Gatekeeper 安装、首次启动、升级和回滚。
2. Windows Squirrel 安装、卸载、N-1 升级和 SmartScreen 检查。
3. `latest.json` 签名验证、下载中断恢复、摘要失败阻断和重启恢复。
4. Desktop 关闭后 Daemon 继续同步，升级后 outbox、cursor 与分片状态保持不变。

发现资产错误时，优先把 GitHub Release 保持为 draft 或撤下对应 channel 元数据，再重新构建更高版本；不复用同一版本号覆盖不同内容，也不通过降级更新修复客户端状态。
