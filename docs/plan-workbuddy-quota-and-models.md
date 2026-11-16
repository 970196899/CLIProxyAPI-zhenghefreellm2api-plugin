# WorkBuddy 配额对齐与模型倍率纠偏 施工方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 WorkBuddy（CN/Global）配额里的死包虚增与 500 点基础包被吞、按 `SubProductCode` 分类展示、国际服产品码自适应、模型倍率改以企业计费端点为准、探测 UA 现代化，并为每项补齐单测。

**Architecture:** 全部改动收敛在 `internal/vendors/workbuddy` 单包内，不跨层、不新增依赖。配额侧在既有 `billingResourceAccount → parseBalance → Balance → BuildQuotaResponse` 链路上做四处最小增强（状态过滤、基础包回退、分组展示、请求体分域）；模型侧把「v3 骨架 + 企业端点纠偏」抽成纯函数 `mergeModelCatalogs`，使倍率规则可脱离宿主 HTTP 做表驱动单测；UA 侧只改一个常量表达式。

**Tech Stack:** Go 1.22（`go.mod`），CI 用 Go 1.24；标准库 only（模块无 `go.sum`）；`testing` 表驱动；宿主 HTTP 桥 `internal/hostbridge` 在测试中可替身化（`hostbridge.Configure`）。

**Spec:** `D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main\docs\spec-workbuddy-quota-and-models.md`

---

## Global Constraints

- 只允许改动这 7 个文件：`internal/vendors/workbuddy/tasks.go`、`models.go`、`chat.go`、`adapter.go`、`tasks_test.go`、`chat_headers_test.go`，以及新建 `internal/vendors/workbuddy/models_test.go`。**不得**触碰 `cpasdk/`、`internal/core/`、`internal/admin/`、`internal/app/` 或其他 vendor 目录。
- 不允许引入第三方依赖；`go.mod` 保持不变。
- 代码注释一律简体中文，与该包现有风格一致（说明「为什么」，不复述「做了什么」）。
- 必须通过 CI 的 `gofmt -l .`（排除 `cpasdk/`）。本机无 Go 工具链，格式只能由 CI 把关：缩进用 tab、行尾不留空白、对齐按现有列宽。
- 本机无 `go.exe`、无 Docker、WSL `AlmaLinux-8` 为 Stopped。**任何 `go test` 都不得假设本地可运行**，红绿一律取 GitHub Actions 结果。
- Spec 钉死的字面量必须逐字照抄：`500`、`"p_tcaca"`、`"sp_tcaca_codebuddy_ide"`、`"sp_tcaca_codebuddyide_bonus_pack"`、`"CLI/2.179.0 CodeBuddy/2.179.0"`、分组名 `基础月度套餐` / `活动与签到奖励` / `资源包明细`、文案 `浮动倍率`、`"PageSize": 100`、`"PageNumber": 1`、`"Status": []int{0, 3}`。
- 严禁为了让新用例变绿而删改既有断言、放宽阈值或替换为假数据。

---

## 施工前置（只做一次，不计入 Task）

**凭据实况（已复核，修正初稿判断）**：`gh auth status` 报的是 token 正常（账号 `970196899`，scopes 含 `repo`/`workflow`）；初稿所称「token 失效」实为未加 `env -u SSL_CERT_DIR -u NODE_EXTRA_CA_CERTS` 前缀时撞上本机 conda 证书目录导致的 x509 报错。**所有 `gh` 命令必须带该前缀。**

**fork 阻断（当前唯一硬前置）**：该仓库是 `cxy2730/CLIProxyAPI-freetier2api-plugin` 的 fork，GitHub 未注册其 workflow（`repos/.../actions/workflows` 返回 0，`gh api .../actions/workflows/ci.yml` 返回 404，尽管 `ci.yml` 文件与 Actions 权限 `enabled:true` 均在）。**须由仓库所有者在网页端首次启用 Actions** 后，workflow 才会被注册。启用前任何 `gh run list` 恒为空。

1. 确认凭据与 workflow 已注册（两条都必须成立才可开工）：
   ```
   env -u SSL_CERT_DIR -u NODE_EXTRA_CA_CERTS gh auth status
   env -u SSL_CERT_DIR -u NODE_EXTRA_CA_CERTS gh api repos/970196899/CLIProxyAPI-zhenghefreellm2api-plugin/actions/workflows --jq '.total_count'
   ```
   预期：`gh auth status` 显示 `✓ Logged in`；第二条输出 `1`（或 ≥1）。若仍为 `0`，说明网页端尚未启用，**熔断并上报，不得跳过红绿环节继续施工**。
2. 先把现有未跟踪文档入库（`git status` 显示 `?? docs/`），避免方案文档游离在分支外：
   ```
   git add docs/spec-workbuddy-quota-and-models.md docs/plan-workbuddy-quota-and-models.md
   git commit -m "docs: 补入 WorkBuddy 配额与倍率 spec 及施工方案"
   ```
3. 推送并开草稿 PR（`ci.yml` 只在 `push: main`、`pull_request`、tag 上触发；特性分支必须走 PR）：
   ```
   git push -u origin feat/workbuddy-quota-models
   gh pr create --base main --head feat/workbuddy-quota-models --draft --title "feat(workbuddy): 配额对齐与模型倍率纠偏"
   ```
4. 每个 Task 的固定红绿循环（下文 `gh` 均指 `env -u SSL_CERT_DIR -u NODE_EXTRA_CA_CERTS gh`）：
   ```
   git push                      # 红灯提交
   gh run list --workflow=ci.yml --branch feat/workbuddy-quota-models --limit 1
   gh run view <run-id> --log-failed | tail -80
   # ……写实现……
   git push                      # 绿灯提交
   gh run watch <run-id> --exit-status
   ```
   - **红灯判据**：CI 的 `go vet` 或 `go test` 步骤失败，且失败日志中出现该 Task 指定的断言文案。
   - **绿灯判据**：`gh run watch --exit-status` 退出码 0，且 `gh run view <run-id>` 中 `gofmt`、`go vet`、`go test`、`race tests`、`编译 live 探针`、`校验 c-shared 编译` 六步全绿。
   - 红灯提交后必须立即推送同 Task 的实现提交，不允许把分支长时间留在红态。

**为何每个 Task 都要在 CI 上单独跑一轮红：** 本仓库无本地 Go，PR 上的 run 是唯一证据链；红态先入库，日后 `git log` 能直接证明「测试先于实现」。

---

## Review Focus

以下五类输入/条件，Spec 隐含但没有任何单个 Task 的测试能全部覆盖，最容易在真实账号上出事。每条都已在指定 Task 里挂了对应断言。

1. **上游根本不下发 `Status` 字段**（字段名假设无法自证）——期望行为是「全部照旧计入」，绝不能把字段缺失当成「已过期」。见 Task 1 的兼容用例。
2. **Cycle 与 Capacity 两本账同时陈旧**——回退规则可能复活幽灵额度（详见风险 R3）。期望行为：真正的耗尽包必须保持 0 剩余。见 Task 2 的耗尽用例。
3. **企业账号（带 `EnterpriseID`）个人接口返回 200 + 空 `Accounts`**——新的状态过滤与分组不能打乱 `resolveBalance` 的企业回退链路。见 Task 3 对既有 `TestBuildQuotaResponseEnterprisePlan` / `TestFetchBalanceFallsBackToEnterpriseUsage` 的回归约束。
4. **同一个 `PackageName` 出现在不同产品码下**——不能被跨组合并成一条，否则用户看到的分类又被抹平。见 Task 3 的跨组同名用例。
5. **国际服去掉 `ProductCode` 后分页与状态参数被顺手改掉**——请求体必须只少一个键。见 Task 4 的参数保全断言。

---

## 文件交叉矩阵

| 文件 | 涉及 Task | 备注 |
| --- | --- | --- |
| `internal/vendors/workbuddy/tasks.go` | 1, 2, 3, 4 | 四处改动共用 `billingResourceAccount` / `parseBalance`，**必须严格串行** |
| `internal/vendors/workbuddy/tasks_test.go` | 1, 2, 3, 4 | 同上，串行追加 |
| `internal/vendors/workbuddy/models.go` | 5 | 与 tasks.go 无交集 |
| `internal/vendors/workbuddy/adapter.go` | 5 | 仅 `ConsoleModels` / `ModelCredits` 两处调用点换用 `displayCredits` |
| `internal/vendors/workbuddy/models_test.go`（新建） | 5 | Task 5 创建 |
| `internal/vendors/workbuddy/chat.go` | 6 | 与上述无交集 |
| `internal/vendors/workbuddy/chat_headers_test.go` | 6 | UA 断言的既有归属文件 |

**依赖与顺序：** Task 1 → 2 → 3 → 4 强制串行（同一批函数的连续编辑，跳序会产生无意义的冲突）；Task 5、Task 6 互相独立，可在 Task 4 之后任意顺序执行。推荐整体顺序 1→2→3→4→5→6，让 CI 历史线性可读。Task 6 的最后一步即全量回归验收（Spec 要求 7 由前六个 Task 的测试分布式落实，不单设空壳任务）。

**行号约定：** 本文所有行号均为 `b52f27d` 快照。Task 1 之后的 Task 一律按符号名或语句锚点定位，不要复用旧行号。

**Task 3 依赖 Task 1 的物理前提：** Task 1 把 `Status` 插在 `billingResourceAccount.PackageName` 之后；Task 3 把 `SubProductCode` 也插在 `PackageName` 之后（把 `Status` 顺次下推）。最终字段顺序钉死为 `PackageName`、`SubProductCode`、`Status`、`CycleEndTime`、`CapacitySize`、`CapacityRemain`、`CapacityUsed`、`CycleCapacitySize`、`CycleCapacityRemain`、`CycleCapacityUsed`。

---

## Task 1: 配额状态过滤（Spec 要求 1，P0）

**Files:**
- Modify: `internal/vendors/workbuddy/tasks.go:2760-2769`（`billingResourceAccount` 结构体）
- Modify: `internal/vendors/workbuddy/tasks.go:2884-2898`（`parseBalance`）
- Test: `internal/vendors/workbuddy/tasks_test.go`（追加到文件末尾，当前 667 行）

**Interfaces:**
- Consumes: 无（本 Task 是链路起点）
- Produces: `billingResourceAccount.Status int64`（JSON 键 `Status`）；`parseBalance` 跳过 `Status != 0` 的条目，且被跳过的包不进 `Balance.Packages`

### 证据缺口（必须随代码一起保留）

全仓 `grep -rn "SubProductCode\|Status" internal/ tests/` 无任何真实响应样本、无 fixture 文件；Spec 里的「69 个包 / 25 个 Status 0 / 44 个 Status 3 / 虚增 5489 点」在本仓库无法复核。因此字段名 `Status` 是**假设**。实现必须让「字段缺失」等价于「不过滤」，把风险降级为「过滤不生效、行为与今天一致」，绝不能降级为「把有效包算成过期」。

- [ ] **Step 1: 写失败测试**

在 `tasks_test.go` 末尾追加两个用例：

```go
// TestParseBalanceSkipsInactiveAccounts 锁定死包过滤：
// 上游请求用 "Status": [0, 3] 同时取回生效包与历史死包，
// 把 Status 3 计入总额会让面板虚增数千点。
func TestParseBalanceSkipsInactiveAccounts(t *testing.T) {
	raw := `{"Response":{"Data":{"Accounts":[
	  {"PackageName":"基础套餐","Status":0,"CycleCapacitySize":1000,"CycleCapacityRemain":400,"CycleCapacityUsed":600},
	  {"PackageName":"历史死包","Status":3,"CapacitySize":5489,"CapacityRemain":5489},
	  {"PackageName":"加油包","Status":0,"CapacitySize":1004,"CapacityRemain":1004}
	]}}}`
	var payload billingResourceResponse
	if errUnmarshal := json.Unmarshal([]byte(raw), &payload); errUnmarshal != nil {
		t.Fatalf("unmarshal: %v", errUnmarshal)
	}
	balance := parseBalance(payload)

	if balance.Total != 2004 {
		t.Fatalf("total = %v, want 2004（Status 3 的 5489 点死包不能计入）", balance.Total)
	}
	if balance.Remain != 1404 {
		t.Fatalf("remain = %v, want 1404", balance.Remain)
	}
	if len(balance.Packages) != 2 {
		t.Fatalf("packages = %d, want 2（死包不应出现在明细里）", len(balance.Packages))
	}
}

// TestParseBalanceKeepsAccountsWithoutStatus 锁定兼容形态：
// 上游若不下发 Status（或字段名与假设不符），全部资源包必须照旧计入。
// 过滤只能剔除明确标记为非 0 的包，绝不能把「字段缺失」当成「已过期」。
func TestParseBalanceKeepsAccountsWithoutStatus(t *testing.T) {
	raw := `{"Response":{"Data":{"Accounts":[
	  {"PackageName":"加油包","CapacitySize":500,"CapacityRemain":500}
	]}}}`
	var payload billingResourceResponse
	if errUnmarshal := json.Unmarshal([]byte(raw), &payload); errUnmarshal != nil {
		t.Fatalf("unmarshal: %v", errUnmarshal)
	}
	balance := parseBalance(payload)
	if balance.Total != 500 || balance.Remain != 500 {
		t.Fatalf("balance = %v/%v, want 500/500", balance.Remain, balance.Total)
	}
}
```

**注意：** 第二个用例是**回归护栏**，实现前后都应为绿，不要为它追红灯。只有第一个用例预期红。

- [ ] **Step 2: 推送并验证红灯**

```
git add internal/vendors/workbuddy/tasks_test.go
git commit -m "test: 锁定 WorkBuddy 余额解析的死包过滤（红灯）"
git push
gh run list --workflow=ci.yml --branch feat/workbuddy-quota-models --limit 1
gh run view <run-id> --log-failed | tail -80
```

预期：`go test` 步骤失败，`internal/vendors/workbuddy` 的失败日志中出现全部三条断言文案：
- `total = 7493, want 2004（Status 3 的 5489 点死包不能计入）`
- `remain = 6893, want 1404`
- `packages = 3, want 2（死包不应出现在明细里）`

（7493 = 2004（Spec 报告的真实总额）+ 5489（Spec 报告的死包虚增），说明夹具与 Spec 的数量关系自洽；**但明细拆分（1000 / 5489 / 1004）是本方案构造的，不是真实样本**，见风险 R4。）

- [ ] **Step 3: 实现**

`billingResourceAccount`：在 `PackageName` 之后插入

```go
	// Status 是资源包状态：0 = 生效中，3 = 已过期/归档。
	//
	// 请求体用 "Status": [0, 3] 同时取回两类，因此过滤必须在解析期做。
	// 字段缺失时零值是 0，等价于「生效中」——上游改名不应把有效额度算没。
	Status int64 `json:"Status"`
```

`parseBalance`：在 `for` 循环体第一条语句加入 `if account.Status != 0 { continue }`，其余代码不变。

- [ ] **Step 4: 推送并验证绿灯**

```
git add internal/vendors/workbuddy/tasks.go
git commit -m "fix: 余额解析剔除 Status 非 0 的历史死包"
git push
gh run watch <run-id> --exit-status
```

预期：退出码 0；六步全绿；`TestPackageRemainUsedCycleWins`、`TestParseBalanceMatchesUpstreamShape` 等既有用例不受影响。

- [ ] **Step 5: 提交**

已在 Step 4 完成。确认 `git status --porcelain` 为空。

---

## Task 2: 月度基础包 500 点回退（Spec 要求 2，P0）

**Files:**
- Modify: `internal/vendors/workbuddy/tasks.go:2790-2816`（`packageRemainUsed`）
- Test: `internal/vendors/workbuddy/tasks_test.go`（追加在 Task 1 的用例之后）

**Interfaces:**
- Consumes: `billingResourceAccount`（Task 1 已加 `Status`，本 Task 不使用它）
- Produces: `packageRemainUsed` 在「Cycle 账已清零且 Capacity 账从未动用」时回退采信 `CapacityRemain/CapacityUsed/CapacitySize`

### 规则钉死

在 `CycleCapacitySize > 0` 分支前计算一个布尔量 `nonRollingBase`，条件为五者**同时**成立：

1. `account.CycleCapacitySize > 0`
2. `account.CycleCapacityRemain <= 0`
3. `account.CapacitySize > 0 && account.CapacityRemain > 0`
4. `account.CapacitySize == account.CycleCapacitySize`
5. `account.CapacityUsed == 0 || account.CycleCapacityUsed >= account.CycleCapacitySize`

`nonRollingBase` 为真 → 跳过 Cycle 分支、直接走既有 Capacity 分支；否则逻辑与今天完全一致（Case 1/2/3 三条既有断言全部保住）。

条件 5 完全照抄 Spec §三.1.2 的析取式；条件 3 的 `CapacitySize > 0` 是防止「Capacity 只剩脏数据」时算出 `size=0` 的除零形态。

**条件 4 是终审整改追加的**（原方案只有四条件）：只有描述同一笔发放的两本账才能互证。缺了它，「终身额度包」（Capacity 记终身发放量 5000、Cycle 记本月用量 500/0/500）会被误判成基础包，凭空复活 3000 点额度并抬高总额——与被修复缺陷同类、方向相反。反例由 `TestPackageRemainUsedRejectsNonBasePackWithoutMatchingSize` 钉死。

- [ ] **Step 1: 写失败测试**

```go
// TestPackageRemainUsedFallsBackToCapacityForBasePack 锁定月度基础包：
// 上游对非周转基础包下发 Capacity 500/500/0、Cycle 500/0/500，
// 无条件采信 CycleCapacityRemain 会把未动过的 500 点判成「剩余 0、已用 500」。
func TestPackageRemainUsedFallsBackToCapacityForBasePack(t *testing.T) {
	remain, used, size := packageRemainUsed(billingResourceAccount{
		CapacitySize: 500, CapacityRemain: 500, CapacityUsed: 0,
		CycleCapacitySize: 500, CycleCapacityRemain: 0, CycleCapacityUsed: 500,
	})
	if remain != 500 || used != 0 || size != 500 {
		t.Fatalf("base pack: remain=%d used=%d size=%d, want 500/0/500", remain, used, size)
	}
}

// TestPackageRemainUsedKeepsExhaustedCyclePackAtZero 锁定回退的反向边界：
// Cycle 期真的耗尽、Capacity 也没有可采信的剩余时，必须保持 0 剩余——
// 回退规则不能把已用完的周转包复活成满额。
func TestPackageRemainUsedKeepsExhaustedCyclePackAtZero(t *testing.T) {
	remain, used, size := packageRemainUsed(billingResourceAccount{
		CapacitySize: 1000, CapacityRemain: 0, CapacityUsed: 1000,
		CycleCapacitySize: 1000, CycleCapacityRemain: 0, CycleCapacityUsed: 1000,
	})
	if remain != 0 || used != 1000 || size != 1000 {
		t.Fatalf("exhausted: remain=%d used=%d size=%d, want 0/1000/1000", remain, used, size)
	}
}
```

第二个用例是护栏（实现前后皆绿），只有第一个预期红。

- [ ] **Step 2: 推送并验证红灯**

```
git add internal/vendors/workbuddy/tasks_test.go
git commit -m "test: 锁定月度基础包的 Capacity 回退（红灯）"
git push
```

预期 `go test` 失败，日志含 `base pack: remain=0 used=500 size=500, want 500/0/500`。

- [ ] **Step 3: 实现**

在 `packageRemainUsed` 函数体开头、现有 `if account.CycleCapacitySize > 0 {` 之前，加入 `nonRollingBase` 布尔量（按上文四条件），并把该 `if` 条件改为 `if account.CycleCapacitySize > 0 && !nonRollingBase {`。函数其余部分一字不改。

在函数文档注释里补一段说明「为什么需要回退」，点明这是上游对非周转基础包的双账本口径，而不是脏数据兼容。

- [ ] **Step 4: 推送并验证绿灯**

```
git add internal/vendors/workbuddy/tasks.go
git commit -m "fix: 非周转基础包回退 Capacity 账，修复 500 点月包被吞"
git push
gh run watch <run-id> --exit-status
```

预期：六步全绿，`TestPackageRemainUsedCycleWins` 三条断言全部保留通过。

- [ ] **Step 5: 提交**

已在 Step 4 完成。

---

## Task 3: 按 SubProductCode 分桶展示（Spec 要求 3）

**Files:**
- Modify: `internal/vendors/workbuddy/tasks.go:2760-2769`（结构体：在 `PackageName` 之后插入 `SubProductCode`，`Status` 顺次下推）
- Modify: `internal/vendors/workbuddy/tasks.go:2739-2755`（`BalancePackage` 加字段）
- Modify: `internal/vendors/workbuddy/tasks.go:2884-2898`（`parseBalance` 透传）
- Modify: `internal/vendors/workbuddy/tasks.go:3016-3106`（`BuildQuotaResponse` 分组）
- Test: `internal/vendors/workbuddy/tasks_test.go`（追加）

**Interfaces:**
- Consumes: `billingResourceAccount`（Task 1 已加 `Status`）
- Produces:
  - `billingResourceAccount.SubProductCode string`（JSON 键 `SubProductCode`）
  - `BalancePackage.SubProductCode string`
  - `quotaGroupDisplayName(subProductCode string) string`
  - `quotaGroupRank(subProductCode string) int`

### 规则钉死

| `SubProductCode`（trim + 小写后精确匹配） | 分组名 | rank |
| --- | --- | --- |
| `sp_tcaca_codebuddy_ide` | `基础月度套餐` | 0 |
| `sp_tcaca_codebuddyide_bonus_pack` | `活动与签到奖励` | 1 |
| 其余（含空串） | `资源包明细` | 2 |

- 聚合键为 `(SubProductCode, PackageName)` 二元组，**不再是单独的 `PackageName`**：同名包分属不同产品码时不得跨组合并。
- 分组输出顺序按 rank 升序，与上游返回顺序无关（保证面板稳定）。同秩内保持首次出现顺序。
- 空组不输出。
- 未知产品码统一落 `资源包明细`，保住旧行为：无 `SubProductCode` 的响应（含企业额度）仍然只有一组。

**既有测试为什么无需改动：** `TestBuildQuotaResponsePersonalDescription`（夹具无产品码 → rank 2，恰好 1 组）与 `TestBuildQuotaResponseEnterprisePlan`（企业包无产品码 → rank 2，恰好 1 组 1 桶，`Window` 仍是 `企业额度`）在「未知码 → 资源包明细」规则下断言全部成立。**禁止**为了新逻辑去改这两个用例。

- [ ] **Step 1: 写失败测试**

```go
// TestBuildQuotaResponseGroupsBySubProductCode 锁定资源包按产品码分类展示：
// 每日/任务奖励与月度基础套餐混在一个「资源包明细」里，用户看不出哪部分会过期。
func TestBuildQuotaResponseGroupsBySubProductCode(t *testing.T) {
	raw := `{"Response":{"Data":{"Accounts":[
	  {"PackageName":"每日签到","SubProductCode":"sp_tcaca_codebuddyide_bonus_pack","CapacitySize":100,"CapacityRemain":80},
	  {"PackageName":"月度基础包","SubProductCode":"sp_tcaca_codebuddy_ide","CycleCapacitySize":500,"CycleCapacityRemain":500},
	  {"PackageName":"裂变奖励","SubProductCode":"sp_tcaca_codebuddyide_bonus_pack","CapacitySize":100,"CapacityRemain":100}
	]}}}`
	var payload billingResourceResponse
	if errUnmarshal := json.Unmarshal([]byte(raw), &payload); errUnmarshal != nil {
		t.Fatalf("unmarshal: %v", errUnmarshal)
	}
	response := BuildQuotaResponse(testTaskCredential("uid-subproduct"), parseBalance(payload))

	if len(response.Groups) != 2 {
		t.Fatalf("groups = %+v, want 2（基础月度套餐 + 活动与签到奖励）", response.Groups)
	}
	if response.Groups[0].DisplayName != "基础月度套餐" {
		t.Fatalf("first group = %q, want 基础月度套餐（基础月包必须排在最前）", response.Groups[0].DisplayName)
	}
	if len(response.Groups[0].Buckets) != 1 || response.Groups[0].Buckets[0].Window != "月度基础包" {
		t.Fatalf("first group buckets = %+v", response.Groups[0].Buckets)
	}
	if response.Groups[1].DisplayName != "活动与签到奖励" {
		t.Fatalf("second group = %q, want 活动与签到奖励", response.Groups[1].DisplayName)
	}
	if len(response.Groups[1].Buckets) != 2 {
		t.Fatalf("second group buckets = %d, want 2", len(response.Groups[1].Buckets))
	}
}

// TestBuildQuotaResponseKeepsSameNameAcrossGroups 锁定聚合键必须带产品码：
// 两个产品码下都叫「赠送包」的两条记录不能被合成一条，否则分类又被抹平。
func TestBuildQuotaResponseKeepsSameNameAcrossGroups(t *testing.T) {
	raw := `{"Response":{"Data":{"Accounts":[
	  {"PackageName":"赠送包","SubProductCode":"sp_tcaca_codebuddy_ide","CapacitySize":500,"CapacityRemain":500},
	  {"PackageName":"赠送包","SubProductCode":"sp_tcaca_codebuddyide_bonus_pack","CapacitySize":100,"CapacityRemain":100}
	]}}}`
	var payload billingResourceResponse
	if errUnmarshal := json.Unmarshal([]byte(raw), &payload); errUnmarshal != nil {
		t.Fatalf("unmarshal: %v", errUnmarshal)
	}
	response := BuildQuotaResponse(testTaskCredential("uid-same-name"), parseBalance(payload))

	if len(response.Groups) != 2 {
		t.Fatalf("groups = %+v, want 2（同名包分属两个产品码，不能合并）", response.Groups)
	}
	for index, group := range response.Groups {
		if len(group.Buckets) != 1 || group.Buckets[0].Window != "赠送包" {
			t.Fatalf("group[%d] buckets = %+v", index, group.Buckets)
		}
	}
}
```

- [ ] **Step 2: 推送并验证红灯**

```
git add internal/vendors/workbuddy/tasks_test.go
git commit -m "test: 锁定资源包按 SubProductCode 分组展示（红灯）"
git push
```

预期 `go test` 失败，日志含 `groups = [{资源包明细 ...}], want 2（基础月度套餐 + 活动与签到奖励）`。

- [ ] **Step 3: 实现**

1. `billingResourceAccount`：在 `PackageName` 后插入
   ```go
   	// SubProductCode 是上游的产品码：基础月包与活动奖励包用它区分。
   	// 缺失时为空串，由 quotaGroupRank 归入兜底分组。
   	SubProductCode string `json:"SubProductCode"`
   ```
   （注意 Task 1 已插入的 `Status` 要排在其后。）
2. `BalancePackage`：在 `Name` 后插入 `SubProductCode string`。
3. `parseBalance`：构造 `BalancePackage` 时补 `SubProductCode: strings.TrimSpace(account.SubProductCode)`。
4. 新增两个包级函数 `quotaGroupDisplayName` / `quotaGroupRank`，按上表返回分组名与秩；匹配前统一 `strings.ToLower(strings.TrimSpace(...))`。
5. `BuildQuotaResponse`：把聚合 map 的键换成 `(code, name)` 二元结构体（Go 中可比较，可直接作 map key），`order` 切片同步存该结构体；输出阶段按 rank 0/1/2 三轮遍历 `order`，把同秩的桶依次收进该组，跳过空组。桶的构造逻辑（fraction、description、ResetTime）一字不改。

- [ ] **Step 4: 推送并验证绿灯**

```
git add internal/vendors/workbuddy/tasks.go
git commit -m "feat: 资源包按 SubProductCode 分为基础月度套餐与活动奖励两组"
git push
gh run watch <run-id> --exit-status
```

预期六步全绿；`TestBuildQuotaResponsePersonalDescription`、`TestBuildQuotaResponseEnterprisePlan` 原样通过。

- [ ] **Step 5: 提交**

已在 Step 4 完成。

---

## Task 4: 国际服产品码自适应（Spec 要求 4）

**Files:**
- Modify: `internal/vendors/workbuddy/tasks.go:2821-2849`（`FetchBalance` 请求体）
- Test: `internal/vendors/workbuddy/tasks_test.go`（追加，含一个新的抓包 helper）

**Interfaces:**
- Consumes: `region.IsGlobal()`（`chat.go:399`）、`billingMeterPaths`（`tasks.go:2855`）
- Produces: `FetchBalance` 在 CN 下请求体含 `ProductCode: "p_tcaca"`，在 Global 下完全不含该键；`PageNumber`/`PageSize`/`Status`/时间范围四组参数两域一律不变。

### 裁定：分页截断不纳入本次范围

Spec §二.4 的标题提到「分页截断」，但 §三（技术方案）与 §五（验收标准）均未给出翻页规则、终止条件或总量字段语义。本仓库无任何真实响应样本可核实 `TotalCount` / `TotalDosage` 的语义（`TestFetchBalanceFallsBackToEnterpriseUsage` 的夹具里这两个字段恒为 0）。在没有证据的前提下加翻页循环，等于凭空发明停止条件，属于臆造。

**结论：** 本 Task 只做产品码分域；分页留作后续独立工单，触发条件写明在下方风险 R5。`PageSize: 100` / `PageNumber: 1` 保持字面量不变，并加测试锁死，防止顺带被改。

- [ ] **Step 1: 写失败测试**

在 `tasks_test.go` 追加抓包 helper 与两个用例：

```go
// capturingHost 记录最近一次 host.http.do 的 URL 与请求体。
type capturingHost struct {
	url  string
	body []byte
}

// installCapturingHost 回固定响应，并把请求 URL 与 body 记进 capturingHost。
func installCapturingHost(t *testing.T, responseBody string) *capturingHost {
	t.Helper()
	host := &capturingHost{}
	hostbridge.Configure(func(_, method string, payload any) (json.RawMessage, error) {
		if method != pluginabi.MethodHostHTTPDo {
			return nil, fmt.Errorf("test host: unsupported method %s", method)
		}
		raw, errMarshal := json.Marshal(payload)
		if errMarshal != nil {
			return nil, errMarshal
		}
		var req struct {
			URL  string `json:"url"`
			Body []byte `json:"body"`
		}
		if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
			return nil, errDecode
		}
		host.url = req.URL
		host.body = req.Body
		return json.Marshal(map[string]any{
			"StatusCode": 200,
			"Headers":    map[string][]string{"Content-Type": {"application/json"}},
			"Body":       []byte(responseBody),
		})
	})
	t.Cleanup(func() { hostbridge.Configure(nil) })
	return host
}

// TestFetchBalanceSendsProductCodeForCN 锁定国内服必须带国服产品码。
func TestFetchBalanceSendsProductCodeForCN(t *testing.T) {
	host := installCapturingHost(t, `{"code":0,"data":{"Response":{"Data":{"Accounts":[]}}}}`)
	client := NewClient(newRuntime(core.Env{}), Options{})
	if _, errFetch := client.FetchBalance(testTaskCredential("uid-cn-product-code")); errFetch != nil {
		t.Fatalf("fetch balance: %v", errFetch)
	}
	var body map[string]any
	if errUnmarshal := json.Unmarshal(host.body, &body); errUnmarshal != nil {
		t.Fatalf("decode request body %s: %v", host.body, errUnmarshal)
	}
	if body["ProductCode"] != "p_tcaca" {
		t.Fatalf("ProductCode = %v, want p_tcaca", body["ProductCode"])
	}
	if body["PageNumber"] != float64(1) || body["PageSize"] != float64(100) {
		t.Fatalf("paging = %v/%v, want 1/100", body["PageNumber"], body["PageSize"])
	}
	if statuses, ok := body["Status"].([]any); !ok || len(statuses) != 2 {
		t.Fatalf("Status = %v, want [0 3]", body["Status"])
	}
}

// TestFetchBalanceOmitsProductCodeForGlobal 锁定国际服不得带国服专用产品码：
// p_tcaca 在国际服不匹配，会静默返回空列表、面板显示 0 额度。
func TestFetchBalanceOmitsProductCodeForGlobal(t *testing.T) {
	host := installCapturingHost(t, `{"code":0,"data":{"Response":{"Data":{"Accounts":[]}}}}`)
	client := NewClient(newRuntime(core.Env{}), Options{})
	if _, errFetch := client.FetchBalance(testGlobalCredential("uid-global-product-code")); errFetch != nil {
		t.Fatalf("fetch balance: %v", errFetch)
	}
	var body map[string]any
	if errUnmarshal := json.Unmarshal(host.body, &body); errUnmarshal != nil {
		t.Fatalf("decode request body %s: %v", host.body, errUnmarshal)
	}
	if _, present := body["ProductCode"]; present {
		t.Fatalf("国际服请求体不得带 ProductCode，body = %s", host.body)
	}
	// 其余参数必须原样保留：只允许少一个键。
	if body["PageNumber"] != float64(1) || body["PageSize"] != float64(100) {
		t.Fatalf("paging = %v/%v, want 1/100", body["PageNumber"], body["PageSize"])
	}
	if statuses, ok := body["Status"].([]any); !ok || len(statuses) != 2 {
		t.Fatalf("Status = %v, want [0 3]", body["Status"])
	}
}
```

（`pluginabi`、`hostbridge`、`fmt`、`json` 在 `tasks_test.go` 已导入，无需新增 import。）

- [ ] **Step 2: 推送并验证红灯**

```
git add internal/vendors/workbuddy/tasks_test.go
git commit -m "test: 锁定国际服余额请求体不得带国服产品码（红灯）"
git push
```

预期 `go test` 失败，日志含 `国际服请求体不得带 ProductCode，body = {"PageNumber":1,"PageSize":100,"ProductCode":"p_tcaca",...}`。CN 用例此时应已通过。

- [ ] **Step 3: 实现**

`FetchBalance`：把请求体拆成「两域共用部分」与「CN 专属 `ProductCode`」。最直接的写法是先构造不含 `ProductCode` 的 map，再 `if !region.IsGlobal() { body["ProductCode"] = "p_tcaca" }`。**`PageNumber`、`PageSize`、`Status`、`PackageEndTimeRangeBegin/End` 保持字面量不动。** 同步更新函数上方那句「缺 ProductCode 会拿到空列表」的注释，改成「国服缺 ProductCode 会拿到空列表；国际服带国服码同样拿到空列表」。

- [ ] **Step 4: 推送并验证绿灯**

```
git add internal/vendors/workbuddy/tasks.go
git commit -m "fix: 国际服余额请求不再注入国服专用 ProductCode"
git push
gh run watch <run-id> --exit-status
```

- [ ] **Step 5: 提交**

已在 Step 4 完成。

---

## Task 5: 模型倍率以企业计费端点为准（Spec 要求 5，P0）

**Files:**
- Modify: `internal/vendors/workbuddy/models.go:501-507`（合并逻辑）
- Modify: `internal/vendors/workbuddy/models.go:391-401`（文件头合并策略注释）
- Modify: `internal/vendors/workbuddy/adapter.go:492-522`（`ConsoleModels` / `ModelCredits` 改用 `displayCredits`）
- Create: `internal/vendors/workbuddy/models_test.go`
- Test: `internal/vendors/workbuddy/models_test.go`

**Interfaces:**
- Consumes: `ModelInfo`（`models.go:35-64`）、`core.FormatRate`（`internal/core/capabilities.go:203`，只读，不改）
- Produces:
  - `mergeModelCatalogs(primary, fallback []ModelInfo) []ModelInfo` — 纯函数，无 I/O
  - `displayCredits(model ModelInfo) string` — 控制台展示用倍率文案
  - `fetchModelsFromUpstream` 以 `mergeModelCatalogs(primaryModels, fallbackModels)` 取代原 `dedupeModels` + 追加循环

### 规则钉死

1. 骨架：`dedupeModels(primary)` 保持不变（IDE 结果在前，字段权威）。
2. 企业端点中 **ID 不存在**于骨架 → 追加整条（与今天一致）。
3. ID 已存在时，**只处理 `Credits`**：`fallback.Credits` 为空则完全不覆盖；非空时，把两侧倍率都用 `core.FormatRate` 归一化后比较，**不相等才**用企业端点原串覆盖骨架的 `Credits`；相等则保持骨架原串。
   - **为何不照抄三选一的析取式：**「ID 带 `-free`」与「倍率冲突」两个分支在实现上等价——`-free` 模型若不冲突，覆盖是一次空操作。保留两个分支会产生永不改变结果的死代码。因此收敛为单一条件「归一化后不相等」。`-free` 的语义保证由测试钉死（`hy4flash-free` 从 `x0.03` 纠正为 `x0.00`）。
   - 用 `core.FormatRate` 归一化比较，可避免 `"x0.05"` 与 `"x0.05 credits"` 这种等价写法触发无意义覆盖。
4. **除 `Credits` 外的任何字段（`Name`、`Description`、`ContextWindow`、`MaxTokens`、`Efforts`、`DefaultEffort`、能力布尔、`IsDefault`）一律保留骨架值。**
5. `displayCredits`：返回 `core.FormatRate(model.Credits)`；若结果为空且 `model.ID == "auto"`，返回 `"浮动倍率"`；否则返回空串。
   - 放在 models.go 而非 adapter.go：它是 `ModelInfo` 的纯函数，且被控制台两条路径共用。
   - 不写进 `ModelInfo.Credits` 本体：`describeModel` 会把 `Credits` 拼成 `[xxx credit]` 前缀，`[浮动倍率 credit]` 是坏文案。

- [ ] **Step 1: 写失败测试**

新建 `internal/vendors/workbuddy/models_test.go`：

```go
package workbuddy

import "testing"

// TestMergeModelCatalogsCorrectsFreeModelCredits 锁定免费模型倍率纠偏：
// v3 目录把 hy4flash-free 标成 x0.03，企业计费端点给的才是真实扣费口径 x0.00。
func TestMergeModelCatalogsCorrectsFreeModelCredits(t *testing.T) {
	primary := []ModelInfo{{ID: "hy4flash-free", Name: "HY4 Flash Free", Credits: "x0.03"}}
	fallback := []ModelInfo{{ID: "hy4flash-free", Name: "企业侧名字", Credits: "x0.00"}}

	merged := mergeModelCatalogs(primary, fallback)
	if len(merged) != 1 {
		t.Fatalf("merged = %d, want 1（不得因覆盖倍率而新增条目）", len(merged))
	}
	if merged[0].Credits != "x0.00" {
		t.Fatalf("credits = %q, want x0.00（免费模型的真实倍率以企业端点为准）", merged[0].Credits)
	}
	if merged[0].Name != "HY4 Flash Free" {
		t.Fatalf("name = %q, want v3 骨架值（只允许改 Credits）", merged[0].Name)
	}
}

// TestMergeModelCatalogsFillsMissingCredits 锁定 v3 缺倍率时用企业端点补齐。
func TestMergeModelCatalogsFillsMissingCredits(t *testing.T) {
	primary := []ModelInfo{{ID: "default", Name: "Default"}}
	fallback := []ModelInfo{{ID: "default", Credits: "x2.20"}}

	merged := mergeModelCatalogs(primary, fallback)
	if len(merged) != 1 || merged[0].Credits != "x2.20" {
		t.Fatalf("merged = %+v, want single entry with x2.20", merged)
	}
}

// TestMergeModelCatalogsKeepsPrimaryWhenFallbackHasNoCredits 锁定「企业端点没给
// 明确倍率时不得清空 v3 值」：空倍率是「未知」，不是「免费」。
func TestMergeModelCatalogsKeepsPrimaryWhenFallbackHasNoCredits(t *testing.T) {
	primary := []ModelInfo{{ID: "glm-5.3", Name: "GLM", Credits: "x0.71"}}
	fallback := []ModelInfo{{ID: "glm-5.3", Name: "另一名字"}}

	merged := mergeModelCatalogs(primary, fallback)
	if merged[0].Credits != "x0.71" {
		t.Fatalf("credits = %q, want x0.71", merged[0].Credits)
	}
	if merged[0].Name != "GLM" {
		t.Fatalf("name = %q, want 骨架值", merged[0].Name)
	}
}

// TestMergeModelCatalogsAppendsFallbackOnlyIDs 锁定企业独有模型仍然补齐。
func TestMergeModelCatalogsAppendsFallbackOnlyIDs(t *testing.T) {
	primary := []ModelInfo{{ID: "glm-5.3"}}
	fallback := []ModelInfo{{ID: "deepseek-v4", Credits: "x0.10"}}

	merged := mergeModelCatalogs(primary, fallback)
	if len(merged) != 2 || merged[1].ID != "deepseek-v4" {
		t.Fatalf("merged = %+v, want appended deepseek-v4", merged)
	}
}

// TestMergeModelCatalogsTreatsEquivalentCreditsAsEqual 锁定归一化比较：
// "x0.05" 与 "x0.05 credits" 是同一倍率，不该触发无意义覆盖。
func TestMergeModelCatalogsTreatsEquivalentCreditsAsEqual(t *testing.T) {
	primary := []ModelInfo{{ID: "kimi-k3.1-pro", Credits: "x0.05"}}
	fallback := []ModelInfo{{ID: "kimi-k3.1-pro", Credits: "x0.05 credits"}}

	merged := mergeModelCatalogs(primary, fallback)
	if merged[0].Credits != "x0.05" {
		t.Fatalf("credits = %q, want 保留骨架原串 x0.05", merged[0].Credits)
	}
}

// TestDisplayCreditsMarksAutoAsFloating 锁定 auto 的空白倍率不再呈现为空白。
func TestDisplayCreditsMarksAutoAsFloating(t *testing.T) {
	if got := displayCredits(ModelInfo{ID: "auto"}); got != "浮动倍率" {
		t.Fatalf("auto credits = %q, want 浮动倍率", got)
	}
	if got := displayCredits(ModelInfo{ID: "auto", Credits: "x1.00"}); got != "1.00X" {
		t.Fatalf("auto with explicit credits = %q, want 1.00X（有值时不覆盖）", got)
	}
	if got := displayCredits(ModelInfo{ID: "glm-5.3", Credits: "x0.05"}); got != "0.05X" {
		t.Fatalf("numeric credits = %q, want 0.05X", got)
	}
	if got := displayCredits(ModelInfo{ID: "glm-5.3"}); got != "" {
		t.Fatalf("unknown credits = %q, want empty", got)
	}
}
```

- [ ] **Step 2: 推送并验证红灯**

```
git add internal/vendors/workbuddy/models_test.go
git commit -m "test: 锁定模型目录合并的倍率纠偏规则（红灯）"
git push
```

预期 `go vet` 或 `go test` 失败，日志含 `undefined: mergeModelCatalogs` 与 `undefined: displayCredits`（新文件引用了尚不存在的符号）。

- [ ] **Step 3: 实现**

1. `models.go` 新增 `mergeModelCatalogs(primary, fallback []ModelInfo) []ModelInfo`：内部先 `dedupeModels(primary)`，再遍历 `fallback`；对每个条目用**按下标遍历的循环**在 `merged` 中找同 ID 项，找到则按「规则钉死 3」就地回写 `merged[idx].Credits`，找不到则追加整条。
   - **不要复用 `findModel`**：它返回 `ModelInfo` 值拷贝（`models.go:712-719`），改它不会影响切片；必须自己按下标定位。若嫌循环嵌套可维护性差，可新增 `modelIndex(models []ModelInfo, id string) int`（找不到返回 -1），但仍不得改动 `findModel` 的既有签名与调用方。
2. `models.go` 新增 `displayCredits(model ModelInfo) string`。
3. `fetchModelsFromUpstream`：把 501-507 行整段替换为 `merged := mergeModelCatalogs(primaryModels, fallbackModels)`。
4. 更新 `models.go:391-401` 的文件头注释：合并策略从「企业端点只补缺失 id」改为「v3 骨架 + 企业端点纠正倍率」。
5. `adapter.go`：`ConsoleModels` 的 `Credits: core.FormatRate(model.Credits)` 改为 `Credits: displayCredits(model)`；`ModelCredits` 的 `if rate := core.FormatRate(model.Credits)` 改为 `if rate := displayCredits(model)`。

- [ ] **Step 4: 推送并验证绿灯**

```
git add internal/vendors/workbuddy/models.go internal/vendors/workbuddy/adapter.go
git commit -m "fix: 模型倍率以企业计费端点为准，auto 显示浮动倍率"
git push
gh run watch <run-id> --exit-status
```

预期六步全绿；`internal/admin` 的 `TestConsoleModelTableMergesIDAndCredits`、`catalog_test.go` 倍率用例不受影响（`FormatRate` 未改）。

- [ ] **Step 5: 提交**

已在 Step 4 完成。

---

## Task 6: 探测 UA 现代化（Spec 要求 6）

**Files:**
- Modify: `internal/vendors/workbuddy/chat.go:560-565`（`codeBuddyCLIUA`）
- Test: `internal/vendors/workbuddy/chat_headers_test.go`（追加；需新增 `encoding/json`、`fmt`、`freetier2api-plugin/cpasdk/pluginabi`、`freetier2api-plugin/internal/hostbridge` 四个 import）

**Interfaces:**
- Consumes: `defaultCLIVersion`（`chat.go:555`，常量 `"2.179.0"`）、`probeV3Config`（`models.go:533`）、`applyV3ConfigHeaders`（`chat.go:879`）
- Produces: `codeBuddyCLIUA` 常量值变为 `CLI/2.179.0 CodeBuddy/2.179.0`，并仍然是 `models.go:469` 那一路探测实际发出的 UA

### 裁定：复用 `defaultCLIVersion` 拼接，不写独立字面量

```go
codeBuddyCLIUA = "CLI/" + defaultCLIVersion + " CodeBuddy/" + defaultCLIVersion
```

理由：
1. 现有代码第 551-552 行已声明 `defaultCLIVersion` 与正版 5.8.3 的 `CLI/2.179.0` 对齐——探测 UA 过时的根因正是「同一个版本号存在两份字面量」。拼接从结构上消灭这类漂移。
2. Go 常量表达式允许在同组 const 内引用先声明的常量，无运行期开销。
3. 该常量仅被 `models.go:469` 的 global 探测使用，与 `endpointsGlobal.CLIVersion`（`2.161.0`，对话 UA 用）**不是同一个语义**。这层区别必须写进注释，否则后人会误以为可以合并。

反方意见（已考虑并否决）：写死字面量更贴合 Spec 原文，但会让下一个人工升版时重犯同一个错误，且与第 552 行注释形成第二份事实源。

- [ ] **Step 1: 写失败测试**

在 `chat_headers_test.go` 追加：

```go
// TestProbeCLIUserAgentIsModern 锁定模型探测 UA 必须是当前官方 CLI 版本。
// 上游已废弃 2.63.x 的探测请求，旧 UA 会让 /v3/config 静默拿到精简目录。
func TestProbeCLIUserAgentIsModern(t *testing.T) {
	if codeBuddyCLIUA != "CLI/2.179.0 CodeBuddy/2.179.0" {
		t.Fatalf("codeBuddyCLIUA = %q, want CLI/2.179.0 CodeBuddy/2.179.0", codeBuddyCLIUA)
	}
}

// TestFetchModelsFromUpstreamGlobalProbesWithModernCLIUA 锁定接线：
// 只改常量而没有任何探测路径真的发它，是这类 bug 的另一种形态。
func TestFetchModelsFromUpstreamGlobalProbesWithModernCLIUA(t *testing.T) {
	// fetchModelsFromUpstream 是三路并发的，抓包必须加锁。
	var (
		uaMu       sync.Mutex
		userAgents []string
	)
	hostbridge.Configure(func(_, method string, payload any) (json.RawMessage, error) {
		if method != pluginabi.MethodHostHTTPDo {
			return nil, fmt.Errorf("probe host: unsupported method %s", method)
		}
		raw, errMarshal := json.Marshal(payload)
		if errMarshal != nil {
			return nil, errMarshal
		}
		var req struct {
			Headers map[string][]string `json:"headers"`
		}
		if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
			return nil, errDecode
		}
		uaMu.Lock()
		if values := req.Headers["User-Agent"]; len(values) > 0 {
			userAgents = append(userAgents, values[0])
		}
		uaMu.Unlock()
		return json.Marshal(map[string]any{
			"StatusCode": 200,
			"Headers":    map[string][]string{"Content-Type": {"application/json"}},
			"Body":       []byte(`{"code":0,"data":{"models":[]}}`),
		})
	})
	t.Cleanup(func() { hostbridge.Configure(nil) })

	client := NewClient(newRuntime(core.Env{}), Options{})
	// 三路探测都回空清单，函数必然返回 "no models returned"：本用例只查 UA。
	_, _ = client.fetchModelsFromUpstream(RegionGlobal)

	uaMu.Lock()
	captured := append([]string(nil), userAgents...)
	uaMu.Unlock()

	var sawCLI bool
	for _, ua := range captured {
		if ua == codeBuddyCLIUA {
			sawCLI = true
		}
		if strings.Contains(ua, "2.63.2") {
			t.Fatalf("探测请求仍在用已废弃的 2.63.x UA: %q", ua)
		}
	}
	if !sawCLI {
		t.Fatalf("没有任何探测请求携带 codeBuddyCLIUA（实际发出 %v）", captured)
	}
}
```

两个用例**修复前都是红的**：第一个因常量值不符；第二个因实际发出的 UA 里含 `2.63.2`、且没有任何请求等于新常量。这不是重复——第一个锁值，第二个锁「被真的发出去」。

（`chat_headers_test.go` 现有 import 需补 `encoding/json`、`fmt`、`sync`、`freetier2api-plugin/cpasdk/pluginabi`、`freetier2api-plugin/internal/hostbridge`；`strings` 已存在。）

- [ ] **Step 2: 推送并验证红灯**

```
git add internal/vendors/workbuddy/chat_headers_test.go
git commit -m "test: 锁定模型探测 UA 为当前 CLI 版本（红灯）"
git push
```

预期 `go test` 失败，日志同时含：
- `codeBuddyCLIUA = "CLI/2.63.2 CodeBuddy/2.63.2", want CLI/2.179.0 CodeBuddy/2.179.0`
- `探测请求仍在用已废弃的 2.63.x UA: "CLI/2.63.2 CodeBuddy/2.63.2"`

- [ ] **Step 3: 实现**

`chat.go:565` 改为 `codeBuddyCLIUA = "CLI/" + defaultCLIVersion + " CodeBuddy/" + defaultCLIVersion`。在该 const 块的注释里补两句：探测 UA 复用 `defaultCLIVersion` 以免二次漂移；它与 `endpointsGlobal.CLIVersion`（对话 UA 的 2.161.0）语义不同，不可合并。

- [ ] **Step 4: 推送并验证绿灯 + 全量回归验收**

```
git add internal/vendors/workbuddy/chat.go
git commit -m "fix: 模型探测 UA 升至 2.179.0，修复国际服 /v3/config 静默失效"
git push
gh run watch <run-id> --exit-status
```

预期：`gofmt`、`go vet`、`go test ./... -count=1 -timeout 300s`、`race tests`、`编译 live 探针`、`校验 c-shared 编译` 六步全绿，退出码 0。这一步同时是 Spec §五.2「全量通过」的验收落点——`internal/vendors/workbuddy` 的全部用例（含本方案新增的 16 个：Task 1 的 2 个、Task 2 的 2 个、Task 3 的 2 个、Task 4 的 2 个、Task 5 的 6 个、Task 6 的 2 个）都已随全量 `go test ./...` 跑过。

**注意：** `ci.yml` 的 race 步骤只覆盖 `internal/core`、`internal/state`、`internal/auth`、`internal/tasks`、`internal/admin`，不含 `internal/vendors/workbuddy`。本方案不新增并发路径（`mergeModelCatalogs` / `displayCredits` 均为纯函数），因此无需为 race 额外开子集；不要为此修改 `ci.yml`。

**符号重名核查：** 本方案新增的 16 个测试函数名与 6 个包级符号（`mergeModelCatalogs`、`displayCredits`、`quotaGroupDisplayName`、`quotaGroupRank`、`capturingHost`、`installCapturingHost`）已全仓扫描，与现有 78 个测试函数及全部包级符号均无重名。`quotaGroup`（无后缀）已被 `internal/vendors/qoder/tasks.go:432` 占用，但那在另一个包内，不构成冲突——本方案仍刻意命名为 `quotaGroupDisplayName`，避免日后读代码时误认。

**并发注意：** Task 6 第二个用例的抓包闭包会被 `fetchModelsFromUpstream` 的三路 goroutine 并发调用，必须用 `sync.Mutex` 保护 `userAgents`。虽然 `workbuddy` 包不在 CI 的 `-race` 子集里，但 `git diff` 审查时会看这一点，且日后若加进 race 子集不该直接爆。

- [ ] **Step 5: 提交**

已在 Step 4 完成。确认 `git status --porcelain` 为空。

---

## 范围边界与非目标

**本次必须做（Spec 七项要求覆盖表）：**

| Spec 要求 | 落实位置 |
| --- | --- |
| 1. Status 过滤死包 | Task 1 |
| 2. 500 点基础包回退 | Task 2 |
| 3. 按 SubProductCode 分桶 | Task 3 |
| 4. 国际服产品码自适应 | Task 4（分页部分见下方裁定） |
| 5. 模型倍率合并纠偏 | Task 5 |
| 6. 探测 UA 现代化 | Task 6 |
| 7. 测试覆盖（含新建 `models_test.go`） | 每个 Task 的 Step 1；`models_test.go` 由 Task 5 创建 |

**明确不做（Out of scope）：**

1. **余额接口翻页**——见 Task 4 的裁定。不加循环、不加 `TotalCount` 语义推断。
2. **不改 `cpasdk/`**（宿主 ABI 契约冻结）、**不改 `internal/core/`**（`FormatRate` 已满足需求）、**不改 `internal/admin/`**。
3. **不动其他供应商**（Qoder / Trae / Cline / CodeArts / MiniMax 等）。
4. **不改 `ci.yml`**、不改 `go.mod`、不引入第三方依赖。
5. **不新增界面控件**：本次不新增按钮、不新增控制台页面元素。唯一可见变化是额度卡片里的分组名（`基础月度套餐` / `活动与签到奖励` / `资源包明细`）与模型表的倍率文案（`0.00X`、`2.20X`、`浮动倍率`）。
6. **不改 `login.go:267` 的 `loginClientUA`**（同样是 `2.63.2` 字面量）：登录链路不在 Spec 范围内，且无任何证据表明它失效。若后续登录报错再单独立项。
7. **不做模型倍率的持久化迁移**：`ModelCredits` 的语义不变，只是取值来源多了一个 `auto` 兜底；已落盘的旧倍率表会在下一次探测成功后整表替换。

**落盘目录：** 本方案唯一写盘产物是 `docs/plan-workbuddy-quota-and-models.md`。不生成任何运行期文件。

---

## 证据缺口与风险清单

| 编号 | 风险 | 证据状态 | 缓解措施 | 若失效如何处置 |
| --- | --- | --- | --- | --- |
| R1 | `Status` 字段名无法自证：Spec 声称上游下发 `Status`，但全仓无样本；请求参数 `"Status": []int{0, 3}` 只暗示上游能返回 Status 3 的包，不能证明字段名 | **不可证** | 零值语义选为「生效中」，字段缺失时全量计入（`TestParseBalanceKeepsAccountsWithoutStatus`）；过滤只剔除明确非 0 | 真实响应回来后若字段名不同，改 `json` tag 一处即可，测试夹具同步换真样本 |
| R2 | `SubProductCode` 字段名同样无法自证；两个产品码字面量只在 Spec 里出现 | **不可证** | 未知码归入 `资源包明细`，与今天行为完全一致；`TestBuildQuotaResponsePersonalDescription` 原样通过即是证据 | 若字段名不同，改 tag + `quotaGroupRank` 一处；分组退化为单组，不会丢额度 |
| R3 | Task 2 的回退规则可能复活幽灵额度：若某包 Cycle 账因上游缺陷清零、而 Capacity 账仍是历史满额，会被算成满额 | 理论风险，无样本 | 回退要求 `CycleCapacityRemain <= 0` **且** `CapacityRemain > 0` **且** `CapacitySize == CycleCapacitySize` **且** `CapacityUsed == 0 \|\| CycleCapacityUsed >= CycleCapacitySize`；`TestPackageRemainUsedKeepsExhaustedCyclePackAtZero` 锁死反向边界，`TestPackageRemainUsedRejectsNonBasePackWithoutMatchingSize` 锁死「两账 size 不等不得回退」 | **不可约残留**：同为 500/500 的周转包若 Capacity 账从不递减，与基础包数值完全同形，任何算术判据都无法区分，只能靠产品码或真实样本收敛。若线上出现新虚增，改为按 `SubProductCode` 白名单收窄 |
| R4 | Task 1 的红灯预期值 7493 与明细拆分（1000 / 5489 / 1004）是本方案构造的夹具，不是仓库内真实样本；Spec 只报告了「总额 7493、有效 2004、死包虚增 5489」三个数 | **不可证** | 夹具的三段形态与 Spec 的数量关系一致（7493 − 5489 = 2004）；断言值随夹具确定，验证的是过滤规则而非真实数据 | 无 |
| R5 | 若某账号资源包真的超过 100 条（`PageSize: 100` 截断），本次修复不覆盖，欠额仍会发生 | 理论风险 | 本次不做（见 Task 4 裁定） | 出现「额度少于预期且包数恰为 100」时立独立工单 |
| R6 | 企业端点若对 `-free` 模型也不下发 `Credits`，纠偏不生效 | 理论风险 | 规则 3 明确「空倍率不覆盖」，不做猜测 | 需真实企业端点响应后再定策略 |
| R7 | `hy4flash-free` / `default` 两个模型名只在 Spec 里出现，仓库内无同名 fixture | **不可证** | 夹具用 Spec 给定的 ID 与倍率原串；测试只验证**规则**，不依赖模型是否真实存在 | 无 |
| R8 | `displayCredits` 把 `浮动倍率` 写入倍率表后，会随 `state.json` 落盘 | 已知副作用 | 该表每轮探测整表替换；`core.FormatRate` 对非数值文案原样透传（`internal/core/capabilities.go:207-222`，已有单测覆盖） | 若控制台排序异常（`modelCreditValue` 解析非数值下沉），属既有排序行为，不改 |
| R9 | 本机无 Go 工具链，`gofmt` 只能由 CI 把关，格式失败会消耗一轮 run | 已确认（无 `go.exe` / 无 Docker / WSL 停机） | 每步严格用 tab 缩进、对齐照抄邻近代码 | 按 CI 输出的 `gofmt` 差异逐行修正后重推 |
