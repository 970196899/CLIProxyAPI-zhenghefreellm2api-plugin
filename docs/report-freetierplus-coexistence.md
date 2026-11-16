# FreeTier Plus 全量双开共存改造 · 完工战报

- **工单来源**：「需求」窗口 2026-11-16 移交
- **规格说明书**：`docs/spec-freetierplus-coexistence.md`
- **施工方案**：`docs/plan-freetierplus-coexistence.md`
- **分支**：`feat/freetierplus-coexistence`，基线 `e810ec3`
- **PR**：[#2](https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin/pull/2)（草稿态，**未合并**）
- **最终提交**：`52e96e2`，CI run [43894096948](https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin/actions/runs/43894096948) 六步全绿
- **状态**：**主体完成 + 一项已知残余风险（用户已拍板方案一）**

---

## 一、交付内容

插件身份从 `freetier` 全面改为 `freetierplus`，可与上游原版在同一 CPA 实例双开共存。保留全部 10 个供应商与原模型名（不加前缀）。

| 维度 | 变化 |
| --- | --- |
| Plugin ID / ProviderKey | `freetier` → `freetierplus` |
| 显示名 | `FreeTier 2API` → `FreeTier Plus` |
| 管理路由 | `/plugins/freetier` → `/plugins/freetierplus` |
| 登录路由 | `/freetier-auth-url` → `/freetierplus-auth-url` |
| 状态目录 | `.freetier-plugin` → `.freetierplus-plugin` |
| 环境变量 | `FREETIER2API_PLUGIN_HOME` → `FREETIERPLUS_PLUGIN_HOME` |
| 日志前缀 | `freetier-` / `[freetier]` → `freetierplus-` / `[freetierplus]` |
| 前端存储键 | `freetier-model-optional-columns` → `freetierplus-...` |
| Qoder 缓存目录 | `freetier` → `freetierplus` |
| 构建产物 | `freetier.<ext>` → `freetierplus.<ext>` |

改动范围：53 个文件，+1433 / −179 行。未触碰 `go.mod`、`cpasdk/`、`autoclaw` 的上游指纹头。

**新增两道守卫**：
1. `tests/integration/identity_residue_test.go` — 防残留守卫，全仓扫描身份字面量，168 处 → 0
2. `internal/auth/pool_owned_test.go` — 写入门禁的单元 + Load 级接线断言

---

## 二、终审过程（三轮）

### 第一轮：必须整改
- **Critical**（部分成立）：`filterPluginAuths` 的第二层文件名判据被保活链路复用，双开时会改写原版凭证。
  - 我最初判断「type 不会被改写」是**错的**——只查了 trae 的 `MergeStorageJSON` 就下全局结论。审查员指出 WorkBuddy 走另一条路径：`credential.go:340` 的 `StorageJSON()` 无条件写 `type`，`mergeMaps` 是 src 优先，原文件的 `type=freetier` 确实会被覆盖。
- 其余：README 缺机器盐说明、英文版有矛盾承诺、守卫豁免过宽。

### 第二轮：必须整改
- **Critical 1**：停用/删除走 `hostbridge.ListAuths`，绕过了 `Pool.Load` 的门禁，可跨插件删对方凭证。
- **Critical 2**：`ownedByPlugin` 用 provider 单维判据，会误伤「被前置插件抢走 provider 的本方账号」——这恰恰是本插件历史上刻意维护的容错。
- **修复**：引入 `core.CredentialOwnedByType`，判据改用凭证内容的 `type`；覆盖面扩到停用/删除/手动签到。

### 第三轮：附建议放行
- **Important 1**：五个供应商（cline/qoder/autoclaw/raccoon/minimaxcode）的凭证不写 `type`，门禁对它们空转。
- **Important 2**：判据过严会误伤本插件承认的历史形态（cline/workbuddy/codebuddy）。
- **修复**：判据改为**排除法**（`foreignCredentialTypes` 表，目前只有 `freetier`），两端都对了；补 Load 级接线断言。

---

## 三、已知残余风险（用户已拍板方案一）

**五个 typeless 供应商在双开时仍会互刷 token。**

`cline`、`qoder`、`autoclaw`、`raccoon`、`minimaxcode` 的凭证文件不写 `type` 字段（改造前就不写）。写入门禁靠 `type` 判断归属，对空 `type` 只能按「本方」处理。因此这五家的原版凭证与本插件凭证，门禁分不出来——双开时两边仍会各自续期同一份 refresh token，可能互相刷失效。

**受影响**：上述五家。
**不受影响**：WorkBuddy、Trae、CodeArts、ZCode（写 `type`，受门禁保护）。

**用户选择**：方案一 —— 接受现状，README 写明规避方式（零风险，不改供应商代码）。

**已落实**：`README.md:157` 与 `README_EN.md:157` 第 6 条明确写出：这五家的账号只在一侧挂载，或只在其中一侧启用自动保活与签到。

**未选的方案及代价**：
- 方案二（让这五家也写 `type`）：存量凭证不会迁移，只解决一半；且触碰本工单「严禁误改供应商业务逻辑」的边界。
- 方案三（`state.json` 记归属名单）：需新增持久化设计，超出本工单范围，应单独立项。

---

## 四、过程中的失误（如实记录）

1. **只查一个供应商就下全局结论**：抽查 trae 的 `MergeStorageJSON` 后断言「type 不会被改写」，而 WorkBuddy 的路径恰好会改写。审查员纠正后已复核确认。
2. **用 Python 脚本批量改文件，未验证结果**：一次写坏 `identity_residue_test.go` 的 map 块（重复条目、丢闭合括号），一次误删 `trackExemption`/`exemptionHits` 定义，一次 `sort` import 替换未匹配却未检查。三次都靠 CI 兜住，各花一轮。
3. **测试夹具选错判据**：文件名用 `wb-*` 前缀，不满足 `filterPluginAuths` 的判据，导致测试到不了门禁，误以为门禁失效。

根因都是同一个：本机无 Go 工具链，无法本地验证，而我对脚本化改动缺乏结果校验。

---

## 五、其他记录

- **禁止误改项全部完好**：`go.mod`、200+ 处 import 路径、`autoclaw` 的 `x_trace_id`、`opencodezen` 的免费层语义、`cpasdk/` 均未改动。
- **gofmt 反复卡壳**：本机无 gofmt，注释列对齐、顶层声明间空行、map 字面量对齐都只能靠规则检查猜，共失败 5 次。
- **PR #2 仍为草稿态**，未合并到 main。
- 审查员明确要求：**不得在 typeless 问题拍板前合并发版**。用户已选方案一且 README 已落实，该前提已解除；但合并动作本身仍需用户明确指示。

---

## 六、遗留

1. **PR #2 未合并** —— 待用户指示。
2. **端到端实测未做** —— 全部为单元测试 + CI，未在真实 CPA 实例上验证双开共存。
3. **机器盐变化** —— 状态目录改名后 WorkBuddy 设备指纹会变，README 第 4 条给了两条自助保留方案，但未实测验证。
4. **存量凭证迁移** —— `type=freetier` 的存量凭证需用户手工改，无自动迁移（Spec 明确非目标）。
