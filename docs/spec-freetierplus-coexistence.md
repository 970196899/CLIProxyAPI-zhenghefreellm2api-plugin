# 设计规格说明书 (Spec)：FreeTier Plus 全量双开共存改造

- **创建日期**：2026-11-16
- **项目路径**：`D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main`
- **目标版本**：基于 `main` 最新提交（`e810ec3` 之后）
- **状态**：已定稿，待执行窗口实施

---

## 一、背景与目标

当前仓库与上游原版 `cxy2730/CLIProxyAPI-freetier2api-plugin` 共用完全相同的插件 ID（`freetier`）、Provider 键（`freetier`）、动态库文件名（`freetier.dll`）、管理路由（`/plugins/freetier/...`）及本地状态目录（`.freetier-plugin`）。将两者同时放入 CPA 的 `plugins/` 目录时，宿主会并发加载两份插件并产生注册表与状态文件冲突，无法直接双开共存。

本 Spec 的目标是将当前分支改造为可与原版 `freetier` 在同一 CPA 实例中**全量双开共存**的独立插件：
1. **插件身份全面独立**：采用 `freetierplus` 作为插件 ID 与 Provider 键，显示名称为 `FreeTier Plus`，打包动态库为 `freetierplus.dll` / `freetierplus.so`。
2. **保留全部 10 个供应商与原模型名**：不删减供应商，也不给对外模型 ID（如 `workbuddycn-*`）加额外前缀，确保现有客户端配置与重写规则无缝复用。
3. **运行状态物理隔离**：状态目录、环境变量、日志文件名、前端 `localStorage` 键、Qoder 嵌入身份缓存目录全部独立，避免双开互相覆盖。

---

## 二、核心身份映射表

| 维度 | 原版 (`freetier`) | 本分支新版 (`freetierplus`) | 所在关键位置 |
| --- | --- | --- | --- |
| Plugin ID | `freetier` | `freetierplus` | `internal/app/main.go:20`、`registry.json:5` |
| 显示名称 | `FreeTier 2API` | `FreeTier Plus` | `internal/app/main.go:21`、`registry.json:6`、`internal/admin/page/{00-head,10-body,30-accounts}.html` |
| ProviderKey | `freetier` | `freetierplus` | `internal/core/vendor.go:31` |
| 仓库地址 | `cxy2730/CLIProxyAPI-freetier2api-plugin` | `970196899/CLIProxyAPI-zhenghefreellm2api-plugin` | `internal/app/main.go:23`、`registry.json:9-10` |
| 管理路由前缀 | `/plugins/freetier` | `/plugins/freetierplus` | 由 `pluginID` 自动派生（`50-models.html:323` 需同步改用 `BASE`） |
| 宿主登录路由 | `/freetier-auth-url` | `/freetierplus-auth-url` | `internal/admin/page/70-login.html:324` |
| 默认状态目录 | `~/.cli-proxy-api/freetier` / `~/.freetier-plugin` | `~/.cli-proxy-api/freetierplus` / `~/.freetierplus-plugin` | `internal/admin/config.go:56,106` |
| 状态目录环境变量 | `FREETIER2API_PLUGIN_HOME` | `FREETIERPLUS_PLUGIN_HOME` | `internal/admin/config.go:58` |
| 日志文件前缀 | `freetier-YYYY-MM-DD.log` | `freetierplus-YYYY-MM-DD.log` | `internal/logger/logger.go:106,125,144,208` |
| Qoder 嵌入缓存目录 | `freetier` | `freetierplus` | `internal/vendors/qoder/credential.go:462` |
| 前端列偏好键 | `freetier-model-optional-columns` | `freetierplus-model-optional-columns` | `internal/admin/page/50-models.html:38` |
| 构建产出名 | `freetier.<ext>` / `freetier_<ver>_*.zip` | `freetierplus.<ext>` / `freetierplus_<ver>_*.zip` | `.github/workflows/ci.yml`、`scripts/build.sh` |

---

## 三、详细改造清单 (Technical Specification)

### 1. 插件身份与组合根 (`internal/app/main.go`, `internal/core/vendor.go`)
- `internal/app/main.go:20-23`：
  - `pluginID = "freetierplus"`
  - `pluginDisplayName = "FreeTier Plus"`
  - `pluginRepository = "https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin"`
  - `pluginAuthor` 可保持或标注维护者信息，不影响冲突隔离。
- `internal/core/vendor.go:31`：
  - `const ProviderKey = "freetierplus"`

### 2. 消除各供应商内硬编码的 `"freetier"` 字符串
以下供应商代码当前硬编码了 `"freetier"`，必须统一改为引用 `core.ProviderKey`，确保以后单点收口：
1. `internal/vendors/codearts/login.go:69,180`：`Provider: core.ProviderKey`
2. `internal/vendors/codearts/credential.go:103`：`"type": core.ProviderKey`
3. `internal/vendors/zcode/credential.go:194,234`：`"type": core.ProviderKey` / `m["type"] = core.ProviderKey`
4. `internal/vendors/trae/credential.go:125`：`"type": core.ProviderKey`
5. `internal/vendors/trae/login.go:497,520`：`"type": core.ProviderKey` / `Provider: core.ProviderKey`（以及第 80 行 `Provider: core.ProviderKey`）
6. `internal/vendors/cline/credential.go:187`：`if trimmed != "" && trimmed != core.ProviderKey && trimmed != "cline"`（防止误把新版凭证当作外部未知 provider 吞掉或拒绝）

### 3. 运行期状态、日志与缓存物理隔离
1. `internal/admin/config.go`：
   - `DefaultStateDirName = ".freetierplus-plugin"`
   - `StateDirEnvOverride = "FREETIERPLUS_PLUGIN_HOME"`
   - `DefaultStateDir()` 中 `cpaDir` 子目录改为 `filepath.Join(cpaDir, "freetierplus")`
   - 更新文件内相关注释（`plugins.configs.freetierplus`）。
2. `internal/logger/logger.go`：
   - 日志文件前缀改为 `"freetierplus-"`（125 行、144 行），stderr 错误前缀改为 `[freetierplus]`（106 行、208 行）。
3. `internal/vendors/qoder/credential.go:462`：
   - `embeddedIdentityDir = "freetierplus"`

### 4. 管理界面与静态 HTML 片段 (`internal/admin/`)
1. `internal/admin/admin.go:120`：资源页描述改为 `"FreeTier Plus 账号、额度与模型管理页。"`
2. `internal/admin/page/00-head.html:6`：`<title>FreeTier Plus</title>`
3. `internal/admin/page/10-body.html:3`：`<h1>FreeTier Plus</h1>`
4. `internal/admin/page/30-accounts.html:214`：引导文案中的 `FreeTier 2API` 改为 `FreeTier Plus`。
5. `internal/admin/page/50-models.html`：
   - 第 38 行：`var MODEL_COLUMNS_STORE_KEY = "freetierplus-model-optional-columns";`
   - 第 323 行：`var configPath = hostPath(BASE + "/config");`（消除 `/plugins/freetier/config` 硬编码，直接随 `BASE` 动态切换！）
6. `internal/admin/page/70-login.html:324`：
   - 将 `hostPath("/freetier-auth-url?vendor=" + ...)` 改为 `hostPath("/freetierplus-auth-url?vendor=" + ...)`（并同步更新 321 行及 `20-core.html:10` 注释中的示例路径）。

### 5. CI/CD、构建脚本与清单文档
1. `.github/workflows/ci.yml`：
   - 第 109、123、124、135、174 行及相关产物/说明中的 `freetier` 全部改为 `freetierplus`：
     - 编译输出 `dist/freetierplus-${{ matrix.goos }}-${{ matrix.goarch }}.${{ matrix.ext }}`
     - `packstore -id freetierplus`
     - Release 校验归档 `assets/freetierplus_${version}_${goos}.zip`
     - Release 正文说明与 artifact 名称同步改为 `freetierplus`
2. `scripts/build.sh`：输出路径改为 `dist/freetierplus.$ext`；`scripts/godocker.sh` 中卷名改为 `freetierplus-gocache`（避免与原版构建混用 cache）。
3. `registry.json`：`"id": "freetierplus"`、`"name": "FreeTier Plus"`、更新 `repository` / `homepage` 为当前仓库地址。
4. `README.md`、`README_EN.md`：同步更新控制台访问路径 `/v0/resource/plugins/freetierplus/console`、配置节 `plugins.configs.freetierplus`、默认状态目录 `~/.freetierplus-plugin` 及安装产物文件名说明。

### 6. 严禁误改的保留项（反例防护）
以下出现 `freetier` / `FreeTier` 的位置**属于上游协议特征或 Go 内部编译路径，严禁修改**：
- `go.mod` 与所有 `.go` 文件的 `import "freetier2api-plugin/..."`（Go 模块内部包路径，改之无收益且会波及 200+ 文件）；
- `internal/vendors/autoclaw/chat.go:176`：`req.Header.Set("x_trace_id", "freetier")`（上游请求指纹头）；
- `internal/vendors/opencodezen/` 中所有的 `isFreeTierModel`、`prepareFreeTierBody`、`FreeTierError`、`free tier can only be used`（OpenCode Zen 免费模型判定与上游报错关键字）；
- `cpasdk/` 下宿主 ABI 契约；
- 任何供应商的业务逻辑、配额算法、模型探测逻辑与签到流程。

---

## 四、底层假定与非目标 (Assumptions & Non-Goals)

1. **底层假定**：
   - CPA 宿主允许同时加载 `freetier` 与 `freetierplus` 两个插件，并分别依据凭证 JSON 中的 `"type": "freetier"` 与 `"type": "freetierplus"` 分发给对应插件。
   - 保持原模型名不变，用户在双开环境下通过将同一供应商的账号错开挂载到不同插件（或接受 CPA 同名模型合并调度）来使用。
2. **明确非目标 (Out of Scope)**：
   - 不开发自动扫描或改写现有 `auth-files/*.json` 文件的迁移脚本/UI 按钮；
   - 不修改 `go.mod` 模块路径；
   - 不改动任何线上 CPA 配置或远程文件。

---

## 五、验收标准 (Acceptance Criteria)

1. **测试断言同步与全绿**：
   - 更新受插件 ID / Provider 键 / 状态目录 / 路由前缀 / 页面标题变更影响的单元测试与集成测试（包括 `internal/app/auth_identifier_test.go`、`internal/admin/*_test.go`、`internal/logger/hostsink_test.go`、`internal/vendors/*_test.go`、`tests/integration/*_test.go` 等）。
   - 新增/强化防残留校验：验证 `app.PluginID == "freetierplus"`、`core.ProviderKey == "freetierplus"`、管理路由以 `/plugins/freetierplus` 开头、凭证序列化 `"type"` 为 `"freetierplus"`、控制台 HTML 不含 `/plugins/freetier/` 或 `/freetier-auth-url`。
2. **CI 六步全绿与构建产物验证**：
   - GitHub Actions `gofmt`、`go vet`、`go test ./...`、`-race`、`live` 编译、`go build -buildmode=c-shared` 全部通过（Exit Code 0）。
   - 如有触发 `workflow_dispatch` 验证构建 job，确认产物名为 `freetierplus-windows-amd64.dll` 等（且 `packstore` 单元测试若覆盖 `-id` 逻辑同样通过）。
