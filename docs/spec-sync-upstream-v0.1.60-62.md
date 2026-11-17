# 设计规格说明书 (Spec)：全量同步上游 v0.1.60 ~ v0.1.62

- **创建日期**：2026-11-17
- **项目路径**：`D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main`
- **基线提交**：`e810ec3`（`main`，已打 tag `v0.1.60`，对应 PR #1 WorkBuddy 配额对齐与模型倍率纠偏）
- **上游目标**：`https://github.com/cxy2730/CLIProxyAPI-freetier2api-plugin.git` 的 `a639f23`（自共同祖先 `b52f27d` 起的全部 3 个提交：`7efe1cf`、`c2c1e66`、`a639f23`）
- **状态**：已定稿，待执行窗口实施

---

## 一、背景与目标

当前仓库 `main`（`e810ec3`）基于上游 `b52f27d`（`v0.1.59`）合入了 PR #1（WorkBuddy 配额对齐与模型倍率纠偏），并已回退双开共存改造（保持单开原版 `freetier` 身份）。此后上游连续发布了三个版本：

1. **`7efe1cf`（v0.1.60）**：`internal/vendors/workbuddy/tasks.go` — `ReportBillingEvents` 取消 `IsGlobal()` 区域条件，国服/国际服统一剥离 `knowledgeId` 与 `knowledgeName`，修复国服活跃上报被计费域校验拒绝（code 10001 / 400）的问题。
2. **`c2c1e66`（v0.1.61）**：
   - `internal/app/executor.go` + `executor_bench_test.go`：顶层 `"model"` 原地扫描替换 `spliceModelValue`（带 fallback 兜底）及 `streamEmitRequest` 结构体化；
   - `internal/state/state.go`：`saveLocked` 改为紧凑 `json.Marshal`；
   - `internal/vendors/workbuddy/chat.go` + 相关测试：流式帧改结构体白名单解析、请求体解码改用 `UseNumber`、丢弃 `: heartbeat` 等 SSE 注释行。
3. **`a639f23`（v0.1.62）**：
   - `internal/admin/admin.go`、`console.go`、`console_test.go`：按 `basePath` 缓存 `PageBytes`；
   - `internal/admin/page/{20-core,40-quota,80-columns}.html`：复用鉴权探针 `GET /vendors`、首屏先渲染 `/status` 缓存并仅自动刷新超过 10 分钟或缺失缓存的过期额度（`QUOTA_AUTO_REFRESH_MS = 10 * 60 * 1000`），手动查询仍全量刷新。

本次目标是将上述 3 个提交完整合入当前仓库，同时**完整保留 PR #1（`e810ec3`）的所有修复与测试守卫**，经 CI 全绿验证后合并 `main` 并发布 `v0.1.62`。

---

## 二、实施规格 (Technical Specification)

1. **分支与合并方式**：
   - 从 `e810ec3` 创建特性分支 `feat/sync-upstream-v0.1.60-62`（纳入本 Spec 文件）；
   - 从 `https://github.com/cxy2730/CLIProxyAPI-freetier2api-plugin.git` 拉取 `main` 至 `a639f23`，以保留三路合并基线的方式合并 `a639f23`（若无 `upstream` remote 可按临时 fetch URL 拉取，避免污染 `origin`）；
   - 核查合并 diff，确认：
     - `internal/vendors/workbuddy/tasks.go` 中 PR #1 的余额/资源包解析（`SubProductCode`、`Status` 过滤、非周转基础包回退、双账 `size` 一致性等）与上游 `ReportBillingEvents` 的无条件剥离 `knowledgeId`/`knowledgeName` **共存无误**；
     - `internal/vendors/workbuddy/chat.go` 中 PR #1 的 `codeBuddyCLIUA = "CLI/" + defaultCLIVersion + " CodeBuddy/" + defaultCLIVersion` 完整保留，且与上游流式帧解析/心跳丢弃改动兼容；
     - `internal/vendors/workbuddy/adapter.go`、`models.go`、`*_test.go` 中 PR #1 的改动与守卫测试完整无损。
2. **PR 与 CI 验证**：
   - 将 `feat/sync-upstream-v0.1.60-62` 推至 `origin` 并向 `main` 发起 PR；
   - 等待 GitHub Actions `test` 作业 6 步全绿（`gofmt`、`go vet`、`go test ./...`、`-race`、`live` 编译、`go build -buildmode=c-shared`）。
3. **合并与发版 (`v0.1.62`)**：
   - CI 全绿且审查通过后合并 PR 至 `main`；
   - 在 `main` 最新提交上创建附注 tag `v0.1.62`（简明列出合入 PR #1 保留项与上游 `v0.1.60~v0.1.62` 同步要点）并推送到 `origin`；
   - 等待 tag 触发的 CI `test` → `build`（`linux/amd64`、`linux/arm64`、`windows/amd64`）→ `release` 全部成功，确认 GitHub Release `v0.1.62` 生成完整 3 平台 `freetier_0.1.62_*.zip` 与 `checksums.txt`。

---

## 三、底层假定与非目标 (Assumptions & Non-Goals)

1. **底层假定**：
   - `e810ec3` 与 `a639f23` 共享祖先 `b52f27d`，改动位置互相错开，预期可干净合并或仅需极小冲突检查；
   - 插件身份保持单开原版 `freetier`（`freetier.dll` / `freetier.so`、`plugins.configs.freetier`）。
2. **明确非目标 (Out of Scope)**：
   - 不重新引入任何双开共存（`freetierplus`）代码或文档；
   - 不改动 PR #1（`e810ec3`）引入的 WorkBuddy 配额解析、模型目录与倍率逻辑；
   - 不修改 `192.168.1.200` 线上 CPA 配置/动态库及本机 `~/.claude/settings.json`；
   - 不新增上游 `7efe1cf`、`c2c1e66`、`a639f23` 之外的功能性改动。

---

## 四、验收标准 (Acceptance Criteria)

1. PR 中 GitHub Actions 6 步 CI 全绿（Exit Code 0）；
2. PR #1 所有 WorkBuddy 测试与上游新增测试（`executor_bench_test.go`、`console_test.go`、`stream_bench_test.go` 等）均通过且无功能回退；
3. `main` 合入完成，`v0.1.62` tag 触发的三平台构建与 GitHub Release `v0.1.62` 发布成功（含 `linux_amd64`、`linux_arm64`、`windows_amd64` 归档及 `checksums.txt`），且本地工作区干净。
