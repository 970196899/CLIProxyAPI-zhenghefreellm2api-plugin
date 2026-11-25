# 设计规格说明书 (Spec)：全量同步上游 v0.1.68 ~ v0.1.75 并发布 v0.1.75.1

- **项目路径**：`D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main` | **基线**：`f615801`（`main`，`v0.1.67.1`） | **上游目标**：`cxy2730/CLIProxyAPI-freetier2api-plugin` `ce26d57`（`v0.1.75`）

## 1. 目标与实证
- **核心目标**：以三路合并全量同步上游 `v0.1.68 ~ v0.1.75`（WorkBuddy 积分耗尽 14018 跨模型冷却与任务 429 跳过、控制台工具栏重排/渠道分页/移动端反馈、移除 OpenCode ZEN、Qoder 权限模型上下架与主源读取、Trae DeepSeek 工具恢复、Raccoon 回调轮询及 Cline/AutoClaw/CodeArts/MiniMaxCode/ZCode 更新），完整保留本地 PR #1 与 `zhenghefreellm` 独立身份。
- **只读实证**：`git merge-tree` 干跑确认 90 余个变更文件中仅 `registry.json` 与 `internal/admin/console_test.go` 存在文本冲突，其余自动合并；WorkBuddy PR #1 改动区与上游冷却/429 改动区互相错开。

## 2. 流程与兜底
- **合并与解冲突**：创建分支 `feat/sync-upstream-v0.1.68-75` 执行保留共同祖先的三路合并；`registry.json` 保留 `zhenghefreellm` 身份/仓库地址并移除 OpenCode ZEN 与 `"OpenCode"` 标签；`console_test.go` 并存翻译守卫与移动端反馈测试并将路由改为 `/plugins/zhenghefreellm`。
- **前端与多语言补全**：核查 `10-body.html`、`30-accounts.html`、`00-head.html` 保留 `Zhenghe FreeLLM 2API` 标题/空账号提示与 `#language` 控件；在 `20-core.html` 保留既有词条并补齐上游新增 3 条中文界面文案（`← 左右滑动查看完整表格 →`、`全部`、`该渠道下没有匹配的账号。`）的英文映射，同步扩展翻译守卫测试。
- **文档与逻辑复核**：`README.md` / `README_EN.md` 保留独立维护与升级指引，合入移除 OpenCode ZEN 与「一键任务与限流」说明；核查全局无新增 `/plugins/freetier` 残留引用，确认 `workbuddy/` 与 `executor.go` 本地修复完整无损。

## 3. 硬数值（旧 → 新）
- **发布版本与 Tag**：`v0.1.67.1` → **`v0.1.75.1`**（注解 tag，归档名 `zhenghefreellm_0.1.75.1_<goos>_<goarch>.zip`）。
- **上游同步基线**：`v0.1.67`（`df2812d`） → **`v0.1.75`（`ce26d57`）**。
- **OpenCode ZEN 渠道**：保留 → **随上游彻底移除（README 声明供应商数 `15 → 14`）**。

## 4. 不改啥与验收
- **禁止误改**：严禁改动 `core.ProviderKey`（`"freetier"`）、默认状态目录（`.freetier-plugin`）、`FREETIER2API_PLUGIN_HOME`、`/freetier-auth-url`、`50-models.html` 的 `BASE + "/config"`、PR #1 WorkBuddy 余额/倍率规则、`ci.yml`/`build.sh` 产物守卫，以及 `192.168.1.200` 线上配置、本机 `plugins\windows\freetier-v0.1.59.dll` 与 `~/.claude/settings.json`。
- **验收标准**：PR 阶段 GitHub Actions 6 步 CI 全绿（含 `TestRegistryManifestIsValid`、`TestPluginIDMatchesLibraryName`、`TestConsoleTranslationsCoverLocalStrings`、`TestConsoleMobileFeedbackVisible`）；`reviewer` 终审全票 PASS 后合入 `main` 并打注解 tag `v0.1.75.1`，确认 Release 生成三平台归档与 `checksums.txt` 齐全。
