# 施工方案 (Plan)：全量同步上游 v0.1.68 ~ v0.1.75 并发布 v0.1.75.1

- **目标仓库**：`D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main`
- **对应 Spec**：`docs/spec-sync-upstream-v0.1.68-75.md`
- **本地基线**：`f6158013775c98819897042203a607df9251b99d`（`main`，`v0.1.67.1`）
- **上游目标**：`https://github.com/cxy2730/CLIProxyAPI-freetier2api-plugin.git` tag `v0.1.75`（`ce26d57a76a91d4897bf67527a194a050fdbe904`；共同祖先 `df2812d7e3c2f93d2acbf3c3b34cff8be2a7a788`，即 `v0.1.67`，共 30 个上游提交）
- **目标特性分支**：`feat/sync-upstream-v0.1.68-75`（需同步纳入 `docs/spec-sync-upstream-v0.1.68-75.md` 与 `docs/plan-sync-upstream-v0.1.68-75.md`）
- **合并树只读预演基线**：`git merge-tree --write-tree --name-only --messages f615801 ce26d57` 产出 tree `72c8774c8c8ac75b8b3f709b44313c72ba5dbad7`（共 92 个变更文件：90 个自动合并/增删，仅 2 处显式文本冲突）
- **目标发布版本**：`v0.1.75.1`（附注 tag，归档命名 `zhenghefreellm_0.1.75.1_<goos>_<goarch>.zip`）
- **环境事实**：本机 Windows 环境已安装 `node`（v24.22.1）与 `gh`（但需按 Task 1 前置核查 `gh` 登录态与 `SSL_CERT_DIR` 证书污染并备好 REST API 兜底），未安装 `go` 与 `docker`；本地验证由静态/Node/JSON/grep 校验完成，Go 编译、格式化与全量测试由 GitHub Actions CI 6 步门禁权威执行（若在含 Go 工具链的环境执行，各 Task 同步给出精确 `go` 命令）。

---

## 一、只读预演实证与二次核查结论

### 1. 显式冲突文件（2 个）
1. **`registry.json`**（合并树第 5~19 行）：
   - 冲突原因：本地保留 `zhenghefreellm` 独立身份（ID/名称/作者/仓库/主页），上游 `4493009` 从 `description` 移除了 `OpenCode ZEN、`（`tags` 数组中 `"OpenCode"` 已由三路合并自动移除）。
   - 解法：保留本地 `id` / `name` / `author` / `repository` / `homepage`，采纳上游移除 `OpenCode ZEN、` 后的 `description`，保持 `tags` 中不含 `"OpenCode"`。
2. **`internal/admin/console_test.go`**（合并树第 957~1009 行）：
   - 冲突原因：本地在文件末尾新增 `TestConsoleTranslationsCoverLocalStrings`，上游 `ce26d57` 在同一位置新增 `TestConsoleMobileFeedbackVisible`。
   - **共享大括号陷阱**：合并树第 1009 行的 `}` 被 Git 合并为两函数共用的收尾大括号，第 985 行仅闭合 `if strings.Contains(page, stale)`。解决冲突时必须在 `TestConsoleTranslationsCoverLocalStrings` 末尾显式补上函数收尾 `}` 与空行，再接 `TestConsoleMobileFeedbackVisible`，并将其第 994 行 `Page("/plugins/freetier")` 改为 `Page("/plugins/zhenghefreellm")`。

### 2. 自动合并文件（90 个）关键核查结论
1. **前端页面（`internal/admin/page/*.html`）**：
   - `00-head.html`：保留第 6 行 `<title>Zhenghe FreeLLM 2API</title>`，自动合入吸顶提示横幅、窄屏居中浮层（`.banner.show { position: fixed; ... }`）、工具栏分簇（`.toolbar` / `.tgroup`）、渠道页签横滑（`.vendor-tabs`）与 `.scroll-hint` 样式。
   - `10-body.html`：保留第 3~7 行 `<h1>Zhenghe FreeLLM 2API</h1>` 与 `<select id="language">` 下拉框，合入 `#vendorTabs`、工具栏 `.toolbar`/`.tgroup`、`placeholder="正在获取授权链接…"`，以及 `#accountsScrollHint`（第 123 行）与 `#modelsScrollHint`（第 153 行）两处 `← 左右滑动查看完整表格 →`。
   - `30-accounts.html`：第 266 行空账号提示完整保留 `Zhenghe FreeLLM 2API`；合入 `renderVendorTabs`（`t("全部")`、`t(" 个账号")`）、`activeVendor` 空筛选提示（`t("该渠道下没有匹配的账号。")`）、`.pending-tag` 类名、紧凑刷新时间格式与 `syncScrollHints()`。
   - `50-models.html`：完整保留 `MODEL_COLUMNS_STORE_KEY = "freetier-model-optional-columns"` 与 `var configPath = hostPath(BASE + "/config");`；仅在第 188 行末尾新增 `syncScrollHints()`。
   - `60-views.html` / `70-login.html` / `80-columns.html`：完整保留 `70-login.html` 中的 `hostPath("/freetier-auth-url?vendor=" + ...)`；合入 `revealPanel(...)`、Esc 关闭浮层、`btnLoadQuotas` 防连点与 `isBenignSkip` 逻辑。
   - **全量 JS 语法与多语言缺口审计（Node 实测 `72c8774`）**：
     - 拼接全部 9 份 `internal/admin/page/*.html` 的 `<script>` 块（125,088 字节）执行 `node --check` 通过。
     - 扫描全部 `t(...)` 调用及 `10-body.html` 所有中文文本节点（`>...<`）与 `placeholder`/`title`/`aria-label` 属性，与 `20-core.html` 现有 308 条 `translations` 比对：`"正在获取授权链接…"` 与 `" 个账号"` 均已存在词典中，**新增未翻译中文界面文案严格且仅有 3 条（2 条 `t()` 词典键 + 1 条 HTML 文本节点）**：
       1. `t()` 词典键：`"全部"`（`30-accounts.html:220`）
       2. `t()` 词典键：`"该渠道下没有匹配的账号。"`（`30-accounts.html:272`）
       3. HTML 文本节点：`"← 左右滑动查看完整表格 →"`（`10-body.html:123,153`，由 `20-core.html:39-44` 的 `TreeWalker` 自动翻译）
2. **文档（`README.md` / `README_EN.md`）**：
   - 两份文档均无冲突自动合并完成：保留独立维护声明、替换安装升级指引、`plugins.configs.zhenghefreellm` 与 `## 兼容性说明（x86-64）` / `## Compatibility (x86-64)`；同步将供应商实例数由 `15` 更新为 `14`、删除 `opencodezen` 行、合入「一键任务与限流」/「Batch Tasks & Rate Limits」章节与控制台渠道分页说明。无需额外手工修改。
3. **本地 WorkBuddy（PR #1）与 `executor.go`**：
   - `internal/vendors/workbuddy/`：PR #1 的 `packageRemainUsed`（`tasks.go:2857`）、`quotaGroupDisplayName`（`tasks.go:3229`）、`mergeModelCatalogs`（`models.go:788`）、`displayCredits`（`models.go:812`）、`ConsoleModels`（`adapter.go:590`）、`ModelCredits`（`adapter.go:615`）及 `models_test.go` / `tasks_test.go` 全部完好。上游仅改动 `adapter.go`（`creditCooldown` 跨模型冷却）、`chat.go`（`creditStreamError`）、`errors.go`（`14018` 流内积分耗尽识别）、`tasks.go`（`RunAllTasks` 429 跳过剩余任务）及新增 `credit_cooldown_test.go` / `probe_live_test.go`，代码区完全错开，逻辑互补无冲突。
   - `internal/app/executor.go`：未在上游变更集内，`spliceModelValue`（第 152 行）与 `hasAnotherModelKey`（第 261 行）完好无损。
4. **身份与全局残留排查（全树 grep 实测 `72c8774`）**：
   - `/plugins/freetier`：全树仅存 `console_test.go:580`（反向硬编码守卫测试）、冲突区 `console_test.go:994`（Task 2 修正）及历史 `docs/spec-rename-zhenghefreellm.md`；上游新增业务源码与集成测试零误用。
   - `FreeTier 2API`：全树仅存冲突区 `registry.json:14`（Task 2 修正）、`console_test.go:963,982,984`（反向守卫测试）、`tests/integration/auth_test.go:131`（历史背景注释）及历史 `docs/`；零业务源码与前端残留。
   - `opencode`：`internal/vendors/opencodezen/` 已全量删除、`internal/app/vendors.go` 与 `tests/architecture_test.go` 已移除注册；全树仅余冲突区 `registry.json:8`（Task 2 修正）与 `internal/vendors/cline/credential.go:157`（外来凭证文件名黑名单，用于防止 Cline 误认老用户磁盘遗留的 `opencodezen-*.json`，**刻意保留不得删改**）。
   - 备注：`tests/integration/vendor_compat_test.go:14-38` 的注释与 `want` 切片同为 15 条、完全自洽（上游移除的是第 16 条 `opencodezen`）；`README.md` / `README_EN.md` 正文写 `14` 为上游既有口径，均保持上游原样以减少无谓 diff。
5. **工程护栏与禁改项**：
   - `.github/workflows/ci.yml` 与 `scripts/build.sh` 未在上游变更集内：Go `1.26`、Linux 钉死 `ubuntu-24.04`、`zhenghefreellm` 产物路径与 `-id zhenghefreellm`、Release 升级说明、以及 `./tests/live` 与 `./internal/vendors/...` live 探针编译门禁（将自动覆盖上游新增的 `trae/version_live_test.go`、`workbuddy/probe_live_test.go`、`zcode/version_live_test.go`、`tests/live/audit_live_test.go`）全部保持完好。
   - `internal/admin/authfile.go` 新增 `<state_dir>/auths` 优先兜底仅读取 `s.deps.Config().StateDir`，未触碰 `DefaultStateDirName`（`".freetier-plugin"`）、`StateDirEnvOverride`（`"FREETIER2API_PLUGIN_HOME"`）或 `filepath.Join(cpaDir, "freetier")`。
   - `internal/core/vendor.go:31`（`ProviderKey = "freetier"`）与 `internal/app/main.go:20-26`（`pluginID = "zhenghefreellm"`）未受触动。

---

## 二、有序施工 Task 列表（Task 1 ~ Task 5）

### Task 1：创建同步分支、拉取上游 ref 并执行三路合并

1. **前置核查（红灯/基线确认）**：
   - 确认当前处于 `main`（`f615801`），且工作区仅含未跟踪的 `docs/spec-sync-upstream-v0.1.68-75.md` 与本方案文档 `docs/plan-sync-upstream-v0.1.68-75.md`。
   - **GitHub CLI 与 TLS 可用性前置探测**（避免 Task 5 卡死）：先检查 `$env:SSL_CERT_DIR`；若其指向 Miniforge 等非系统证书目录（导致 `x509: certificate signed by unknown authority`），在当前会话清空 `$env:SSL_CERT_DIR = ""` 后执行 `gh auth status` 与只读探针 `gh api rate_limit`。若 `gh` 仍因 keyring token 无效或网络策略不可用，记录为「启用兜底通道」：Task 5 改为经 `$env:GH_TOKEN` 或 `git credential fill` 获取凭证后通过 GitHub REST API（`curl`）执行 PR/CI/Release 读写，或在无可用 API token 时退化为 `git push` 后由人工在 GitHub 网页操作并明确上报阻塞。
2. **执行操作（最小实现）**：
   - 从 `f615801` 创建并切换特性分支：`git checkout -b feat/sync-upstream-v0.1.68-75 f615801`。
   - 只读拉取上游 `v0.1.75` 到临时 ref（不污染本地 tags）：
     `git fetch --no-tags https://github.com/cxy2730/CLIProxyAPI-freetier2api-plugin.git refs/tags/v0.1.75:refs/remotes/upstream-preview/v0.1.75`
   - 校验附注 tag 与目标 commit SHA（上游 `v0.1.75` 为附注 tag，未解引用时 `git rev-parse` 返回 tag 对象 `94c4a8e76ac36514e8011074ff1a8c77fd88d956` 而非 commit）：
     - `git cat-file -t refs/remotes/upstream-preview/v0.1.75` 输出 `tag`（旁证）；
     - `git rev-parse 'refs/remotes/upstream-preview/v0.1.75^{commit}'`（或 `git rev-list -n 1 refs/remotes/upstream-preview/v0.1.75`）必须严格等于 `ce26d57a76a91d4897bf67527a194a050fdbe904`；
     - `git merge-base HEAD 'refs/remotes/upstream-preview/v0.1.75^{commit}'` 必须等于 `df2812d7e3c2f93d2acbf3c3b34cff8be2a7a788`。
   - 执行保留三路共同祖先的合并（暂不提交）：`git merge --no-commit 'refs/remotes/upstream-preview/v0.1.75^{commit}'`。
3. **验证合并现场（绿灯校验）**：
   - `git diff --name-only --diff-filter=U` 输出必须**严格只有 2 行**：
     - `internal/admin/console_test.go`
     - `registry.json`
   - `Test-Path internal/vendors/opencodezen` 必须为 `False`。

---

### Task 2：解决 2 处显式文本冲突并先落地多语言扩展守卫单测（TDD 红灯）

1. **写失败测试与解 `internal/admin/console_test.go` 冲突（第 957~1009 行）**：
   - 去除冲突标记，保留 `TestConsoleTranslationsCoverLocalStrings(t *testing.T)`，并在其 `want` 切片中**提前追加**上游新增 3 条中文文案的英文映射断言：
     - `` `"← 左右滑动查看完整表格 →": "← Scroll horizontally to view full table →"` ``
     - `` `"全部": "All"` ``
     - `` `"该渠道下没有匹配的账号。": "No matching accounts under this provider."` ``
   - 在 `TestConsoleTranslationsCoverLocalStrings` 的 `if strings.Contains(page, stale) { ... }` 块之后**显式补齐函数闭合 `}`** 与空行。
   - 续接 `TestConsoleMobileFeedbackVisible(t *testing.T)`（保留原第 1009 行收尾 `}`），仅将第 994 行：
     - 旧：`page := Page("/plugins/freetier")`
     - 新：`page := Page("/plugins/zhenghefreellm")`
   - 保持全部 6 条移动端反馈字面量断言不变，确保缩进为 Tab（符合 `gofmt`）。
2. **验证红灯**：
   - 本地无 Go 环境时，执行 Node 词典审计脚本（**必须同时扫描两类通道**：全部 `internal/admin/page/*.html` 中的 `t(...)` 调用，以及 `10-body.html` 中的 `>...<` HTML 中文文本节点与 `placeholder`/`title` 属性），对照 `20-core.html` `translations` 校验覆盖率：预期明确报出全部 3 处缺失——2 条 `t()` 词典键（`"全部"`、`"该渠道下没有匹配的账号。"`）+ 1 条 HTML 文本节点（`"← 左右滑动查看完整表格 →"`），退出码非 0。
   - （有 Go 环境时执行 `go test ./internal/admin -run TestConsoleTranslationsCoverLocalStrings -count=1`，预期 FAIL 并报 `console page missing expected translation/header entry: "← 左右滑动查看完整表格 →": ...`。）
3. **解 `registry.json` 冲突（第 5~19 行）**：
   - 去除冲突标记，精确落定以下字段（保留 `zhenghefreellm` 独立身份，移除 `OpenCode ZEN` 描述与 `"OpenCode"` 标签）：
     - `"id": "zhenghefreellm"`
     - `"name": "Zhenghe FreeLLM 2API"`
     - `"description": "把多个免费额度供应商（WorkBuddy 国内版/国际版、Qoder 国内版/国际版、Cline、ZCode 国内版/国际版、Trae 国内版/国际版、CodeArts、MiniMax Code 国内版/国际版、AutoClaw 国内版/国际版、小浣熊）接入 CLIProxyAPI：auth / model / executor / quota / management 五类能力，按供应商插桩，支持凭证归属隔离、模型清单动态探测、额度查询与每日签到。"`
     - `"author": "970196899"`
     - `"repository": "https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin"`
     - `"homepage": "https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin"`
     - `"tags"`：保持自动合并结果（15 项，已不含 `"OpenCode"`）。
4. **验证冲突清零**：
   - `git diff --check` 无输出且退出码 0；
   - `node -e "const r=JSON.parse(require('fs').readFileSync('registry.json','utf8')); const p=r.plugins[0]; if(r.schema_version!==1||p.id!=='zhenghefreellm'||p.name!=='Zhenghe FreeLLM 2API'||p.author!=='970196899'||/opencode/i.test(JSON.stringify(p))) process.exit(1); console.log('registry.json OK');"` 输出 `registry.json OK`。

---

### Task 3：补全 `internal/admin/page/20-core.html` 英文词典映射（TDD 绿灯）

1. **红灯前置**：已由 Task 2 步骤 2 验证缺失 3 条词条。
2. **最小实现（`internal/admin/page/20-core.html` 第 24 行下方）**：
   - 在现有本地预留词条行（`"基础月度套餐": "Base monthly plan", "活动与签到奖励": "Promotional & check-in rewards", "资源包明细": "Resource packages",`）正下方新增 2 行（与上游词典行物理正交，避免未来合并冲突）：
     ```js
         // 补全上游 v0.1.68-v0.1.75 新增的中文界面文案（表格横滑提示、渠道页签与空筛选提示）。
         "← 左右滑动查看完整表格 →": "← Scroll horizontally to view full table →", "全部": "All", "该渠道下没有匹配的账号。": "No matching accounts under this provider.",
     ```
   - 核查前端其余页面无需额外改动：`00-head.html:6`（`<title>Zhenghe FreeLLM 2API</title>`）、`10-body.html:3-7`（`<h1>Zhenghe FreeLLM 2API</h1>` + `<select id="language">`）、`30-accounts.html:266`（`Zhenghe FreeLLM 2API` 空账号提示）、`50-models.html`（`hostPath(BASE + "/config")`）均已自动合并正确。
3. **验证绿灯**：
   - 重新拼接 `internal/admin/page/*.html` `<script>` 块并运行 `node --check`：退出码 0。
   - 重新运行同一份双通道 Node 词典全量审计脚本（同时覆盖 `t(...)` 调用与 `>...<` HTML 文本节点两条通道）：
     - `t(...)` 调用通道缺失数为 `0`（原缺 2 条：`"全部"`、`"该渠道下没有匹配的账号。"` 已全部命中）；
     - `10-body.html` `>...<` 文本节点与 `placeholder`/`title` 属性通道中，除语言选择器自身 `"中文"` 与 `aria-label="Language / 语言"` 外缺失数为 `0`（原缺 1 条：`"← 左右滑动查看完整表格 →"` 已命中）；
     - `TestConsoleTranslationsCoverLocalStrings` 全部 10 条 `want` 字面量与 `TestConsoleMobileFeedbackVisible` 全部 6 条字面量在渲染后的 `Page("/plugins/zhenghefreellm")` 中命中率 100%，且不包含过时的 `FreeTier 2API` 空账号键。
   - （有 Go 环境时运行 `go test ./internal/admin -run "TestConsoleTranslationsCoverLocalStrings|TestConsoleMobileFeedbackVisible" -count=1`，预期 PASS。）

---

### Task 4：本地 `tester` 静态、边界与回归全面核验

在提交前由 `tester` 执行完整本地验证（PowerShell 5.1 逐项执行，不使用 `&&`）：

1. **冲突标记与格式检查**：
   - `git diff --check`：零输出、退出码 0。
   - 核对 `internal/admin/console_test.go` 新增/修改行全用 Tab 缩进、无多余空行或尾随空格，确保通过 CI `gofmt -l . | grep -v '^cpasdk/'`。
2. **前端 JS 语法与多语言守卫核验**：
   - 使用 `node` 拼接 `internal/admin/page/*.html`（按文件名排序）校验 `node --check` 通过，确认 `Page("/plugins/zhenghefreellm")` 同时包含 `TestConsoleTranslationsCoverLocalStrings` 与 `TestConsoleMobileFeedbackVisible` 全部断言且零缺失翻译。
3. **身份一致性与禁改项硬核验**：
   - 全局 grep 核查（排除 `docs/` 历史规格文档与反向守卫测试行）：
     - `git grep -n "/plugins/freetier"`：仅允许命中 `internal/admin/console_test.go:580`（反向硬编码守卫）与 `docs/`。
     - `git grep -n "FreeTier 2API"`：仅允许命中 `internal/admin/console_test.go` 反向守卫、`tests/integration/auth_test.go:131` 历史注释与 `docs/`。
     - `git grep -in "opencode"`：仅允许命中 `internal/vendors/cline/credential.go:157`（外来凭证排除黑名单）与 `docs/`。
   - 逐项核对下表「三、禁改项核对清单」全部 8 项边界无任何偏移。
4. **Go 单测命令清单（CI 6 步必跑；若在含 Go 工具链的容器/主机运行则同步本地执行）**：
   - 格式化：`gofmt -l . | grep -v '^cpasdk/'`（须为空）
   - 静态分析：`go vet ./...`
   - 全量测试：`go test ./... -count=1 -timeout 300s`
   - 关键定向回归清单：
     - `go test ./tests/integration -run "TestRegistryManifestIsValid|TestPluginIDMatchesLibraryName|TestVendorRegistryCompatibility|TestVendorCapabilities" -count=1`
     - `go test ./internal/admin -run "TestConsoleTranslationsCoverLocalStrings|TestConsoleMobileFeedbackVisible|TestConsoleModelChangesTriggerHostReload|TestConsolePageShowsPendingGrant|TestHandleLoginCallbackReportsPersistFailure" -count=1`
     - `go test ./internal/vendors/workbuddy -count=1`（覆盖 PR #1 余额/倍率全套测试 + 上游 `credit_cooldown_test.go`）
     - `go test ./internal/core ./internal/tasks -run "RateLimit" -count=1`
   - 竞态检测：`go test -race ./internal/core ./internal/state ./internal/auth ./internal/tasks ./internal/admin -count=1`
   - Live 探针编译校验：`go test -tags live ./tests/live -run '^$' -count=1` 与 `go test -tags live ./internal/vendors/... -run '^$' -count=1`
   - 动态库编译校验：`go build -buildmode=c-shared -o /dev/null .`

---

### Task 5：提交 PR、GitHub Actions 6 步 CI、终审、合入 `main` 与发布 `v0.1.75.1`

1. **暂存、提交并推送特性分支**：
   - 将全部合并结果及 `docs/spec-sync-upstream-v0.1.68-75.md`、`docs/plan-sync-upstream-v0.1.68-75.md` 一并 `git add`，使用合并 commit 信息提交（保留双亲父提交 `f615801` 与 `ce26d57`）。
   - 验证 `git rev-list --parents -n 1 HEAD` 包含 2 个父提交（确认是保留共同祖先的三路合并，而非 squash）。
   - 推送分支：`git push -u origin feat/sync-upstream-v0.1.68-75`。
2. **创建 PR 并等待 CI 6 步全绿**（所有 `gh` 命令均承接 Task 1 的 TLS/鉴权检查结果：若存在 `SSL_CERT_DIR` 污染先清空 `$env:SSL_CERT_DIR = ""`；若 `gh` 不可用则按 Task 1 兜底方案改用 `GH_TOKEN` / `git credential fill` 调 GitHub REST API `curl`，或退化为 `git push` + 网页操作并上报阻塞）：
   - 使用 `gh pr create --base main --head feat/sync-upstream-v0.1.68-75` 创建 PR（正文概括上游 v0.1.68~v0.1.75 核心变更、2 处冲突解法、3 条多语言补齐与保留本地身份/PR #1 声明）。
   - 使用 `gh pr checks --watch` 或 `gh run watch` 等待 `CI / 测试（gofmt / vet / go test）` 全部 6 步（`gofmt` / `go vet` / `go test` / `race tests` / `编译 live 探针` / `校验 c-shared 编译`）全部绿灯退出。
   - 若 CI 任一步失败，直接在 `feat/sync-upstream-v0.1.68-75` 修复并重跑直至全绿，绝不跳过门禁。
3. **`reviewer` 只读终审**：
   - 在 PR CI 全绿后，派发 `reviewer` 对 `git diff f615801..HEAD` 执行只读终审（重点审计冲突解决完整性、前端多语言与移动端反馈、WorkBuddy PR #1 无损、测试无放水、身份与禁改清单合规）。
   - 获全票 PASS 后，使用 `gh pr merge --merge` 显式以 Merge Commit 方式将 PR 合入 `main`（严禁使用默认或 `--squash` 策略，防止改写三路合并双亲历史而丢失与上游 `ce26d57` 的共同祖先链路，确保后续同步 `v0.1.76+` 仍可三路合并；REST API 兜底时显式传 `"merge_method": "merge"`）。
4. **打附注 tag `v0.1.75.1` 并触发 Release**：
   - 本地切回 `main` 并快进同步：`git checkout main`；`git pull --ff-only origin main`。
   - 在最新 `main` 提交上创建附注 tag（`git tag -a v0.1.75.1 -m "..."`，正文列出本版同步 v0.1.68~v0.1.75 要点、移除 OpenCode ZEN、补齐控制台英文翻译及保留 `zhenghefreellm` 独立身份/PR #1）。
   - 校验 tag 类型：`git cat-file -t refs/tags/v0.1.75.1` 必须输出 `tag`（非 `commit`）。
   - 推送 tag：`git push origin v0.1.75.1`。
5. **核验 Release 产物与清理现场**：
   - 等待 `v0.1.75.1` 触发的 CI（`test` → `build` 3 平台 → `release`）全绿。
   - 使用 `gh release view v0.1.75.1`（或 REST API 兜底）与下载校验确认以下 4 个产物齐全：
     - `zhenghefreellm_0.1.75.1_linux_amd64.zip`（根目录仅含 `zhenghefreellm.so`）
     - `zhenghefreellm_0.1.75.1_linux_arm64.zip`（根目录仅含 `zhenghefreellm.so`）
     - `zhenghefreellm_0.1.75.1_windows_amd64.zip`（根目录仅含 `zhenghefreellm.dll`）
     - `checksums.txt`（三份 zip 的 SHA256 与实际下载文件逐一匹配）
   - 清理临时 ref、悬空对象与临时脚本：
     - `git update-ref -d refs/remotes/upstream-preview/v0.1.75`
     - 按需执行 `git gc --prune=now` 回收临时 ref 删除后产生的悬空对象；
     - 确认 `git status --short --branch` 干净（`main...origin/main`，无未跟踪残留）。

---

## 三、禁改项与边界核对清单

| 核对项 | 期望值 / 状态 | 实证文件与行号 |
| --- | --- | --- |
| `core.ProviderKey` | `"freetier"`（严禁修改） | `internal/core/vendor.go:31` |
| 状态目录常量与宿主默认目录 | `DefaultStateDirName = ".freetier-plugin"`、`StateDirEnvOverride = "FREETIER2API_PLUGIN_HOME"`、`filepath.Join(cpaDir, "freetier")` | `internal/admin/config.go:56-58,110` |
| 宿主 OAuth 登录路由 | `hostPath("/freetier-auth-url?vendor=" + ...)` | `internal/admin/page/70-login.html:327` |
| 宿主插件配置重载路由 | `var configPath = hostPath(BASE + "/config");` 与 `freetier-model-optional-columns` | `internal/admin/page/50-models.html` |
| 插件对外身份与仓库地址 | `pluginID = "zhenghefreellm"`、`Zhenghe FreeLLM 2API`、`970196899`、`CLIProxyAPI-zhenghefreellm2api-plugin` | `internal/app/main.go:20-26`、`registry.json`、`00-head.html:6`、`10-body.html:3`、`30-accounts.html:266` |
| PR #1 WorkBuddy 规则与 `executor.go` | `packageRemainUsed`、`quotaGroupDisplayName`、`mergeModelCatalogs`、`displayCredits`、`ConsoleModels`、`ModelCredits`、`spliceModelValue` 完整保留 | `internal/vendors/workbuddy/{tasks,models,adapter}.go`、`internal/app/executor.go:152,261` |
| 构建与 CI 产物守卫 | Go `1.26`、Linux 钉 `ubuntu-24.04`、`dist/zhenghefreellm-*`、`-id zhenghefreellm`、`./internal/vendors/...` live 编译探针 | `.github/workflows/ci.yml`、`scripts/build.sh` |
| 外部环境禁区 | 严禁触碰 `192.168.1.200` 线上配置、本机 `plugins\windows\freetier-v0.1.59.dll` 与 `~/.claude/settings.json` | 外部环境 |
