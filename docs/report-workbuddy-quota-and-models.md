# WorkBuddy 配额对齐与模型倍率纠偏 · 完工战报

- **工单来源**：「需求」窗口 2026-11-16 移交
- **规格说明书**：`docs/spec-workbuddy-quota-and-models.md`
- **施工方案**：`docs/plan-workbuddy-quota-and-models.md`
- **仓库**：`970196899/CLIProxyAPI-zhenghefreellm2api-plugin`（fork 自 `cxy2730/CLIProxyAPI-freetier2api-plugin`）
- **分支**：`feat/workbuddy-quota-models`，基线 `b52f27d`
- **PR**：[#1](https://github.com/970196899/CLIProxyAPI-zhenghefreellm2api-plugin/pull/1)（草稿态，未合并）
- **最终提交**：`1eb8eb9`，18 个提交，工作区 clean

---

## 一、七项要求的落地情况

| Spec 要求 | 状态 | 关键实现 |
| --- | --- | --- |
| 1. Status 过滤死包（P0） | 已实现 | `billingResourceAccount.Status` + `parseBalance` 跳过非 0 |
| 2. 500 点基础包回退（P0） | 已实现 | `packageRemainUsed` 的 `nonRollingBase` 五条件 |
| 3. 按 SubProductCode 分桶 | 已实现 | 结构体/`BalancePackage` 加字段 + 三组展示 |
| 4. 国际服产品码自适应 | 已实现 | `FetchBalance` 仅国服注入 `p_tcaca` |
| 5. 模型倍率以企业端点为准（P0） | 已实现 | `mergeModelCatalogs` + `displayCredits` |
| 6. 探测 UA 现代化 | 已实现 | 复用 `defaultCLIVersion` 拼接 |
| 7. 自动化测试覆盖 | 已实现 | 新增 18 个测试用例 |

改动规模：9 个文件，+1623 / −34 行。生产代码仅动 `internal/vendors/workbuddy` 单包，未触碰 `cpasdk/`、`internal/core/` 或其他 vendor。

---

## 二、红绿实证（每项均有 CI 记录）

本机无 Go 工具链（无 go.exe、无 Docker、WSL 的 AlmaLinux-8 停止），所有证据来自 GitHub Actions。

| Task | 红灯 run | 红灯断言 | 绿灯 run |
| --- | --- | --- | --- |
| 1 | 43833913807 | `total = 7493, want 2004` | 43834540583 |
| 2 | 43834880542 | `base pack: remain=0 used=500 size=500` | 43835095809 |
| 3 | 43835474538 | 单组「资源包明细」+ 同名包合并为 `600 / 600` | 43835753795 |
| 4 | 43836469000 | 国际服请求体带 `"ProductCode":"p_tcaca"` | 43836718988 |
| 5 | 43837021276 | `undefined: mergeModelCatalogs` | 43837516139 → 43837984374（gofmt 修正后） |
| 6 | 43838348897 | `CLI/2.63.2 ... want CLI/2.179.0` | 43838857881 |
| 终审整改 | 43846357053 | `lifetime pack: remain=3000 used=2000 size=5000` | 43846878515 |
| 最终 | — | — | 43848694981 |

绿灯判据统一为六步全绿：`gofmt`、`go vet`、`go test ./...`、`race tests`、`编译 live 探针`、`校验 c-shared 编译`。

---

## 三、终审与整改

**首轮终审裁决：必须整改（Needs fixes）**

1 条 Important + 7 条 Minor。

### Important（已修）

`nonRollingBase` 的四条件无法排除幽灵额度。反例：某包 `Capacity 5000/3000/0`（终身额度）+ `Cycle 500/0/500`（本月用尽），四条件全部成立 → 被汇报为 `3000/5000`，把从未动用的终身额度当成可用余额，且总总额虚增 4500。这是与被修复缺陷同类的反向错误。

**整改**：增加第五条件 `CapacitySize == CycleCapacitySize`——只有描述同一笔发放的两本账才能互证。新增反例测试 `TestPackageRemainUsedRejectsNonBasePackWithoutMatchingSize`，红灯实证 `remain=3000 used=2000 size=5000, want 0/500/500`。

### 已修的 Minor

| 编号 | 内容 |
| --- | --- |
| Minor 1 | `billingResourceResponse` 的旧注释与「国际服不注入产品码」矛盾，会诱导后人复现原缺陷 |
| Minor 2 | `ModelCredits()` 改用 `displayCredits` 会让哨兵「浮动倍率」经 `state.json` 回灌进 `ModelInfo.Credits`，破坏方案自设不变量 |
| Minor 6 | 两处 `Status` 断言只校验长度，改为校验 `[0, 3]` 实际取值 |

另补一条接线回归测试 `TestModelCreditsExcludesDisplaySentinel`，锁住「展示路径可用哨兵、持久化路径不可」。

### 未修的 Minor（已知取舍）

| 编号 | 内容 | 不修理由 |
| --- | --- | --- |
| Minor 3 | 企业端点的脏倍率会无条件覆盖 v3 值 | Spec 明确要求「企业端点为准」；全仓无代码把 `Credits` 当数值消费，仅影响展示 |
| Minor 4 | 试用横幅模型的 Credits 可能被企业目录回填 | 需企业目录恰好含同名试用 ID，概率极低 |
| Minor 5 | 分组映射两份 switch + rank 上限硬编码为 2 | 当前无功能缺陷；但**已知隐患**：将来新增 rank > 2 的产品码会让该 bucket 完全不进任何组且 CI 全绿 |
| Minor 7 | `Status` 若被上游改成字符串类型，整个余额接口会硬失败 | 请求侧发整数，概率低；修复需自定义 `UnmarshalJSON`，超出本次范围 |

### 复核裁决

**附建议放行（Approved with minor fixes）**。原阻断项经确定性反例测试验证已销项，受保护用例与基线逐字节一致，测试零删改。

---

## 四、必须知晓的三项证据缺口

### 1. `Status` 与 `SubProductCode` 两个字段名是假设

Spec 声称从真实上游响应观测到这两个字段（含「69 个包 / 25 个 Status 0 / 44 个 Status 3 / 虚增 5489 点」的统计），但仓库内**无任何真实响应样本或 fixture 可复核**。全仓 `grep` 除 Spec 文档本身外零引用。

已做的失败方向安全设计：`Status` 缺失时零值为 0（= 生效中，不过滤），退化结果与旧版一致，不会把有效额度误判成过期。

**但这也意味着**：若上游字段真的改名，虚增会原样回归。

### 2. 双账 size 同额约束是新的承重假设

终审追加的 `CapacitySize == CycleCapacitySize` 依赖「每月一条独立记录且两账 size 相同」。若真实基础包会跨月累加（如第 2 个月 Capacity 1000 而 Cycle 500），回退会失效、500 点被吞的原缺陷会静默回归，且 CI 全绿——现有测试只锁「不等不回退」，锁不住「相等必须回退」的真实性。

**建议**：用真实国服账号读取一次 `Accounts` 原始响应，重点确认使用超过一个周期账号的基础包两账 size 是否相等。

### 3. 「不可约残留」无法靠算术消除

同为 500/500 的周转包，若上游 Capacity 账从不递减，与基础包数值完全同形。任何「采信 Capacity」的规则都会误判它未动用，任何「采信 Cycle」的规则都会吞掉基础包。这是上游双账本语义的固有歧义，只能靠产品码白名单或真实样本收敛。

---

## 五、行为迁移（未被测试覆盖，但已论证）

`Status` 过滤会让「个人包全部为 Status 3」的企业账号首次真正走到企业用量端点。旧代码因死包 `size` 累加使 `resolveBalance` 的 `personal.Total > 0` 恒成立，从未触发过该回退。这是正向改动，但建议在真机上确认企业端点返回的 `limitNum` 正常。

---

## 六、流程与环境说明

- **fork 阻断**：该仓库是 fork，GitHub 默认不注册 workflow。需仓库所有者在网页端首次启用 Actions（用户已操作），此后 `workflows` 计数从 0 变为 1，CI 才可触发。
- **gh 凭据**：`gh auth status` 报 x509 错误实为 conda 的 `SSL_CERT_DIR` 导致，非 token 失效。所有 `gh` 命令需加 `env -u SSL_CERT_DIR -u NODE_EXTRA_CA_CERTS` 前缀。
- **CI 日志下载**：`gh run view --log` 走 Azure blob 直链，在本机会 EOF 失败。改用 `gh api repos/.../actions/runs/<id>/logs` 下载 zip 归档后解压，可稳定取得。
- **本地 git 身份**：仓库未配置全局身份，已设本地 `user.name=970196899` / `user.email=970196899@users.noreply.github.com`，未改全局配置。

---

## 七、遗留

1. **PR #1 仍为草稿态**，未合并到 main。合并前建议先用真实账号验证第四节的三项缺口。
2. **端到端实测未做**：本次全部为单元测试与 CI 验证，未接真实 WorkBuddy 账号跑通「配额面板显示 + 模型倍率展示」的完整链路。
3. **Minor 5 的隐患**：若后续新增产品码，必须先扩 rank 表再接码，否则额度会在面板上静默消失。
