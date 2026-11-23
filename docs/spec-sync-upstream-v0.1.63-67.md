# 设计规格说明书 (Spec)：全量同步上游 v0.1.63 ~ v0.1.67 并发布 v0.1.67.1

- **项目路径**：`D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main` | **基线**：`c9a3c69`（`main`，`v0.1.64`） | **上游目标**：`cxy2730/CLIProxyAPI-freetier2api-plugin` `df2812d`（`v0.1.67`）

## 1. 目标与实证
- **核心目标**：以三路合并全量同步上游 `v0.1.63 ~ v0.1.67`（Go 1.26、`ubuntu-24.04` 防旧 CPU SIGILL、控制台英文切换、Qoder 排队 `503+Retry-After`、ZCode 订阅展示、WorkBuddy 视频过滤与 `11102` 24h 隔离、CPA 8.1.0 ABI `schema 10` 降级协商、移除 `config_fields` 表单），完整保留本地 PR #1（WorkBuddy 配额分组/月度包回退/倍率纠偏）及 `zhenghefreellm` 独立身份。
- **只读实证**：`git merge-tree` 干跑 38 个变更文件中 35 个自动通过，仅 3 处文本冲突（`10-body.html`、`30-accounts.html`、`workbuddy/models_test.go`）；CPA 宿主 `internal/pluginstore` 按 `.` 拆段做整数比较并精确匹配资产名，四段纯数字 `0.1.67.1` 合法可比且不被 GitHub 改名。

## 2. 流程与兜底
- **合并与解冲突**：保留共同祖先三路合并；`10-body.html` 保留新标题并合入语言下拉框；`30-accounts.html` 用 `t(...)` 包裹新空账号提示；`workbuddy/models_test.go` 合并双方全部测试函数。
- **多语言词典补全（选定方案 A）**：更新 `20-core.html` 词典使空账号提示匹配 `Zhenghe FreeLLM 2API`，并补入本地新增前端可见文案（`浮动倍率`、`基础月度套餐`、`活动与签到奖励`）的中英文映射，确保渲染路径经 `t()` 翻译且不影响中文模式原样输出。
- **兼容性兜底**：`workbuddy/models.go` 确保上游视频过滤与 `11102` 隔离可与本地倍率合并及缓存共存；`ci.yml` 吸收 Go 1.26 与 `ubuntu-24.04` 构建修复，保留 `zhenghefreellm` 产物名、`-id zhenghefreellm` 与 Release 模板；`README*.md` 补入上游旧 CPU SIGILL 提示并保持独立仓库指引。

## 3. 硬数值（旧 → 新）
- **发布版本与 Tag**：`v0.1.64` → **`v0.1.67.1`**（注解 tag，对应归档名 `zhenghefreellm_0.1.67.1_<goos>_<goarch>.zip`）。
- **CI 工具链与 Linux Runner**：Go `'1.24'` → **`'1.26'`**；Linux 构建 runner `ubuntu-latest` → **`ubuntu-24.04`**。
- **宿主契约版本**：`SchemaVersion` 固定 `6` → **`10`（随宿主 `register/reconfigure` 自动降级）**；注册响应 `ConfigFields` 保留 → **清空（不再向宿主声明表单字段）**。

## 4. 不改啥与验收
- **禁止误改**：严禁改动 `core.ProviderKey`（`"freetier"`）、默认状态目录（`.freetier-plugin`）、环境变量（`FREETIER2API_PLUGIN_HOME`）、`/freetier-auth-url`、本地 WorkBuddy 余额/倍率规则、`registry.json` 与 `zhenghefreellm` 身份守卫；严禁改动 `192.168.1.200` 线上配置或 `~/.claude/settings.json`。
- **验收标准**：PR 阶段本地与 GitHub Actions 6 步 CI 全绿（含 `TestRegistryManifestIsValid`、`TestPluginIDMatchesLibraryName`、双方 WorkBuddy 单测及控制台英文模式测试）；合并 `main` 后打注解 tag `v0.1.67.1`，三平台归档与 `checksums.txt` 在 GitHub Release 生成齐全。
