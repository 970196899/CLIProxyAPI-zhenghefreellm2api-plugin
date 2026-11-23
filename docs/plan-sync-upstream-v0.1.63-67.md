# 施工方案 (Plan)：全量同步上游 v0.1.63 ~ v0.1.67 并发布 v0.1.67.1

- **目标仓库**：`D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main`
- **对应 Spec**：`docs/spec-sync-upstream-v0.1.63-67.md`
- **本地基线**：`c9a3c69`（`main`，`v0.1.64`）
- **上游目标**：`FETCH_HEAD`（`df2812d7e3c2f93d2acbf3c3b34cff8be2a7a788`，`v0.1.67`；共同祖先 `a639f23`）
- **合并树基线**：`git merge-tree --write-tree c9a3c69 FETCH_HEAD` 产出 tree `51196bbecee0765d0d5885d4ae34bc41c4917586`（38 个变更文件中 35 个自动合并成功，3 处显式冲突）
- **目标发布版本**：`v0.1.67.1`（注解 tag）

---

## 一、核心风险与只读实证发现摘要

### 1. 自动合并文件（35 个）核查结论
1. **`.github/workflows/ci.yml`**：
   - `test`、`build`、`release` 三个 job 的 `actions/setup-go@v5` 升级为 `go-version: '1.26'`；`build.strategy.matrix.include` 正确挂在 `matrix:` 下（含上游 `8a81518` 缩进修复），Linux runner 钉在 `ubuntu-24.04`。
   - 本地 `./internal/vendors/...` live 编译探针、`zhenghefreellm` 产物路径、`-id zhenghefreellm` 与 Release 升级提示正文均完整保留，满足 `TestPluginIDMatchesLibraryName` 正反向守卫。
2. **`README.md` 与 `README_EN.md`**：
   - `README.md` 自动合并后在第 15~17 行合入上游 `## 兼容性说明（x86-64）`（`26fd084`），`## 功能特性` 移至第 19 行，并完整保留独立维护声明、`zhenghefreellm` 安装/配置指引。
   - **缺口补齐**：上游只改了 `README.md` 未改 `README_EN.md`，需在 `README_EN.md` 第 13 行下方、`## Features` 上方补入对应英文 `## Compatibility (x86-64)` 章节。
3. **`internal/app/main.go`、`internal/app/app.go` 与 `cpasdk/`**：
   - `cpasdk/pluginabi/types.go` `SchemaVersion` 升至 `10`；`cpasdk/pluginapi/types.go` 保留本地两处扩展（`QuotaFetchResponse.Claim` / `ClaimStatus` 与 `ManagementRequest.Context` / `RequestContext()`）。
   - `internal/app/main.go` 完整保留 `pluginID = "zhenghefreellm"`、`pluginDisplayName = "Zhenghe FreeLLM 2API"`、`pluginAuthor = "970196899"`、`pluginRepository` 及 `providerKey = core.ProviderKey`；合入 `hostSchemaFrom` / `noteHostSchema` / `negotiatedSchemaVersion` 降级协商并移除 `Metadata.ConfigFields`。
   - 上游未触碰 `core.ProviderKey`（`"freetier"`）、`DefaultStateDirName`（`".freetier-plugin"`）、`StateDirEnvOverride`（`"FREETIER2API_PLUGIN_HOME"`）与 `/freetier-auth-url`。
4. **`internal/app/executor.go`**：
   - 上游仅更新 `handleExecutorCountTokens`；本地 `spliceModelValue`（`terminated` / `valueTerminated` 守卫）与 `hasAnotherModelKey` 完整保留。
5. **`internal/vendors/workbuddy/models.go` 与 `adapter.go`**：
   - `models.go` 吸收上游视频/音频过滤与 `11102` 模型 24h 隔离（`blockedModelTTL`、`markModelBlocked`、`filterBlockedModels`），本地 PR #1 的 `mergeModelCatalogs` 与 `displayCredits` 位于文件末尾，完全正交共存。
   - `adapter.go` 保留本地 `ConsoleModels()`（使用 `displayCredits`）与 `ModelCredits()`（使用 `core.FormatRate`，不泄漏 `"浮动倍率"` 哨兵到持久状态），合入 `noteModelBlocked` 与移除 `ConfigFields()`。
6. **前端翻译词典 `internal/admin/page/20-core.html` 与 `t()` 调用链路**：
   - `t(text)` 在默认中文模式下恒等返回中文原文，在英文模式下对 `translations` 做精确键匹配。
   - **语义修复 1**：`30-accounts.html` 冲突解决后调用 `t("没有账号。请在 CPA 面板的「添加认证」里选择 Zhenghe FreeLLM 2API，或在控制台页发起登录。")`，需将 `20-core.html` 词典对应键与英文值由 `FreeTier 2API` 同步为 `Zhenghe FreeLLM 2API`。
   - **语义修复 2（Spec 方案 A）**：`50-models.html` 已使用 `t(credits)` 与 `t("上游计费倍率：") + t(credits)` 渲染倍率。在 `20-core.html` `translations` 补入：
     - `"浮动倍率": "Floating"`
     - `"基础月度套餐": "Base monthly plan"`
     - `"活动与签到奖励": "Promotional & check-in rewards"`
     - `"资源包明细": "Resource packages"`
   - `50-models.html` 中 `var configPath = hostPath(BASE + "/config");` 完整保留。
7. **测试兼容性**：
   - 上游已适配 `console*_test.go` 的 `t()` 断言及 Node 治具；本地 `Page("/plugins/zhenghefreellm")` 与 `TestConsoleModelChangesTriggerHostReload` 合并后全部兼容。需在 `internal/admin/console_test.go` 新增 `TestConsoleTranslationsCoverLocalStrings` 锁死本地新增词条与 `Zhenghe FreeLLM 2API` 空账号提示翻译。

---

## 二、有序施工 Task 列表（Task 1 ~ Task 5）

### Task 1：创建同步分支并执行三路合并
1. 从 `c9a3c69` 创建并切换分支 `feat/sync-upstream-v0.1.63-67`：`git checkout -b feat/sync-upstream-v0.1.63-67 c9a3c69`。
2. 发起保留三路共同祖先的合并（暂不提交）：`git merge --no-commit FETCH_HEAD`。
3. 验证 `git diff --name-only --diff-filter=U` 仅包含以下 3 个冲突文件：
   - `internal/admin/page/10-body.html`
   - `internal/admin/page/30-accounts.html`
   - `internal/vendors/workbuddy/models_test.go`

### Task 2：解决 3 处显式文本冲突
1. **`internal/admin/page/10-body.html`**（第 2~13 行 `<header>` 块，冲突标记在 3/5/11 行）：保留本地 `<h1>Zhenghe FreeLLM 2API</h1>`，合入上游 `<select id="language" ...>` 下拉框，去除冲突标记。
2. **`internal/admin/page/30-accounts.html`**（第 214 行 `renderAccounts`）：采纳上游 `t(...)` 包裹并将展示名保留为 `Zhenghe FreeLLM 2API`：
   `host.appendChild(el("div", t("没有账号。请在 CPA 面板的「添加认证」里选择 Zhenghe FreeLLM 2API，或在控制台页发起登录。"), "muted"));`
3. **`internal/vendors/workbuddy/models_test.go`**（`add/add` 冲突，冲突标记在 13/126/184 行，**185-186 行的 `\t}` `}` 是两侧共用的收尾大括号**）：合并双方 `import`（`"testing"`、`"time"`、`"freetier2api-plugin/internal/core"`），在本地 7 个测试函数末尾补 `\t}` 与 `}` 闭合最后函数后，续接上游 2 个测试函数（`TestNonChatModelFiltersVideoFamilies`、`TestFilterBlockedModels`，去掉重复 `import` 及文件头注释），保留原 185-186 行公共 `\t}` `}`，最终合计 9 个 `Test*` 函数且花括号净差为 0。

### Task 3：补全多语言词典（`20-core.html`）与英文兼容性说明（`README_EN.md`）
1. **`internal/admin/page/20-core.html`**（`var translations = { ... }`）：
   - 更新空账号提示词典键与值为 `Zhenghe FreeLLM 2API`；
   - 补入 `"浮动倍率": "Floating"`（供 `50-models.html` 的 `t(credits)` 消费），并按 Spec 方案 A 预留补入 `"基础月度套餐": "Base monthly plan"`、`"活动与签到奖励": "Promotional & check-in rewards"`、`"资源包明细": "Resource packages"`（注释注明后者为主机侧/未来面板渲染预留）。
2. **`README_EN.md`**：在第 13 行下方、`## Features` 上方补入英文版 `## Compatibility (x86-64)`，说明 `v0.1.60–v0.1.65` 在无 AVX 旧 CPU 上的 `SIGILL` 根因与本版 `ubuntu-24.04` 钉死修复。

### Task 4：先写词典与语言控件守卫单测并执行本地静态/边界核查
1. **`internal/admin/console_test.go`**：新增 `TestConsoleTranslationsCoverLocalStrings(t *testing.T)`，校验 `Page("/plugins/zhenghefreellm")` 包含：
   - `<select id="language"` 与 `<h1>Zhenghe FreeLLM 2API</h1>` 同页共存（防止 `10-body.html` 冲突误删语言控件导致 `20-core.html` 顶层 IIFE 抛 `TypeError` 瘫痪）；
   - 新的 `Zhenghe FreeLLM 2API` 空账号词典键值及 `浮动倍率`、`基础月度套餐`、`活动与签到奖励` 映射；
   - 不再包含 `FreeTier 2API` 空账号提示词典键。
   （先写测试验证红灯，再对照 Task 3 验证，注意 tab 缩进符合 `gofmt`）。
2. 本地只读核查（本机无 Go 工具链，本地做静态与语法校验，完整 Go 编译/测试在 CI 完成）：
   - `git diff --check` 零冲突标记与尾随空白；
   - `node --check` 校验拼接后的前端 `<script>` 片段 JS 语法（若本机 `node` 可用）；
   - 核查禁改清单：`internal/core/vendor.go` (`ProviderKey == "freetier"`)、`internal/admin/config.go` (`DefaultStateDirName == ".freetier-plugin"`, `StateDirEnvOverride == "FREETIER2API_PLUGIN_HOME"`, `filepath.Join(cpaDir, "freetier")`)、`internal/admin/page/70-login.html` (`/freetier-auth-url`)、`internal/admin/page/50-models.html` (`hostPath(BASE + "/config")` 与 `freetier-model-optional-columns`)、`registry.json` 与 `.github/workflows/ci.yml` / `scripts/build.sh` 产物守卫。

### Task 5：提交合并、CI 6 步验证、终审、合入 `main` 与发布 `v0.1.67.1`
1. 纳入 `docs/spec-sync-upstream-v0.1.63-67.md` 与 `docs/plan-sync-upstream-v0.1.63-67.md` 及全部已解决修改，提交合并 commit 并推送 `feat/sync-upstream-v0.1.63-67` 至 `origin`。
2. 创建 PR 至 `main`，等待 GitHub Actions CI 6 步全绿（`gofmt` / `go vet` / `go test ./...` / `-race` / 编译 live 探针 / `c-shared`）。
3. 拿到 CI 全绿实证后，按法定程序派发 `reviewer` 子代理对 `git diff c9a3c69..HEAD` 执行只读终审（同步核查 `tests/` 变更无放水）；全票 PASS 后合并 PR 至 `main`。
4. 本地切回并同步 `main`，在最新 `main` 提交上创建附注 tag `v0.1.67.1` 并推送至 `origin`。
5. 等待 `v0.1.67.1` tag 触发的 CI（`test` → `build` 3 平台 → `release`）全绿，下载 Release 资产核验三平台归档（`zhenghefreellm_0.1.67.1_linux_amd64.zip`、`zhenghefreellm_0.1.67.1_linux_arm64.zip`、`zhenghefreellm_0.1.67.1_windows_amd64.zip`）与 `checksums.txt` SHA256 一致、根目录下动态库名正确。
6. 清理临时文件与后台子代理，确认工作区 clean，向「需求」窗口回传结单结果。

---

## 三、禁改项核对清单

| 核对项 | 期望值 / 状态 | 所在文件 |
| --- | --- | --- |
| `core.ProviderKey` | `"freetier"`（严禁修改） | `internal/core/vendor.go:31` |
| `DefaultStateDirName` / `StateDirEnvOverride` | `".freetier-plugin"` / `"FREETIER2API_PLUGIN_HOME"` / `filepath.Join(cpaDir, "freetier")` | `internal/admin/config.go:56-58,110` |
| 宿主 OAuth 登录路由 | `hostPath("/freetier-auth-url?vendor=" + ...)` | `internal/admin/page/70-login.html:324` |
| 宿主插件配置重载路由 | `hostPath(BASE + "/config")` | `internal/admin/page/50-models.html` |
| 插件对外身份 `pluginID` / 显示名 | `"zhenghefreellm"` / `"Zhenghe FreeLLM 2API"` | `internal/app/main.go`、`registry.json`、`00-head.html`、`10-body.html` |
| 本地 WorkBuddy 余额/倍率规则 | `packageRemainUsed`、`quotaGroupDisplayName`、`mergeModelCatalogs`、`displayCredits`、`ModelCredits` 完整保留 | `internal/vendors/workbuddy/tasks.go`、`models.go`、`adapter.go` |
| 线上与全局环境 | 严禁改动 `192.168.1.200` 线上配置或 `~/.claude/settings.json` | 外部环境 |
