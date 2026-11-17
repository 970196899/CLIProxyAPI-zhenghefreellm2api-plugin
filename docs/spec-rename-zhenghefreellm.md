# 设计规格说明书 (Spec)：独立插件身份与商店更新源改造（`zhenghefreellm`）

- **创建日期**：2026-11-17
- **项目路径**：`D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main`
- **基线提交**：`main`（`25c0e4021c39124bfc5aa062c9aa5b85d0a79525`）
- **目标分支**：`feat/rename-zhenghefreellm` → 验收通过后合并 `main` 并打 `v0.1.63` 发版
- **状态**：已获用户确认并授权派发

---

## 一、背景与目标

当前仓库已同步上游 `v0.1.60~v0.1.62` 并保留 PR #1 修复，但对外插件 ID（`freetier`）、显示名称（`FreeTier 2API`）、打包动态库/zip 名及 `registry.json` / 元数据中的 GitHub 仓库地址仍沿用 `cxy2730/CLIProxyAPI-freetier2api-plugin`。

本次改造的目标是在**单装替换场景（不与原版同开共存）**下，将对外身份、打包产物与 CPA 插件商店更新源彻底与 `cxy2730` 区分开，同时保持现有用户凭证与本地状态**零迁移无缝沿用**：

1. **对外插件 ID 与打包产物独立**：`PluginID` 改为 `zhenghefreellm`，构建动态库为 `zhenghefreellm.<so|dll|dylib>`，商店归档为 `zhenghefreellm_<version>_<goos>_<goarch>.zip`。
2. **显示名称与作者独立**：显示名改为 `Zhenghe FreeLLM 2API`，作者改为 `970196899`。
3. **CPA 商店更新源独立**：`registry.json` 与插件注册元数据中的 `repository` / `homepage` 统一指向 `https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin`。
4. **内部凭证标识与运行状态目录保持兼容**：保留 `core.ProviderKey = "freetier"`、默认状态目录（`.freetier-plugin`）与 `/freetier-auth-url`，替换安装后原有 10 家账号凭证、设备指纹（`machineSalt`）及控制台设置无需重新登录或迁移。

---

## 二、核心身份映射与保留边界

| 维度 | 改造前 | 改造后 | 所在关键位置 |
| --- | --- | --- | --- |
| `PluginID` | `freetier` | `zhenghefreellm` | `internal/app/main.go:20`、`registry.json:5` |
| 显示名称 | `FreeTier 2API` | `Zhenghe FreeLLM 2API` | `internal/app/main.go:21`、`registry.json:6`、`internal/admin/page/{00-head,10-body,30-accounts}.html` |
| 作者 | `cxy2730` | `970196899` | `internal/app/main.go:22`、`registry.json:8` |
| 仓库 / 更新地址 | `https://github.com/cxy2730/CLIProxyAPI-freetier2api-plugin` | `https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin` | `internal/app/main.go:23`、`registry.json:9-10` |
| 管理路由前缀 | `/plugins/freetier` | `/plugins/zhenghefreellm` | 由 `pluginID` 自动派生；`50-models.html:323` 改用 `BASE + "/config"` |
| CPA 配置段 | `plugins.configs.freetier` | `plugins.configs.zhenghefreellm` | 宿主按 `PluginID` 匹配（`README.md` / `README_EN.md` 同步） |
| 打包动态库与归档 | `freetier.<ext>` / `freetier_<ver>_*.zip` | `zhenghefreellm.<ext>` / `zhenghefreellm_<ver>_*.zip` | `.github/workflows/ci.yml`、`scripts/build.sh` |
| `ProviderKey`（凭证 `type`） | `freetier` | **保留 `freetier`（禁止修改）** | `internal/core/vendor.go:31` |
| 宿主登录路由 | `/freetier-auth-url` | **保留 `/freetier-auth-url`（禁止修改）** | `internal/admin/page/70-login.html:324` |
| 状态目录 / 日志前缀 / 列偏好键 | `.freetier-plugin` / `freetier-` / `freetier-model-optional-columns` | **保留不变（禁止修改）** | `internal/admin/config.go`、`internal/logger/logger.go`、`50-models.html:38` |

---

## 三、详细改造清单

### 1. 插件元数据与商店清单
- `registry.json`：
  - `"id": "zhenghefreellm"`
  - `"name": "Zhenghe FreeLLM 2API"`
  - `"author": "970196899"`
  - `"repository": "https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin"`
  - `"homepage": "https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin"`
- `internal/app/main.go:20-23`：
  - `pluginID = "zhenghefreellm"`
  - `pluginDisplayName = "Zhenghe FreeLLM 2API"`
  - `pluginAuthor = "970196899"`
  - `pluginRepository = "https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin"`

### 2. 管理端显示与动态配置路由
- `internal/admin/admin.go:120`：资源页描述改为 `"Zhenghe FreeLLM 2API 账号、额度与模型管理页。"`
- `internal/admin/page/00-head.html:6`：`<title>Zhenghe FreeLLM 2API</title>`
- `internal/admin/page/10-body.html:3`：`<h1>Zhenghe FreeLLM 2API</h1>`
- `internal/admin/page/30-accounts.html:214`：空账号提示文案中的 `FreeTier 2API` 改为 `Zhenghe FreeLLM 2API`
- `internal/admin/page/50-models.html:323`：将硬编码的 `hostPath("/plugins/freetier/config")` 改为 `hostPath(BASE + "/config")`，使模型变更重载随 `pluginID` 自动派生（不新增任何按钮或控件）。

### 3. 构建、CI 与 Release 模板
- `.github/workflows/ci.yml`：
  - `build` job 输出路径改为 `dist/zhenghefreellm-${{ matrix.goos }}-${{ matrix.goarch }}.${{ matrix.ext }}`；
  - `packstore` 传入 `-id zhenghefreellm`；
  - `upload-artifact` name 与 path 改为 `zhenghefreellm-${{ matrix.goos }}-${{ matrix.goarch }}` 及 `dist/zhenghefreellm_*_${{ matrix.goos }}_${{ matrix.goarch }}.zip`；
  - `release` job 必选产物校验改为 `assets/zhenghefreellm_${version}_${goos}.zip`；
  - Release 正文模板更新为 `zhenghefreellm_<版本>_<goos>_<goarch>.zip`、`zhenghefreellm{ext}`，说明可通过本仓库 `registry.json` 三方插件源或手动解压安装。
- `scripts/build.sh`：产物输出路径改为 `dist/zhenghefreellm.$ext`。

### 4. 文档与配套测试适配
- `README.md`、`README_EN.md`：更新仓库地址（`970196899/CLIProxyAPI-zhenghefreellm2api-plugin`）、插件名（`Zhenghe FreeLLM 2API`）、动态库文件名（`zhenghefreellm.so` / `zhenghefreellm.dll` / `zhenghefreellm.dylib`）、`config.yaml` 配置段（`plugins.configs.zhenghefreellm`）、控制台 URL（`/v0/resource/plugins/zhenghefreellm/console`）以及配置三方商店源（`registry.json`）说明。
- 配套测试更新：
  - `tests/integration/registry_test.go:93`：`app.PluginID != "zhenghefreellm"`；
  - `internal/admin/admin_test.go`、`console_page_test.go`、`console_quota_test.go`、`console_r2_test.go`、`console_test.go`、`login_test.go`、`routes_cover_test.go` 及 `tests/integration/{activity_probe_test,management_resilience_test,testutil_test}.go` 中锁定 `/plugins/freetier` 与 `FreeTier 2API` 的断言同步更新为 `/plugins/zhenghefreellm` 与 `Zhenghe FreeLLM 2API`（其中 `console_test.go:569` 按新路由或 `BASE` 校验 `hostPath("/plugins/zhenghefreellm/config")` 或对应源码；`console_test.go:328,345` 的 `/freetier-auth-url` 保持不变）。

---

## 四、明确非目标与禁止误改清单

- **禁止修改 `core.ProviderKey`（保持 `"freetier"`）**，禁止改动任何供应商凭证解析/生成逻辑与 `"type"` 字段。
- **禁止修改**默认状态目录（`.freetier-plugin`）、环境变量名（`FREETIER2API_PLUGIN_HOME`）、日志文件名（`freetier-`）、Qoder 嵌入缓存目录、前端列偏好键。
- **禁止修改** `go.mod` 模块路径、供应商业务逻辑、模型 ID 前缀、`ReportMPEvents`。
- **禁止改动** `192.168.1.200` 上的 CPA 或 `~/.claude/settings.json`。

---

## 五、验收标准

1. **PR 阶段 CI 六步全绿（Exit Code 0）**：`gofmt`、`go vet`、`go test ./...`、`-race` 子集、编译 live 探针（含 `./internal/vendors/...`）、`c-shared` 构建全部通过；`TestRegistryManifestIsValid` 与 `TestPluginIDMatchesLibraryName` 通过。
2. **代码与边界核查**：`git diff` 确认 `core.ProviderKey == "freetier"` 未改、`/freetier-auth-url` 未改、`50-models.html` 已改用 `BASE + "/config"`。
3. **合并与发版 `v0.1.63`**：PR 合入 `main` 后打注解 tag `v0.1.63` 并推送，`build`（`linux/amd64`、`linux/arm64`、`windows/amd64`）与 `release` 全绿。
4. **Release 资产核验**：`v0.1.63` Release 含 `zhenghefreellm_0.1.63_linux_amd64.zip`、`zhenghefreellm_0.1.63_linux_arm64.zip`、`zhenghefreellm_0.1.63_windows_amd64.zip` 与 `checksums.txt`，三平台压缩包内根目录动态库分别为 `zhenghefreellm.so` / `zhenghefreellm.so` / `zhenghefreellm.dll` 且 SHA256 与 `checksums.txt` 一致。
