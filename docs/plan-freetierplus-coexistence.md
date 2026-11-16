# FreeTier Plus 全量双开共存 施工方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把插件身份从 `freetier` 全面改为 `freetierplus`（Plugin ID / ProviderKey / 状态目录 / 日志名 / 管理路由 / 构建产物 / 显示名），使本插件能与原版 `freetier` 在同一个 CPA 实例里双开共存，同时保留全部 10 个供应商与全部原模型名。

**Architecture:** 改造是「身份常量单点收口 + 全仓去残留」。`internal/core/vendor.go` 的 `ProviderKey` 与 `internal/app/main.go` 的 `pluginID` 是两个唯一真理源；其余位置一律改为引用它们，不再写字面量。管理路由前缀已由 `pluginID` 派生（`managementRoutePrefix = "/plugins/" + pluginID`），因此改 `pluginID` 即自动迁移路由；控制台页面里唯一写死的 `/plugins/freetier/config` 改为随 `BASE` 走。状态目录 / 日志文件名 / 前端 localStorage 键 / Qoder 嵌入缓存目录是纯字面量，逐处替换。

**Tech Stack:** Go 1.24（CI）/ 模块 `freetier2api-plugin`（**不改**）；标准库 only；`testing` 表驱动；`embed` + 字符串拼接的控制台页面；GitHub Actions `ci.yml`（workflow id `419900436`）是唯一的红绿证据来源（本机无 Go 工具链、无 Docker）。

**Spec:** `D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main\docs\spec-freetierplus-coexistence.md`

---

## Global Constraints

- **本机没有任何 Go 工具链**：无 `go.exe`、无 Docker、WSL `AlmaLinux-8` 停止。**所有**红绿证据只能来自 GitHub Actions。任何 `go test` / `gofmt` / `go vet` 命令都必须理解为「推送到分支后在 CI 上执行」。
- `ci.yml` 的 test 作业依次执行：`gofmt -l .`（排除 `cpasdk/`）→ `go vet ./...` → `go test ./... -count=1 -timeout 300s` → `go test -race ./internal/core ./internal/state ./internal/auth ./internal/tasks ./internal/admin -count=1` → `go test -tags live ./tests/live -run '^$' -count=1` → `go build -buildmode=c-shared -o /dev/null .`。六步全绿才算绿灯。
- **严禁**改动 `go.mod` 的 `module freetier2api-plugin`，也**严禁**改动全仓任何 `"freetier2api-plugin/..."` 形式的 import 路径（涉及 200+ 文件，且与插件身份无关）。
- **严禁**改动 `cpasdk/` 任何文件（宿主 ABI vendored 副本）。
- **严禁**改动 `internal/vendors/autoclaw/chat.go:176` 的 `req.Header.Set("x_trace_id", "freetier")`（上游请求指纹头）。
- **严禁**改动 `internal/vendors/opencodezen/` 里所有 `FreeTier` / `freeTier` / `isFreeTierModel` / `prepareFreeTierBody` / `FreeTierError` / `free tier can only be used` 语义（OpenCode Zen 免费模型判定与上游报错关键字）。
- **严禁**改动任何供应商的业务逻辑、配额算法、模型探测、签到流程；`internal/vendors/*/` 的改动**只允许**把 `"freetier"` 字面量替换为 `core.ProviderKey`。
- 只允许改动本方案列出的文件。允许新建 **1 个**文件：`tests/integration/identity_residue_test.go`。
- 代码注释一律简体中文，与该仓现有风格一致（现有注释把「为什么」写清楚，不复述「做了什么」）。
- **gofmt 只能由 CI 判定**：缩进用 tab、行尾不留空白、对齐沿用周边列宽。凡修改 Go 文件必须在提交前自查缩进（本方案给出的代码片段已按 gofmt 习惯排版）。
- Spec 钉死的字面量必须逐字照抄：`freetierplus`、`FreeTier Plus`、`.freetierplus-plugin`、`FREETIERPLUS_PLUGIN_HOME`、`freetierplus-`、`freetierplus-model-optional-columns`、`/freetierplus-auth-url`、`https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin`。
- **严禁**为了让新用例变绿而删改既有断言、放宽阈值或替换为假数据。凡既有断言写死了 `"freetier"`，只能**按语义**改成新值或改为引用常量；不得删除断言。

## 全局风险裁定（Spec 未覆盖 / 需在方案期钉死）

### 风险 A：凭证认领的第二层判据（文件名前缀）在双开下的重复认领

**代码事实**（`internal/hostbridge/auth.go:193-225`）：

```go
belongs := (provider != "" && strings.EqualFold(provider, core.ProviderKey)) || fileNameBelongsToPlugin(entry.Name, registry)
```

第二层 `fileNameBelongsToPlugin`（同文件 `:296-327`）按 `registry.Vendors()` 的 vendor ID 前缀匹配文件名（`workbuddycn-*.json` 等），**不含任何 `freetier` 字样**。原始需求方判断「改 ProviderKey 不会破坏它」——**这一判断正确**。但由此产生一个 Spec 未讨论的后果：

> 双开后，同一个 `workbuddycn-<uid>.json` 文件对**两个插件**都满足第二层判据。对原版 `freetier`，它的 `provider` 字段若为 `freetier`，第一层就命中；对本插件 `freetierplus`，第一层不命中（provider 是 `freetier`），但第二层命中。

**裁定：不新增 provider 隔离，保持现状，理由有三条，均为代码可证：**

1. **这是既有、被刻意设计的容错行为，不是本次改造引入的新风险。** 同文件 `:90-103` 与 `:185-192` 的注释明确说明：第二层存在的目的是「兜住被前置插件抢走的凭证」——宿主遍历插件解析凭证时会用当前询问插件的 identifier 兜底填 `provider`，导致本插件合法账号的 `provider` 变成别家。删掉或收紧第二层会让这些账号从页面凭空消失。既有守门测试 `internal/hostbridge/auth_test.go:18-47`（`TestListHostAuthsClaimsHijackedCredentials`）直接钉死该行为。
2. **重复「列出」不等于重复「认领」。** `filterPluginAuths` 只影响**本插件自己的管理页/额度/签到看到的账号清单**（调用点：`internal/admin/accounts.go:300,363`、`internal/auth/pool.go:112`、`internal/app/host.go:22`）。凭证**归属**由宿主的 `auth.parse` 流程决定：本插件 `handleAuthParse`（`internal/app/authprovider.go:59-92`）用 `registry.Resolve` 做三级判定（vendor 字段 → 文件名 → 内容嗅探），对同一份 `type=freetier` 的 WorkBuddy 凭证，两侧都会 `Handled=true`。真正决定「这份凭证归谁」的是宿主自己的插件遍历顺序与 `type` 字段匹配，**不是本插件的 `filterPluginAuths`**。
3. **物理上无法从插件侧区分。** 插件的 `host.auth.list` 回调只能拿到 `core.HostAuth`（`internal/core/host.go:24-37`），字段止于 `Provider`/`Name`/`Path`，没有任何字段能表达「这份凭证是否已被另一个插件认领」。在插件侧新增过滤只会把用户合法账号误判成别人的。

**必须配套的验证**：Task 7 新增 `TestCoexistenceAuthListKeepsCurrentClaimBehavior`，用假的 `host.auth.list` 同时喂入 `provider=freetier`（原版）与 `provider=freetierplus`（新版）与 `provider=qoder`（被抢）三类条目，断言 `filterPluginAuths` 对三者的处置与改造前**逐条一致**（即第二层仍然收下 `workbuddy*` 文件名，不因 ProviderKey 改名而改变）。这条测试的存在意义是「锁定它不是本次改造的回归源」，而不是「证明双开无副作用」。

**同时须写进 README 的双开须知（Task 8）**：两个插件都会在自己的控制台里列出同一批 vendor 前缀凭证。用户应把不同账号的凭证文件分开放置，或依赖宿主 `type` 字段分发；插件侧不做二次隔离。这是已知行为，不是缺陷。

### 风险 B：`machine_salt` 随状态目录迁移 → WorkBuddy 设备指纹变化

**代码事实**：
- `internal/app/config.go:95-97` `saltFilePath(cfg) = filepath.Join(cfg.StateDir, admin.SaltFileName)`，`SaltFileName = "machine_salt"`（`internal/admin/config.go:62`）。
- `internal/app/config.go:115-135` `ensureMachineSalt`：`cfg.Raw["machine_salt"]` 有覆盖值时直接用它；否则读文件；**文件不存在或内容为空时生成 24 字节随机盐并写盘**。
- 盐的消费点：`internal/app/host.go:34` 注入 `core.Settings.MachineSalt` → `internal/vendors/workbuddy/adapter.go:578-579` → `rt.deriveIDWithSalt`（`internal/vendors/workbuddy/chat.go:240-244`）→ `X-Machine-ID` 请求头（`internal/vendors/workbuddy/chat.go:618`）与 `machineId`/`sessionId`（`internal/vendors/workbuddy/credential.go:598-599`）。
- `internal/vendors/workbuddy/adapter_test.go:93-95` 明确断言：「changing the install salt must change fingerprints」。

**改造后果**：状态目录从 `~/.freetier-plugin` 改为 `~/.freetierplus-plugin`（`DefaultStateDirName`）后，本插件首次启动会在新目录下生成**全新的随机盐**，WorkBuddy 的 `X-Machine-ID` 随之改变。上游看到的是「同一账号、不同设备」。

**裁定：不迁移、不复制盐文件（符合 Spec 非目标「不开发自动扫描或改写现有 auth-files 的迁移脚本」，盐文件同理），但必须做两件事：**

1. **代码侧不留任何隐式兼容**——不改 `ensureMachineSalt` 的语义。随机盐的生成是既有正确行为（部署隔离），本次身份改名正是一个**新部署**，新盐是语义正确的结果。
2. **文档侧明示影响与解法**（并入 Task 8 的 README 双开须知）：受影响的只有 WorkBuddy 家族（CN/Global）。用户若要保留原指纹，有两种**用户可自行执行**的方式，均无需改代码：
   - 在新的 `config.yaml` 里给 `plugins.configs.freetierplus.machine_salt` 填上原值（`internal/app/config.go:116` 已支持该覆盖键，优先级高于文件）；
   - 或先把旧状态目录里的 `machine_salt` 文件复制到新状态目录（文件名固定为 `machine_salt`，见 `admin.SaltFileName`）。

**必须配套的验证**：Task 6 的 `TestDefaultStateDirFallback` 更新后仅断言目录名；不改盐逻辑，因此不新增盐相关测试（现有 `adapter_test.go:93-95` 已覆盖「盐变则指纹变」，其行为不受本次改造影响）。

### 风险 C：`cline/credential.go:187` 的 `type` 判定语义

**代码事实**（`internal/vendors/cline/credential.go:162-192`）：`hasForeignMarker(raw)` 在 `raw["type"]` 是非空字符串且**既不等于 `"freetier"` 也不等于 `"cline"`** 时返回 `true`（即「带别家特征，不得认领 Cline」）。改造后第一个比较对象变为 `core.ProviderKey`（`freetierplus`）。于是：

| 凭证 `type` | 改造前 | 改造后 | 语义 |
| --- | --- | --- | --- |
| `freetier` | 不算 foreign | **算 foreign** → 拒绝认领 | 原版插件的凭证，本插件不该认领 ✅ |
| `freetierplus` | — | 不算 foreign | 本插件自己写的凭证 ✅ |
| `cline` | 不算 foreign | 不算 foreign | Cline 自身 ✅ |
| `workbuddy` 等 | foreign | foreign | 别家 ✅ |

**裁定：改造后的行为正确，且比现状更好。** 理由：Cline 的 `type` 判据是「排除法」——只有明确不是任何已知 `type` 的字面量才判 foreign。原版 `freetier` 的 type 在改造后被本插件视作別家，正是双开想要的方向（原版凭证归原版）。反方向（原版插件看到本插件写的 `type=freetierplus`）也会判 foreign 并拒绝认领——两侧对称，不会互相误吞。

**语义辨析（必须写进代码注释与 Task 3 的测试名）**：这不是「Cline 认领 freetier 凭证」，而是「Cline 的凭证判定不再把 freetier 凭证当作自己人」。`type=cline` 分支保持不变（Cline 自己写的凭证没有 type 字段，见 `internal/vendors/cline/login.go:255-273`；该分支只为兼容手工导入的 `type=cline` 文件）。

**必须配套的验证**：Task 3 在 `internal/vendors/cline/credential_test.go` 新增两条用例：`{"legacy freetier type is foreign", "acc.json", {"type":"freetier","refreshToken":"rt"}, false}` 与 `{"provider type is ours", "acc.json", {"type":"freetierplus","refreshToken":"rt"}, true}`，直接钉死上表两条关键行。

### 额外的 Spec 冲突（已核实，需在 Task 1 内一并处理）

- **Spec §3.4 第 6 条说 `70-login.html:324` 改路由，并要求「同步更新 321 行及 `20-core.html:10` 注释」。** 实测：`70-login.html:321` 是注释里的示例路径，`20-core.html:10` 也是注释。两者**必须改**（否则防残留测试会命中），但它们的语义是「说明宿主如何解析 provider」，改的是示例字面量，不改变逻辑。
- **Spec §3.5 说 `scripts/godocker.sh` 卷名改 `freetierplus-gocache`。** 已核实 `scripts/godocker.sh:28`。注意：这只是本地开发缓存卷名，与 CI 无关；改它是为了不与原版混用编译缓存。
- **Spec §3.2 未列出 `internal/vendors/workbuddy/credential.go:427,438` 的注释与 `internal/vendors/codearts/*`、`internal/vendors/trae/*` 的 `AuthData.Provider` 字段。** 已核实：`workbuddy/credential.go:439` 用的是 `ProviderKey`（已是常量引用，无需改），仅注释里的 `freetier` 需改。而 `codearts/login.go:69,180`、`trae/login.go:80,497,520`、`zcode/credential.go:194,234`、`trae/credential.go:125`、`codearts/credential.go:103` 是**字面量**，必须改。
- **Spec §5 第 1 条要求「新增/强化防残留校验：验证 `app.PluginID == "freetierplus"`、`core.ProviderKey == "freetierplus"`…」** —— 本方案用 Task 1 的 `tests/integration/identity_residue_test.go` 承载，并在 Task 8 补齐 HTML 侧断言。
- **Spec 漏列的 `internal/logger/hostsink.go`**：它的 `:140`（stderr 前缀 `[freetier]`）与 `:207`（宿主日志来源段 `[freetier.go:0] [freetier]`）也是身份字面量。Spec §3.3.2 只点了 `logger.go`，但日志文件名与宿主日志来源段必须同时改，否则双开时两条日志流在宿主 `main.log` 里无法区分，且防残留守卫会红。已并入 Task 5。
- **Spec 漏列的 `internal/vendors/workbuddy/credential_test.go:137`**：现有用例 `{"type field uppercase", map[string]any{"type": "FreeTier"}, ...}` 依赖 `strings.EqualFold("FreeTier", ProviderKey)`。`ProviderKey` 改为 `freetierplus` 后该用例**必然翻红**（`"FreeTier"` 与 `"freetierplus"` 不再相等）。这是本次改造唯一会因改名而**功能性地**破坏既有断言的用例，已并入 Task 3（改为 `FreeTierPlus` 以保留「大小写变体也认领」的原意）。
- **`.gitignore:8` 的 `/freetier2api-plugin`**：它是 Go 模块名派生的构建产物名（未指定 `-o` 时 `go build` 落盘的默认文件名），与插件身份无关，**不纳入**防残留扫描（无扩展名，天然被跳过）。**不要改它。**

---

## 文件与改动清单（施工前先读这张表）

| 文件（绝对路径以仓库根 `D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main\` 为基准） | 动作 | 归属 Task |
| --- | --- | --- |
| `internal/app/main.go` | 改 `pluginID`/`pluginDisplayName`/`pluginRepository`；改 2 处注释 | T1 |
| `internal/core/vendor.go:31` | 改 `ProviderKey` | T1 |
| `internal/admin/config.go` | 改 `DefaultStateDirName`/`StateDirEnvOverride`/`filepath.Join(cpaDir, ...)`/1 处注释 | T2 |
| `internal/admin/admin.go:120` | 改资源页描述 | T4 |
| `internal/admin/page/00-head.html:6`、`10-body.html:3`、`30-accounts.html:214` | 改显示名 | T4 |
| `internal/admin/page/20-core.html:10` | 改注释示例路径 | T4 |
| `internal/admin/page/50-models.html:38,323` | 改 localStorage 键；`configPath` 改为 `BASE + "/config"` | T4 |
| `internal/admin/page/70-login.html:321,324` | 改 auth-url 路由 | T4 |
| `internal/logger/logger.go:106,125,144,208` | 改 stderr 前缀与日志文件名前缀 | T5 |
| `internal/logger/hostsink.go:140,207` | 改 stderr 前缀与来源段 | T5 |
| `internal/logger/hostsink_test.go:39,131,152,198` | 改来源段与临时目录名 | T5 |
| `internal/vendors/qoder/credential.go:462` | 改 `embeddedIdentityDir` | T5 |
| `internal/vendors/codearts/login.go:69,180`、`codearts/credential.go:103` | `"freetier"` → `core.ProviderKey` | T3 |
| `internal/vendors/zcode/credential.go:194,234` | 同上 | T3 |
| `internal/vendors/trae/credential.go:125`、`trae/login.go:80,497,520` | 同上 | T3 |
| `internal/vendors/cline/credential.go:187` | 同上 + 注释说明「排除法」 | T3 |
| `internal/vendors/workbuddy/credential.go:427,438`（注释） | 文案改 `freetierplus` | T3 |
| `internal/vendors/workbuddy/credential_test.go:137` | `"FreeTier"` → `"FreeTierPlus"`；新增 1 条 legacy type 用例 | T3 |
| `.github/workflows/ci.yml:109,123,124,135,136,174,212,215,216,217,221,222,223` | 产物名 / `-id` / artifact / Release 正文 | T8 |
| `scripts/build.sh:15,34,36`、`scripts/godocker.sh:28`、`scripts/packstore/main.go:17`（示例注释） | 改产物名与卷名 | T8 |
| `registry.json:5,6,9,10` | id / name / repository / homepage | T8 |
| `README.md`、`README_EN.md` 多处 | 同步文档 | T8 |
| `tests/integration/identity_residue_test.go` | **新建**（防残留守卫） | T1 |
| 既有测试（21+ 个，见各 Task） | 按语义更新 | T2–T7 |

---

## 防残留测试（Fixpoint Guard）设计

一个 `go test` 用例，扫描仓库内**受管文件集**，断言 `freetier` 在「身份语境」下零残留。它不追求「全仓无 freetier」（`go.mod` 模块名与 import 路径必须保留），而追求「**身份字面量的白名单封闭**」。

**扫描根目录**：`../..`（即 `tests/integration/` 上两级，仓库根）。
**跳过目录**：`.git`、`docs`、`cpasdk`、`dist`。
**跳过单文件**：`tests/integration/identity_residue_test.go` 自身（它就是扫描器，本体必然含有 `freetier` 字面量与诊断文案；它是纯测试文件，无生产影响）。
**纳入扩展名**：`.go`、`.html`、`.json`、`.yml`、`.md`、`.sh`。

**三段判据（按顺序执行，先剔除合法路径，再判身份残留）：**

1. **剔除 Go 模块路径与新身份**：把整行转小写后依次 `ReplaceAll` 掉 `freetier2api-plugin` 与 `freetierplus` 两个子串。**这一步绝对不能省 `freetierplus`**——目标值本身包含 `freetier` 前缀，若只按 `freetier` 搜子串会把所有正确的新写法判成残留（Go 的 RE2 不支持负向前瞻，无法用 `(?i)freetier(?!plus)` 一步完成）。
2. **剔除显式豁免（allowlist，按 `文件相对路径 → 允许的残留内容` 精确匹配）**：
   - `internal/vendors/autoclaw/chat.go` → 允许 `"x_trace_id", "freetier"`（只需允许该行含 `x_trace_id`）。
   - `internal/vendors/opencodezen/` 下**全部**文件 → 允许任意大小写的 `freetier`（这些是 FreeTier 免费层语义）。
   - `README.md` / `README_EN.md` → 允许**已被明确标注为原版仓库历史**的行（本方案规定：升级说明段落里提到原版 ID 的行必须带「原版」或 `original` 字样；因此允许「含 `原版`/`original upstream` 且含 `freetier`」的行）。
   - `docs/` 目录整体跳过（spec/plan 本身反复提到 `freetier`）。
3. **其余任何在剔除后仍含 `freetier`（大小写不敏感）的行都判失败**，错误信息带上文件相对路径、行号与整行内容。

同时断言四条硬身份（不依赖扫描）：
- `app.PluginID == "freetierplus"`
- `app.ProviderKey == "freetierplus"` 且 `== core.ProviderKey`
- `app.ManagementRoutePrefix == "/plugins/freetierplus"`
- `admin.DefaultStateDirName == ".freetierplus-plugin"`、`admin.StateDirEnvOverride == "FREETIERPLUS_PLUGIN_HOME"`

**为什么这组测试可以「先写、先红」**：改造前 `app.PluginID == "freetier"`，四条断言全部失败；扫描段也会列出全部待改行。它把「目标状态」锁死成一个可执行规格，之后每个 Task 只需让它的失败行数单调递减，最终归零。

---

## Task 1：锁定目标身份（常量单点收口 + 防残留守卫建立）

**Files:**
- Modify: `internal/core/vendor.go:22-31`（`ProviderKey` 及其上方注释）
- Modify: `internal/app/main.go:3,20-23,76-79`
- Create: `tests/integration/identity_residue_test.go`
- Test: `tests/integration/registry_test.go:92-99`

**Interfaces:**
- Consumes: 无（本 Task 是链条起点）。
- Produces: `core.ProviderKey == "freetierplus"`（包级常量）；`app.PluginID == "freetierplus"`、`app.PluginDisplayName` 不导出但 `admin.Identity.DisplayName == "FreeTier Plus"`；`app.ManagementRoutePrefix == "/plugins/freetierplus"`（由 `pluginID` 推导，无需单独改）。后续所有 Task 依赖这两个常量已是新值。

- [ ] **Step 1: 先写失败守卫测试**

新建 `tests/integration/identity_residue_test.go`（包名 `tests`，与同目录其他文件一致）：

```go
package tests

// 本文件是 FreeTier Plus 身份改造的「定点守卫」。
//
// 它把插件身份钉成一个可执行规格：改名为 freetierplus 之后，仓库里不允许
// 再出现任何**身份语境**下的 freetier 字面量。判据不是「全仓无 freetier」——
// go.mod 的模块名与 200+ 处 import 路径必须保留，autoclaw 的上游指纹头、
// opencodezen 的免费层语义也必须保留。这里按「剔除合法项后仍残留」判定。

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"freetier2api-plugin/internal/admin"
	"freetier2api-plugin/internal/app"
	"freetier2api-plugin/internal/core"
)

// 身份硬断言：目标状态的四条基本事实。
func TestPluginIdentityIsFreeTierPlus(t *testing.T) {
	if app.PluginID != "freetierplus" {
		t.Fatalf("app.PluginID = %q, want freetierplus", app.PluginID)
	}
	if app.ProviderKey != "freetierplus" || core.ProviderKey != "freetierplus" {
		t.Fatalf("provider key = %q / %q, want freetierplus", app.ProviderKey, core.ProviderKey)
	}
	if app.ManagementRoutePrefix != "/plugins/freetierplus" {
		t.Fatalf("management prefix = %q, want /plugins/freetierplus", app.ManagementRoutePrefix)
	}
	if admin.DefaultStateDirName != ".freetierplus-plugin" {
		t.Fatalf("state dir name = %q", admin.DefaultStateDirName)
	}
	if admin.StateDirEnvOverride != "FREETIERPLUS_PLUGIN_HOME" {
		t.Fatalf("state dir env = %q", admin.StateDirEnvOverride)
	}
}

// residueAllowed 是身份语境下允许保留 freetier 的文件（键为相对仓库根的
// 斜杠路径前缀，值为该文件内的豁免说明）。
var residueAllowed = map[string]string{
	"internal/vendors/autoclaw/chat.go": "上游请求指纹头 x_trace_id",
}

// residueLineAllowed 判定某行是否属于「合法残留」。调用方已剔除模块路径与新身份
// （freetier2api-plugin / freetierplus），因此进入本函数的行里出现的 freetier
// 一定是需要裁决的形态。
func residueLineAllowed(rel string, line string) bool {
	if _, ok := residueAllowed[rel]; ok && strings.Contains(line, "x_trace_id") {
		return true
	}
	// opencodezen 的免费层语义（isFreeTierModel / prepareFreeTierBody /
	// FreeTierError / free tier）全部合法。
	if strings.HasPrefix(rel, "internal/vendors/opencodezen/") {
		return true
	}
	// README 的升级说明里会提到原版 ID：只允许显式标注「原版 / original」的行。
	if rel == "README.md" && strings.Contains(line, "原版") {
		return true
	}
	if rel == "README_EN.md" && (strings.Contains(line, "original upstream") || strings.Contains(line, "the original")) {
		return true
	}
	return false
}

// TestNoResidualFreeTierIdentity 扫描受管文件集，断言身份语境下零残留。
func TestNoResidualFreeTierIdentity(t *testing.T) {
	root := filepath.Join("..", "..")
	skipDirs := map[string]bool{".git": true, "docs": true, "cpasdk": true, "dist": true}
	// 本文件是扫描器本体：它必然含有 freetier 字面量与失败文案，跳过自身。
	self := "tests/integration/identity_residue_test.go"
	managedExt := map[string]bool{".go": true, ".html": true, ".json": true, ".yml": true, ".md": true, ".sh": true}
	// 大小写不敏感：FreeTier 也是身份残留（显示名/注释）。
	residue := regexp.MustCompile(`(?i)freetier`)

	var offenders []string
	errWalk := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDirs[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !managedExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		raw, errRead := os.ReadFile(path)
		if errRead != nil {
			return errRead
		}
		rel, errRel := filepath.Rel(root, path)
		if errRel != nil {
			return errRel
		}
		rel = filepath.ToSlash(rel)
		if rel == self {
			return nil
		}
		for index, line := range strings.Split(string(raw), "\n") {
			// 先剔除 Go 模块路径与新身份：freetierplus 本身含 freetier 前缀，
			// 不剔掉它会把所有正确写法判成残留。
			stripped := strings.ReplaceAll(line, "freetier2api-plugin", "")
			stripped = strings.ReplaceAll(stripped, "freetierplus", "")
			if !residue.MatchString(stripped) {
				continue
			}
			if residueLineAllowed(rel, stripped) {
				continue
			}
			offenders = append(offenders, rel+":"+strconv.Itoa(index+1)+": "+strings.TrimSpace(line))
		}
		return nil
	})
	if errWalk != nil {
		t.Fatalf("walk repo: %v", errWalk)
	}
	if len(offenders) > 0 {
		t.Fatalf("仍有 %d 处身份残留 freetier（应为 freetierplus）：\n%s",
			len(offenders), strings.Join(offenders, "\n"))
	}
}
```

- [ ] **Step 2: 验证红灯（CI）**

```bash
git add tests/integration/identity_residue_test.go
git commit -m "test: 锁定 freetierplus 身份目标并建立防残留守卫"
git push
env -u SSL_CERT_DIR -u NODE_EXTRA_CA_CERTS gh run list --workflow=ci.yml --branch feat/freetierplus-coexistence --limit 1
```
预期：`go test` 步骤失败。`--log-failed` 中必须同时出现：
- `app.PluginID = "freetier", want freetierplus`
- `仍有 N 处身份残留 freetier`，且清单里至少含 `internal/app/main.go:`、`internal/core/vendor.go:31:`、`internal/admin/config.go:`、`internal/logger/logger.go:`

若红灯文案里出现「`freetier2api-plugin`」被判为残留（而非仅身份行），说明 Step 1 的预处理有误，**必须先修正守卫再继续**。

- [ ] **Step 3: 最小实现**

1. `internal/core/vendor.go` 第 31 行：`const ProviderKey = "freetierplus"`，并把第 22-30 行注释中的「provider 键」表述改为 `freetierplus`（注释里不得再出现 `freetier` 单独字样）。
2. `internal/app/main.go:20-23` 改为：

```go
	pluginID          = "freetierplus"
	pluginDisplayName = "FreeTier Plus"
	pluginAuthor      = "cxy2730"
	pluginRepository  = "https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin"
```

（`pluginAuthor` 保持 `cxy2730`：Spec §3.1 明确「可保持或标注维护者信息，不影响冲突隔离」，且不含身份字面量。）

3. 同文件第 3 行注释 `// freetier —— 多供应商免费额度插件。` → `// freetierplus —— 多供应商免费额度插件。`
4. 同文件第 78 行注释里的 `type=freetier` → `type=freetierplus`；第 76 行 `FreeTier 2API` → `FreeTier Plus`。
5. `tests/integration/registry_test.go:93` 的 `if app.PluginID != "freetier"` → `if app.PluginID != "freetierplus"`（该断言语义是「锁死当前 ID 并要求同步改产物名」，改值即可，**不得删除**）。

- [ ] **Step 4: 验证绿灯（局部）**

```bash
git add -A && git commit -m "feat(core): 插件身份改为 freetierplus（PluginID/ProviderKey/仓库地址）"
git push
```
预期：CI 的 `go test` 仍在**其他**残留行上失败（这是预期的，Task 1 只负责关闭「常量与 main.go」这一批），但失败清单里**不再包含** `internal/app/main.go`、`internal/core/vendor.go`、`tests/integration/registry_test.go`。`TestPluginIdentityIsFreeTierPlus` 必须全绿。`go vet` 与 `gofmt` 必须绿。

- [ ] **Step 5: 本地提交**

```bash
git add -A
git commit -m "feat(core): 插件身份改为 freetierplus（PluginID/ProviderKey/仓库地址）"
```

---

## Task 2：状态目录与环境变量隔离

**Files:**
- Modify: `internal/admin/config.go:16,56,58,106`
- Test: `internal/admin/config_test.go:64-69`（环境变量用例，改用常量，无需改逻辑）
- Test: `tests/integration/plugin_models_test.go:428-449`

**Interfaces:**
- Consumes: 无（纯字面量）。
- Produces: `admin.DefaultStateDirName = ".freetierplus-plugin"`、`admin.StateDirEnvOverride = "FREETIERPLUS_PLUGIN_HOME"`；`admin.DefaultStateDir()` 在有 `~/.cli-proxy-api` 时返回 `filepath.Join(cpaDir, "freetierplus")`。

- [ ] **Step 1: 先写失败测试**

修改 `tests/integration/plugin_models_test.go` 的 `TestDefaultStateDirFallback`（第 428-449 行），把三处期望改为新值：

```go
	t.Setenv("FREETIERPLUS_PLUGIN_HOME", "")

	// 1. 无 ~/.cli-proxy-api → 回退 ~/.freetierplus-plugin
	dir1 := app.DefaultStateDir()
	want1 := filepath.Join(tempHome, ".freetierplus-plugin")
	if dir1 != want1 {
		t.Fatalf("defaultStateDir = %q, want %q", dir1, want1)
	}

	// 2. 存在 ~/.cli-proxy-api（Docker 挂载场景）→ 用其下 freetierplus 子目录
	cpaDir := filepath.Join(tempHome, ".cli-proxy-api")
	if errMkdir := os.MkdirAll(cpaDir, 0o755); errMkdir != nil {
		t.Fatalf("create temp cpa dir: %v", errMkdir)
	}
	dir2 := app.DefaultStateDir()
	want2 := filepath.Join(cpaDir, "freetierplus")
	if dir2 != want2 {
		t.Fatalf("defaultStateDir with cpa dir = %q, want %q", dir2, want2)
	}
```

**关键**：`t.Setenv` 的第一个参数必须从 `"FREETIER2API_PLUGIN_HOME"` 改为 `"FREETIERPLUS_PLUGIN_HOME"`——否则该变量不再被识别，测试会因为宿主环境残留而变得不确定。`internal/admin/config_test.go:65` 用的是 `StateDirEnvOverride` 常量（`t.Setenv(StateDirEnvOverride, ...)`），**无需修改**，改常量后自动跟随。

- [ ] **Step 2: 验证红灯（CI）**

```bash
git add tests/integration/plugin_models_test.go
git commit -m "test: 状态目录改为 freetierplus 的期望"
git push
```
预期 `go test` 失败于 `defaultStateDir = ".../.freetier-plugin", want ".../.freetierplus-plugin"`。

- [ ] **Step 3: 最小实现**

`internal/admin/config.go`：

```go
	// DefaultStateDirName 是默认状态目录名。
	DefaultStateDirName = ".freetierplus-plugin"
	// StateDirEnvOverride 是状态目录的环境变量覆盖项。
	StateDirEnvOverride = "FREETIERPLUS_PLUGIN_HOME"
```

第 106 行：`return filepath.Join(cpaDir, "freetierplus")`。
第 16 行注释：`// PluginConfig 是 plugins.configs.freetierplus 解析出的有效全局配置。`

- [ ] **Step 4: 验证绿灯（CI）**

```bash
git add -A && git commit -m "feat(admin): 状态目录与环境变量改为 freetierplus"
git push
```
预期：`TestDefaultStateDirFallback` 与 `TestDefaultStateDirHonoursEnv` 全绿；防残留清单里 `internal/admin/config.go` 消失（`tests/integration/plugin_models_test.go` 也消失）。

- [ ] **Step 5: 本地提交**

```bash
git add -A && git commit -m "feat(admin): 状态目录与环境变量改为 freetierplus"
```

---

## Task 3：供应商侧硬编码收口到 `core.ProviderKey`

**Files:**
- Modify: `internal/vendors/codearts/login.go:69,180`；`internal/vendors/codearts/credential.go:103`
- Modify: `internal/vendors/zcode/credential.go:194,234`
- Modify: `internal/vendors/trae/credential.go:125`；`internal/vendors/trae/login.go:80,497,520`
- Modify: `internal/vendors/cline/credential.go:184-190`（含注释）
- Modify: `internal/vendors/workbuddy/credential.go:427,438`（仅注释）
- Test: `internal/vendors/cline/credential_test.go`（新增 2 条用例）
- Test: `internal/vendors/workbuddy/credential_test.go`（改 1 条、新增 1 条）
- Test: `internal/vendors/zcode/credential_test.go:246`、`internal/vendors/cline/credential_test.go:32`、`internal/vendors/opencodezen/credential_test.go:27`（调用参数）

**Interfaces:**
- Consumes: `core.ProviderKey == "freetierplus"`（Task 1）。
- Produces: 五个供应商（codearts / zcode / trae / cline / workbuddy）写出的凭证 `"type"` 与 `AuthData.Provider` 均为 `core.ProviderKey`。所有受影响的包都已 import `"freetier2api-plugin/internal/core"`（已核实：`codearts/login.go:24`、`trae/login.go:18`、`trae/credential.go:8`、`zcode/credential.go:13`、`cline/credential.go:16`、`codearts/credential.go:9`），**无需新增 import**。

- [ ] **Step 1: 先写失败测试**

**1a. `internal/vendors/cline/credential_test.go`** —— 在 `TestLooksLikeCredential_Cline` 的 `cases` 里新增两条（插在 `{"workbuddy type rejected", ...}` 之后）：

```go
		// 双开共存：原版 freetier 的凭证对本插件是「别家」，必须拒绝认领；
		// 本插件自己写的 type 不是别家。这两条锁死改造后的 type 判据。
		{"legacy freetier type is foreign", "acc.json", map[string]any{"type": "freetier", "refreshToken": "rt"}, false},
		{"provider type is ours", "acc.json", map[string]any{"type": "freetierplus", "refreshToken": "rt"}, true},
```

**1b. `internal/vendors/zcode/credential_test.go:246`** —— `TestMergeStorageJSONPreservesUserKeys` 的 `original` 从 `{"type":"freetier",...}` 改为 `{"type":"freetierplus",...}`。该测试的意图是「合并时保留用户手写字段」，与 type 的具体值无关，改值是为了避免守卫误判为残留，同时保持语义（合并结果会无条件重写 type）。

**1c. `internal/vendors/workbuddy/credential_test.go:137` —— 必改，否则 Task 3 一定会红**

现状 `{"type field uppercase", map[string]any{"type": "FreeTier"}, "x.json", "", true}` 依赖 `strings.EqualFold("FreeTier", ProviderKey)`。`ProviderKey` 改成 `freetierplus` 后，`"FreeTier"` 与 `"freetierplus"` 不再相等，该用例按 filename / 嵌套 / domain 三条兜底判据都不命中，结果从 `true` 翻成 `false`。**必须**改为 `"FreeTierPlus"`（保留「大小写变体也应被认领」的原意）：

```go
		{"type field uppercase", map[string]any{"type": "FreeTierPlus"}, "x.json", "", true},
```

**1d. `internal/vendors/workbuddy/credential_test.go` —— 新增 1 条双开语义用例**

在 `{"foreign type", ...}` 附近追加：

```go
		// 双开共存：原版插件写的 type=freetier 不再被本插件按 type 认领。
		// 该凭证若文件名/结构仍属本协议（如 workbuddycn-*.json），由后两条
		// 兜底判据收下；这里的"裸文件名 + 无其它证据"形态必须拒绝。
		{"legacy freetier type alone is foreign", map[string]any{"type": "freetier"}, "x.json", "", false},
```

注意与既有 `{"type field uppercase", ...}` 的区别：后者断言的是**大小写变体**仍被认领（`FreeTierPlus`），前者断言的是**旧身份字面量**被拒绝（`freetier`）。两条必须同时存在，否则无法区分「大小写兼容」与「跨版本隔离」。

**1e. 三个 `LooksLikeCredential` 调用点**（`cline/credential_test.go:32`、`opencodezen/credential_test.go:27`、`zcode/adapter_test.go:31,34,37,42,45,48,53,58`）—— 这些测试把 `"freetier"` 作为 **provider 提示**传入。改造后该提示不再是本插件的 provider key。处理原则：

- `cline/credential_test.go:32` 与 `opencodezen/credential_test.go:27`：把 `"freetier"` 改为 **空串 `""`**。理由：这两个 `LooksLikeCredential` 实现都 `_ = provider`（`cline/credential.go:146`、`opencodezen/credential.go:110`），provider 参数无作用；传空串既消除残留又不改变任何断言结果。
- `zcode/adapter_test.go` 的 8 处：改为 `core.ProviderKey`。该文件已 import `core`（`:5`），且 `zcode` 的 `Match` 也忽略 provider 参数（`adapter.go:128-130`），改的是测试输入的语义正确性。

- [ ] **Step 2: 验证红灯（CI）**

```bash
git add -A
git commit -m "test: 锁定 type 判据与 provider 提示的新语义"
git push
```
预期 `go test` 失败于：
- `LooksLikeCredential("acc.json") = true, want false`（cline 的 `legacy freetier type is foreign` 用例：改造前 `"freetier"` 被当作非 foreign，故返回 true）
- `LooksLikeCredential("acc.json") = false, want true`（`provider type is ours` 用例：改造前 `"freetierplus"` 被判 foreign）
- `LooksLikeCredential(x.json) = true, want false`（workbuddy 的 `legacy freetier type alone is foreign` 用例：改造前 `"freetier"` 与 `ProviderKey` 相等故被认领）

若这三条没有红，说明 type 判据的调用链理解有误，**停止并复核**。

- [ ] **Step 3: 最小实现**

统一模式：把字面量 `"freetier"` 替换为 `core.ProviderKey`。

1. `internal/vendors/codearts/login.go:69` 与 `:180` 的 `Provider: "freetier"` → `Provider: core.ProviderKey`。
2. `internal/vendors/codearts/credential.go:103` 的 `"type": "freetier"` → `"type": core.ProviderKey`。
3. `internal/vendors/zcode/credential.go:194` 的 `"type": "freetier"` → `"type": core.ProviderKey`；`:234` 的 `m["type"] = "freetier"` → `m["type"] = core.ProviderKey`。
4. `internal/vendors/trae/credential.go:125` → `"type": core.ProviderKey`。
5. `internal/vendors/trae/login.go:80` 与 `:520` 的 `Provider: "freetier"` → `Provider: core.ProviderKey`；`:497` 的 `"type": "freetier"` → `"type": core.ProviderKey`。
6. `internal/vendors/cline/credential.go:187`：

```go
		// 排除法：只有明确不是任何已知 type 的字面量才判「别家特征」。
		// core.ProviderKey 是本插件（freetierplus）；"cline" 兼容手工导入。
		// 原版 freetier 插件的凭证在此被判为别家——双开时两侧互不认领，
		// 这正是共存想要的方向。
		if trimmed != "" && trimmed != core.ProviderKey && trimmed != "cline" {
			return true
		}
```

7. `internal/vendors/workbuddy/credential.go:427` 注释里的 `是 freetier` → `是 freetierplus`；`:438` 注释里的 `支持当前 freetier 及历史` → `支持当前 freetierplus 及历史`。（`:439` 的代码已是 `ProviderKey` 常量引用，**不改代码**。）

**注意**：`internal/vendors/workbuddy/credential.go:439` 的 `for _, expected := range []string{ProviderKey, "workbuddy", "codebuddy"}` 在改造后只认 `freetierplus`，不再认 `freetier`。这是双开想要的方向（原版凭证不按 type 归本插件），但**原版插件写出的 `workbuddycn-*.json` 仍会由文件名判据（`:449`）被收下**——这正是风险 A 描述的行为，Task 7 已锁定。

- [ ] **Step 4: 验证绿灯（CI）**

```bash
git add -A && git commit -m "refactor(vendors): 凭证 type 与 Provider 统一引用 core.ProviderKey"
git push
```
预期：`internal/vendors/...` 全部包测试绿；`TestLooksLikeCredential_Cline` 新增两条绿；防残留清单里 `internal/vendors/codearts/`、`zcode/`、`trae/`、`cline/`、`workbuddy/credential.go` 全部消失。

- [ ] **Step 5: 本地提交**

```bash
git add -A && git commit -m "refactor(vendors): 凭证 type 与 Provider 统一引用 core.ProviderKey"
```

---

## Task 4：管理界面与控制台页面

**Files:**
- Modify: `internal/admin/admin.go:120`
- Modify: `internal/admin/page/00-head.html:6`、`10-body.html:3`、`30-accounts.html:214`、`20-core.html:10`、`50-models.html:38,323`、`70-login.html:321,324`
- Test: 见下（8 个 admin 测试文件 + `routes_cover_test.go`）

**Interfaces:**
- Consumes: 无（页面是静态字符串 + `__BASE__` 占位符替换，见 `internal/admin/console.go:75-77`）。
- Produces: `Page("/plugins/freetierplus")` 产出的 HTML 不含 `freetier` 残留；`configPath` 随 `BASE` 动态派生。

- [ ] **Step 1: 先写失败测试**

**1a. `internal/admin/console_test.go`** —— 逐处按语义改：

| 行 | 现状 | 改为 |
| --- | --- | --- |
| 41,109,245,292,319,360,406,419,443,483,511,533,558,593,612,654,702,729,753,769,797,813,833,870 | `Page("/plugins/freetier")` | `Page("/plugins/freetierplus")` |
| 322 | `"/freetier-auth-url",` | `"/freetierplus-auth-url",` |
| 335 | 注释里的 `"plugins/freetier/workbuddy"` | `"plugins/freetierplus/workbuddy"` |
| 339 | `api("GET", "/freetier-auth-url` | `api("GET", "/freetierplus-auth-url` |
| 563 | `` `hostPath("/plugins/freetier/config")`, `` | `` `hostPath(BASE + "/config")`, `` |
| 660 | `` `"freetier-model-optional-columns"`, `` | `` `"freetierplus-model-optional-columns"`, `` |

**关键（第 563 行）**：这条断言必须改为**断言新写法**，不得删除。改造后页面里不再有 `/plugins/freetier/config` 字面量，`configPath` 由 `hostPath(BASE + "/config")` 派生，而 `BASE` 在页面里被注入成 `/plugins/freetierplus`（由 `Page()` 的实参决定）。断言 `hostPath(BASE + "/config")` 才能同时锁住「不再硬编码」与「跟随 BASE」。

**1b. `internal/admin/console_page_test.go`** —— `:25,48,59,74,95,118` 的 `Page("/plugins/freetier")` → `Page("/plugins/freetierplus")`；`:59` 的断言 `` `var BASE = "/plugins/freetier"` `` → `` `var BASE = "/plugins/freetierplus"` ``。

**1c. `internal/admin/console_quota_test.go:15,39,62`**、**`console_r2_test.go:17,45`**、**`routes_cover_test.go:20`** —— 同上，`Page("/plugins/freetier")` → `Page("/plugins/freetierplus")`。

**1d. `internal/admin/admin_test.go`**：

- `:33` stub 身份改为 `Identity{ID: "freetierplus", DisplayName: "FreeTier Plus", ProviderKey: "freetierplus", Version: "test"}`
- `:105,130,142,161,233,246` 的 URL 路径 `/plugins/freetier/...` → `/plugins/freetierplus/...`
- `:179` 的 `"FreeTier 2API"` → `"FreeTier Plus"`

**1e. `internal/admin/login_test.go`**：

- `:23,54,231,232,249,267,284,299` 的 `/plugins/freetier/...` → `/plugins/freetierplus/...`
- `:143` 的 `Provider: "freetier",` → `Provider: core.ProviderKey,`（该文件已 import `core`，见 `:14`）
- `:146` 的 `[]byte("{\"type\":\"freetier\",\"vendor\":\"traecn\"}")` → `[]byte("{\"type\":\"freetierplus\",\"vendor\":\"traecn\"}")`

- [ ] **Step 2: 验证红灯（CI）**

```bash
git add -A && git commit -m "test: 控制台页面的 freetierplus 期望"
git push
```
预期失败清单：`console page is missing the add-account wiring (/freetierplus-auth-url)`、`console page missing model-sync wiring (hostPath(BASE + "/config"))`、`console page missing column-visibility wiring "freetierplus-model-optional-columns"`、`管理接口前缀没有注入正确的 base path`、`console page missing "FreeTier Plus"`。

- [ ] **Step 3: 最小实现**

1. `internal/admin/admin.go:120` → `Description: "FreeTier Plus 账号、额度与模型管理页。",`
2. `page/00-head.html:6` → `<title>FreeTier Plus</title>`
3. `page/10-body.html:3` → `  <h1>FreeTier Plus</h1>`
4. `page/30-accounts.html:214` → 文案里的 `FreeTier 2API` → `FreeTier Plus`
5. `page/20-core.html:10` 注释里的 `"plugins/freetier/workbuddy"` → `"plugins/freetierplus/workbuddy"`
6. `page/50-models.html:38` → `var MODEL_COLUMNS_STORE_KEY = "freetierplus-model-optional-columns";`
7. `page/50-models.html:323` → `var configPath = hostPath(BASE + "/config");`（`BASE` 在 `20-core.html:4` 定义，同为 IIFE 内作用域，`50-models.html` 可访问；已核实 `<script>` 只在 `20-core.html:1` 开、`80-columns.html:265` 关，全页单作用域）
8. `page/70-login.html:321` 注释里的 `/freetier-auth-url` → `/freetierplus-auth-url`
9. `page/70-login.html:324` → `api("GET", hostPath("/freetierplus-auth-url?vendor=" + encodeURIComponent(vendor.id))).then(function (data) {`

- [ ] **Step 4: 验证绿灯（CI）**

```bash
git add -A && git commit -m "feat(admin): 控制台页面与显示名改为 FreeTier Plus"
git push
```
预期：`internal/admin` 全包绿（含 `-race`）；`TestConsolePageWiresHostOAuthLogin`、`TestConsoleModelChangesTriggerHostReload`、`TestConsoleModelOptionalColumnsHiddenByDefault`、`TestPageAssemblesAllFragments` 全绿。

- [ ] **Step 5: 本地提交**

```bash
git add -A && git commit -m "feat(admin): 控制台页面与显示名改为 FreeTier Plus"
```

---

## Task 5：日志文件、stderr 前缀与 Qoder 嵌入缓存目录

**Files:**
- Modify: `internal/logger/logger.go:106,125,144,208`
- Modify: `internal/logger/hostsink.go:140,207`
- Modify: `internal/vendors/qoder/credential.go:462`
- Test: `internal/logger/hostsink_test.go:39,131,152,198`

**Interfaces:**
- Consumes: 无。
- Produces: 日志文件 `freetierplus-<date>.log`；宿主日志行来源段 `[freetierplus.go:0] [freetierplus]`；Qoder 嵌入组件解包到 `<UserCacheDir>/freetierplus/umid-<hash>/`。

- [ ] **Step 1: 先写失败测试**

`internal/logger/hostsink_test.go`：

- `:39` `want` 字符串 → `"[2026-11-02 12:26:41] [--------] [info ] [freetierplus.go:0] [freetierplus] 已解析授权\n"`
- `:131,152` `filepath.Join(base, ".freetier-plugin")` → `filepath.Join(base, ".freetierplus-plugin")`（这两处只是临时目录命名，但必须改，否则防残留守卫命中）
- `:198` `"[freetier.go:0] [freetier]"` → `"[freetierplus.go:0] [freetierplus]"`

- [ ] **Step 2: 验证红灯（CI）**

```bash
git add -A && git commit -m "test: 日志来源段与状态目录名改 freetierplus"
git push
```
预期 `go test ./internal/logger` 失败于 `info 行格式不符 got=... want="[...] [freetierplus.go:0] [freetierplus] ..."`。

- [ ] **Step 3: 最小实现**

1. `internal/logger/logger.go:106`：`fmt.Fprintf(os.Stderr, "[freetierplus] close log file failed: %v\n", errClose)`
2. `:125`：`path := filepath.Join(fileDir, "freetierplus-"+day+".log")`
3. `:144`：`if entry.IsDir() || !strings.HasPrefix(entry.Name(), "freetierplus-") {`
4. `:208`：`fmt.Fprintf(os.Stderr, "[freetierplus] rotate log file failed: %v\n", errRotate)`
5. `internal/logger/hostsink.go:140`：`fmt.Fprintf(os.Stderr, "[freetierplus] 宿主日志镜像不可用（%s）：%v\n", h.path, err)`
6. `:207`：`return fmt.Sprintf("[%s] [--------] [%-5s] [freetierplus.go:0] [freetierplus] %s\n", ...)`
7. `internal/vendors/qoder/credential.go:462`：`embeddedIdentityDir = "freetierplus"`

- [ ] **Step 4: 验证绿灯（CI）**

```bash
git add -A && git commit -m "feat(logger): 日志文件名/stderr 前缀/Qoder 缓存目录改为 freetierplus"
git push
```
预期：`internal/logger` 全包绿；`internal/vendors/qoder` 全包绿（`models_test.go`、`tasks*_test.go` 不涉及该常量，但 `int*` 无编译问题）；防残留清单里 `internal/logger/` 与 `internal/vendors/qoder/credential.go` 消失。

- [ ] **Step 5: 本地提交**

```bash
git add -A && git commit -m "feat(logger): 日志文件名/stderr 前缀/Qoder 缓存目录改为 freetierplus"
```

---

## Task 6：集成测试与路由前缀的语义更新

**Files:**
- Test: `tests/integration/testutil_test.go:225`、`management_resilience_test.go:199`、`activity_probe_test.go:89`
- Test: `tests/integration/identity_test.go:59,92`、`model_fallback_test.go:20`
- Test: `internal/app/auth_identifier_test.go:15,39,40,51`
- Test: `internal/tasks/checkin_test.go:412`
- Test: `tests/integration/auth_test.go:75`（仅注释）

**Interfaces:**
- Consumes: Task 1（`core.ProviderKey`）与 Task 4（路由前缀）。
- Produces: 集成测试的 `managementCall` 路径前缀与生产路由一致。

- [ ] **Step 1: 先写失败测试（本次全部是既有断言的值更新）**

1. `tests/integration/testutil_test.go:225` → `Path: "/v0/management/plugins/freetierplus" + path,`
2. `tests/integration/management_resilience_test.go:199` → 同上
3. `tests/integration/activity_probe_test.go:89` → `Method: "POST", Path: "/v0/management/plugins/freetierplus/activities/preview",`
4. `tests/integration/identity_test.go:59` → `const qoderCredential = `{"type":"freetierplus","vendor":"qodercn",...``；`:92` → `{"type":"freetierplus","vendor":"workbuddycn",...`
5. `tests/integration/model_fallback_test.go:20` → `const qoderCredentialJSON = `{"type":"freetierplus","vendor":"qodercn",...`
6. `internal/app/auth_identifier_test.go`：
   - `:15` 注释 `type=freetier` → `type=freetierplus`
   - `:39` `env.Data.CredentialProviders[0] != "freetier"` → `!= "freetierplus"`；`:40` 文案 → `must claim freetierplus`
   - `:51` `e.Data.Identifier != "freetier"` → `!= "freetierplus"`
7. `internal/tasks/checkin_test.go:412`：`"provider": "freetier"` → `"provider": "freetierplus"`
8. `tests/integration/auth_test.go:75` 注释里的 `provider == "freetier"` → `provider == "freetierplus"`

**为何这些必须先红**：`testutil_test.go:225` 是全部管理接口集成测试的入口；改造后生产路由是 `/plugins/freetierplus`，若测试仍打 `/plugins/freetier`，`Service.Handle` 会走 404 分支（`admin.go:159` 之后的路由匹配）。`auth_identifier_test.go:39` 同理——`providerKey` 已是 `freetierplus`。

- [ ] **Step 2: 验证红灯（CI）**

```bash
git add -A && git commit -m "test: 集成测试路由与凭证 type 跟随 freetierplus"
git push
```
预期：`tests/integration` 大面积失败于 `unknown route` / `status != 200`；`internal/app` 失败于 `credential providers must claim freetierplus`；`internal/tasks` 的签到单测若依赖 provider 过滤则会失败。

- [ ] **Step 3: 最小实现**

本 Task **没有生产代码改动**——它验证的是 Task 1–5 已建立的单点真理源。若 Step 2 之后仍失败，只能说明 Task 1–5 有遗漏，**回到对应 Task 修**，不得在此处打补丁。

- [ ] **Step 4: 验证绿灯（CI）**

```bash
git push
```
预期 `go test ./...` 全绿（这是第一个「全量绿」检查点）。

- [ ] **Step 5: 本地提交**

```bash
git add -A && git commit -m "test: 集成测试路由与凭证 type 跟随 freetierplus"
```

---

## Task 7：双开共存行为锁定（含风险 A 的回归守卫）

**Files:**
- Test: `internal/hostbridge/auth_test.go`（新增 1 个用例）

**Interfaces:**
- Consumes: `hostbridge.filterPluginAuths`（包内私有，测试同包可直接调用）、`core.ProviderKey`。
- Produces: 一条断言「双开时凭证列出的处置与改造前一致」的回归测试。

- [ ] **Step 1: 先写测试（这条测试在 Task 1 之后写就是绿的——它是防回归，不是驱动实现）**

在 `internal/hostbridge/auth_test.go` 末尾追加：

```go
// TestCoexistenceAuthListKeepsCurrentClaimBehavior 锁定双开共存时的凭证列出行为。
//
// 背景：本插件改名为 freetierplus 后，与原版 freetier 同时加载时，
// 同一份 workbuddy*-*.json 对两个插件都满足「文件名属于本插件」的第二层判据。
// 这是**既有的容错设计**（见 filterPluginAuths 的说明：兜住被前置插件抢走、
// provider 被兜底改写的凭证），不是本次改造引入的行为。本用例钉死：
//   - provider=freetierplus 的条目被收下（第一层判据）；
//   - provider=freetier（原版插件写的）但文件名是 workbuddy* 的条目**仍被收下**
//     （第二层判据，与改造前一致——不得因 ProviderKey 改名而改变）；
//   - 别家的文件名（qoder-login-*）在任何情况下都不收。
func TestCoexistenceAuthListKeepsCurrentClaimBehavior(t *testing.T) {
	registry := core.NewRegistry()
	entries := filterPluginAuths([]core.HostAuth{
		{Name: "workbuddycn-u1.json", Provider: core.ProviderKey, AuthIndex: "1"},
		{Name: "workbuddycn-u2.json", Provider: "freetier", AuthIndex: "2"},
		{Name: "qoder-login-f34c07f7.json", Provider: "freetier", AuthIndex: "3"},
		{Name: "claude-1.json", Provider: "anthropic", AuthIndex: "4"},
	}, registry)

	got := make(map[string]bool, len(entries))
	for _, entry := range entries {
		got[entry.Name] = true
	}
	if !got["workbuddycn-u1.json"] {
		t.Fatal("本插件自己的凭证必须被列出")
	}
	if !got["workbuddycn-u2.json"] {
		t.Fatal("原版插件写入、但文件名属于本插件约定的凭证必须仍被列出（第二层判据不得因改名失效）")
	}
	if got["qoder-login-f34c07f7.json"] || got["claude-1.json"] {
		t.Fatalf("别家凭证泄漏：%+v", entries)
	}
}
```

**为什么这条不能先红**：`filterPluginAuths` 的两层判据都不含 `freetier` 字样，改造前后行为本就相同。它的价值是把「不会成为回归源」变成 CI 上的事实。这是本方案中唯一一条「先绿」的测试，属有意的规格锁定（Spec §5「新增/强化防残留校验」的延伸）。

- [ ] **Step 2: 验证（CI）**

```bash
git add -A && git commit -m "test(hostbridge): 锁定双开时凭证列出的既有容错行为"
git push
```
预期：`internal/hostbridge` 全绿，`TestCoexistenceAuthListKeepsCurrentClaimBehavior` PASS。若它**失败**，说明二层判据被误改（例如 Task 3 越界改了 `filterPluginAuths`），**立即回退该改动**。

- [ ] **Step 3: 本地提交**

```bash
git add -A && git commit -m "test(hostbridge): 锁定双开时凭证列出的既有容错行为"
```

---

## Task 8：CI/CD、构建脚本、插件清单与文档

**Files:**
- Modify: `.github/workflows/ci.yml:109,123,124,135,136,174,212,215,216,217,221,222,223`
- Modify: `scripts/build.sh:15,34,36`、`scripts/godocker.sh:28`、`scripts/packstore/main.go:17`（示例注释）
- Modify: `registry.json:5,6,9,10`
- Modify: `README.md:1,13,54,65,75,76,80,89,126,138`
- Modify: `README_EN.md:1,13,54,65,75,76,80,89,126,138`
- Test: `tests/integration/registry_test.go`（已由 Task 1 改值，此处验证）；`tests/integration/identity_residue_test.go`（收尾归零）

**Interfaces:**
- Consumes: Task 1–7 全部完成（防残留扫描必须已只剩本 Task 的文件）。
- Produces: 构建产物 `freetierplus-<goos>-<goarch>.<ext>`、商店归档 `freetierplus_<ver>_<goos>_<goarch>.zip`、zip 内动态库名 `freetierplus.<ext>`（由 `packstore` 的 `entryName := pluginID + filepath.Ext(libPath)` 自动派生，只需 `-id` 正确）。

- [ ] **Step 1: 验证红灯**

```bash
env -u SSL_CERT_DIR -u NODE_EXTRA_CA_CERTS gh run list --workflow=ci.yml --branch feat/freetierplus-coexistence --limit 1
```
此时 `TestNoResidualFreeTierIdentity` 应仍失败，清单里只剩：`.github/workflows/ci.yml`、`scripts/`、`registry.json`、`README.md`、`README_EN.md`。**若清单里出现任何 `internal/` 或 `tests/` 路径，回到对应 Task 补齐，不得在此处越权改。**

- [ ] **Step 2: 最小实现**

**2a. `.github/workflows/ci.yml`** —— 逐行替换：

| 行 | 改为 |
| --- | --- |
| 109 | `            -o "dist/freetierplus-${{ matrix.goos }}-${{ matrix.goarch }}.${{ matrix.ext }}" .` |
| 123 | `            -lib "dist/freetierplus-${{ matrix.goos }}-${{ matrix.goarch }}.${{ matrix.ext }}" \` |
| 124 | `            -id freetierplus \` |
| 135 | `          name: freetierplus-${{ matrix.goos }}-${{ matrix.goarch }}` |
| 136 | `          path: dist/freetierplus_*_${{ matrix.goos }}_${{ matrix.goarch }}.zip` |
| 174 | `            archive="assets/freetierplus_${version}_${goos}.zip"` |
| 212 | `            echo '插件商店里的 \`freetierplus\` 条目指向本仓库，安装前请先添加插件源；'` |
| 215 | `            echo '下载对应平台的 \`freetierplus_<版本>_<goos>_<goarch>.zip\`，解压后把其中的'` |
| 216 | `            echo '\`freetierplus\` 动态库放进 CPA 的 \`plugins/\` 目录。'` |
| 217 | `            echo '归档里已经用的是宿主要求的 \`freetierplus{ext}\` 文件名，解压后无需改名。'` |
| 221-223 | 三条表格行里的 `freetier_` → `freetierplus_` |

**注意**：第 212 行原文提到「仍指向已失效的原仓库」，改造后本仓库的 Release 由自己发布，该句不再成立，因此按上表改写语义（本方案已给出），不得保留「原仓库」措辞——否则会与「双开共存」叙事矛盾。

**2b. `scripts/build.sh`**：

- `:15` → `# 产物名必须与 pluginID 一致（freetierplus）。`
- `:34` → `"$GODOCKER" build -buildmode=c-shared -ldflags="$LDFLAGS" -o "dist/freetierplus.$ext" .`
- `:36` → `echo "Built dist/freetierplus.$ext"`

**2c. `scripts/godocker.sh:28`** → `  -v freetierplus-gocache:/gocache \`

**2d. `scripts/packstore/main.go:17`**（仅示例注释）→ `//	打包：  packstore -lib dist/freetierplus-linux-amd64.so -id freetierplus \`

**2e. `registry.json`**：

```json
      "id": "freetierplus",
      "name": "FreeTier Plus",
      "description": "（原文保持不变）",
      "author": "cxy2730",
      "repository": "https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin",
      "homepage": "https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin",
```

**`author` 保持 `cxy2730` 不动**——Spec §2 的映射表只要求改 `repository`/`homepage`，Spec §3.1 也说 `pluginAuthor` 可保持。它不含 `freetier` 字样，不触发防残留。`description` 同样不动。

`tests/integration/registry_test.go:67` 断言 `entry.ID == app.PluginID`，Task 1 已改，此处只改 JSON。

**2f. `README.md` / `README_EN.md`** —— 按下表逐处改（行号是当前快照）：

| 位置 | 改为 |
| --- | --- |
| `README.md:1` / `README_EN.md:1` | `# FreeTier Plus Plugin` |
| `README.md:13` / `README_EN.md:13` | 升级说明段落**整体重写为双开共存叙事**：说明本仓库是 `970196899` 维护的独立分支，插件 ID 为 `freetierplus`，可与原版 `freetier` 同实例共存；原版 ID `freetier` 只在此段以「原版」字样出现一次（供防残留豁免）。README_EN 用 `original upstream` 字样。 |
| `README.md:54` / `README_EN.md:54` | `FreeTier 2API` → `FreeTier Plus` |
| `README.md:65` / `README_EN.md:65` | YAML 配置节 `freetier:` → `freetierplus:` |
| `README.md:75,76` / `README_EN.md:75,76` | `git clone https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin.git` 与 `cd CLIProxyAPI-zhenghefreellm2api-plugin` |
| `README.md:80` / `README_EN.md:80` | `dist/freetierplus.<so\|dylib\|dll>` |
| `README.md:89` / `README_EN.md:89` | `/v0/resource/plugins/freetierplus/console` |
| `README.md:126` / `README_EN.md:126` | `plugins.configs.freetierplus` |
| `README.md:138` / `README_EN.md:138` | `~/.freetierplus-plugin`，并说明「若存在 `~/.cli-proxy-api` 则默认使用其下 `freetierplus` 子目录」 |

**2g. 新增「双开共存须知」小节**（插入 README 的「配置说明」之后、`README_EN` 对应位置）。逐条写清下面五条，不得省略：

1. **模型名不加前缀**：两个插件注册的模型 ID 完全相同（如 `workbuddycn-*`）。CPA 会把同名模型合并调度；若需精确指定，请用各插件控制台里的模型别名区分。
2. **凭证归属由宿主按 `type` 字段分发**：本插件写 `type=freetierplus`，原版写 `type=freetier`。两个插件的控制台都会列出文件名前缀属于本插件约定（`workbuddycn-*` 等）的凭证——这是为兜住「被前置插件抢走」的账号而保留的既有容错行为，不是缺陷。
3. **状态目录相互独立**：本插件用 `~/.freetierplus-plugin`（或 `~/.cli-proxy-api/freetierplus`），可用 `FREETIERPLUS_PLUGIN_HOME` 覆盖。
4. **WorkBuddy 设备指纹会变化**（风险 B）：新状态目录会生成新的 `machine_salt`，WorkBuddy 的 `X-Machine-ID` 随之改变。若需保留原指纹：在 `plugins.configs.freetierplus.machine_salt` 填入原值，或把旧状态目录的 `machine_salt` 文件复制到新状态目录。
5. **日志与列偏好互不覆盖**：日志文件名为 `freetierplus-<date>.log`（原版是 `freetier-<date>.log`）；控制台列偏好键为 `freetierplus-model-optional-columns`。

- [ ] **Step 3: 验证绿灯（CI 全量）**

```bash
git add -A && git commit -m "chore: CI/构建脚本/插件清单/README 同步 freetierplus"
git push
env -u SSL_CERT_DIR -u NODE_EXTRA_CA_CERTS gh run watch <run-id> --exit-status
```
预期：退出码 0；`gofmt`、`go vet`、`go test ./...`、`race tests`、`编译 live 探针`、`校验 c-shared 编译` 六步全绿；`TestNoResidualFreeTierIdentity` 零残留清单。
若六步中任一步红，`gh run view <run-id> --log-failed | tail -80` 定位；**若红在 `gofmt`**，按提示的文件与行修正空白后重推（不要把 `gofmt` 失败当成「本机无法验证」的借口跳过）。

- [ ] **Step 4: 构建产物验证（可选但推荐，需用户授权后执行）**

```bash
env -u SSL_CERT_DIR -u NODE_EXTRA_CA_CERTS gh workflow run ci.yml --ref feat/freetierplus-coexistence
env -u SSL_CERT_DIR -u NODE_EXTRA_CA_CERTS gh run list --workflow=ci.yml --limit 1
```
`workflow_dispatch` 会触发 `build` 作业（`ci.yml:51-53` 的 `if` 条件）。预期 `dist/freetierplus-<goos>-<goarch>.<ext>` 三平台产物构建成功。该步骤不发布 Release（只在 tag 上），因此不会污染仓库。

- [ ] **Step 5: 本地提交**

```bash
git add -A
git commit -m "chore: CI/构建脚本/插件清单/README 同步 freetierplus"
```

---

## Review Focus

以下五类是 Spec 未明说、但最可能让用户踩坑的输入/条件。每一条都已在对应 Task 中挂了测试。

1. **双开时两个插件都列出同一份 `workbuddycn-*.json`**（风险 A）——用户会以为「新版把原版账号吞了」。预期行为：列出是既有的容错设计，归属由宿主按 `type` 分发。测试：Task 7 `TestCoexistenceAuthListKeepsCurrentClaimBehavior`。
2. **原版 `freetier` 凭证被新版 Cline 判定为「别家」并拒绝认领**（风险 C）——用户可能以为这是误判。预期行为：双开下这是正确方向，两侧对称。测试：Task 3 新增的两条 cline 用例。
3. **WorkBuddy 设备指纹因新 `machine_salt` 改变**（风险 B）——用户会看到上游要求重新验证设备。预期行为：新部署新盐是语义正确；README 给出两种用户侧保留方式。测试：无新增（既有 `adapter_test.go:93-95` 已锁「盐变则指纹变」，本次不改盐逻辑）。
4. **`50-models.html` 的 `configPath` 从硬编码改为 `BASE + "/config"` 后，宿主插件配置路由是否仍可写**——若 `BASE` 在派生时作用域不可见，模型变更自动同步会静默失效（页面显示成功但 `/v1/models` 不更新）。预期行为：全页单 `<script>` 单 IIFE，`BASE` 可见。测试：Task 4 `TestConsoleModelChangesTriggerHostReload` 断言 `hostPath(BASE + "/config")` 与 `api("PATCH", configPath`。
5. **防残留守卫的误报/漏报**——若剔除逻辑过宽（例如把任何含 `2api-plugin` 的行都跳过），会漏掉真实残留；若过窄，会把 `go.mod` 与 import 路径判为残留导致 CI 永远红。预期行为：只剔除 `freetier2api-plugin` 这一精确子串，其余按 allowlist。测试：Task 1 `TestNoResidualFreeTierIdentity` 自身即守门；Task 1 Step 2 明确要求检查红灯文案里不出现模块路径。

---

## 施工顺序与依赖

```
Task 1 (身份常量 + 守卫)
  ├── Task 2 (状态目录)
  ├── Task 3 (供应商收口)
  ├── Task 4 (控制台页面)
  └── Task 5 (日志 / Qoder 缓存)
        └── Task 6 (集成测试语义更新)   ← 依赖 1-5 全部完成
              └── Task 7 (双开回归守卫)  ← 依赖 1,3
                    └── Task 8 (CI/脚本/清单/文档)  ← 依赖 1-7（防残留扫描收尾归零）
```

- Task 2/3/4/5 之间无文件交叉，可并行，但**必须串行提交**（每个 Task 一次 commit，便于 CI 定位红灯）。
- Task 6 **必须在 1–5 全绿后**执行，因为它是「全量测试对齐」而非驱动实现。
- Task 8 的 Step 1 是**前置检查**：若防残留清单里仍有 `internal/` 或 `tests/` 路径，说明前面漏了，必须回头补。

## 非目标（Out of Scope）

- 不开发自动扫描 / 改写现有 `auth-files/*.json` 的迁移脚本或 UI 按钮（Spec §4.2）。
- 不迁移 `machine_salt` 文件本身（只在 README 给用户侧操作指引）。
- 不修改 `go.mod` 的 `module freetier2api-plugin`。
- 不修改 `cpasdk/`。
- 不给模型 ID 加供应商前缀（Spec §1.2）。
- 不删减任何供应商（10 个供应商全部保留）。
- 不改动任何线上 CPA 配置或远程文件。
- 不为 `filterPluginAuths` 新增 provider 隔离（风险 A 裁定：保持现状）。
