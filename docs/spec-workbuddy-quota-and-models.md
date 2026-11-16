# 设计规格说明书 (Spec)：WorkBuddy 模块配额对齐与模型计费倍率纠偏

- **创建日期**：2026-11-16
- **项目路径**：`D:\cliproxyapi\CLIProxyAPI-zhenghefreellm2api-plugin-main`
- **目标组件**：`internal/vendors/workbuddy`、`internal/core`
- **状态**：待审批 / 准备派发实施

---

## 一、背景与目标

`CLIProxyAPI-freetier2api-plugin`（FreeTier 2API）的 `workbuddy` 模块在接入 WorkBuddy 国内版（`workbuddycn`）与国际版（`workbuddyglobal`）时，存在严重的配额计算错误与模型计费倍率脱节问题。

本 Spec 的目标是：
1. **配额彻底对齐**：修复把已过期历史死包计入总额导致的数值虚增、修复月度免费基础包（500 点）因字段冲突被误判为 0 的 Bug，实现精准的剩余/总额/已用计算与分类展示。
2. **模型倍率对齐**：纠正多端点探测合并策略，防止免费模型（如 `hy4flash-free`）被误标为收费模型、补齐高倍率模型（如 `default 2.20X`），并更新失效的探测 UA，确保国服与国际服计费倍率真实反映上游扣费口径。

---

## 二、问题清单与根因实证 (Defects & Evidence)

### 1. 配额虚增：已过期死包（Status 3）被累加进总额度
* **问题现象**：面板显示的“总额度”远大于实际有效额度，已用积分高达数千点。
* **物理代码**：`internal/vendors/workbuddy/tasks.go` 的 `FetchBalance` 与 `parseBalance`（2884 行）。
* **根因分析**：请求带 `"Status": []int{0, 3}` 查回了所有生效（0）和已失效（3）的资源包。`parseBalance` 循环中**没有任何状态过滤**，无条件执行 `balance.Total += float64(size)`，导致历史上所有已过期的包被强行累加。
* **真实实证**：真实国服账号返回 69 个包（25 个 Status 0，44 个 Status 3）。真实有效总额为 2,004 点（剩余 1,447 点），插件算成总额 7,493 点、已用 6,046 点（多算了 5,489 点死包容量）。

### 2. 免费基础包吞没：Cycle 字段冲突导致 500 点基础额度算为 0
* **问题现象**：国服每月的 500 点免费基础额度未计入剩余额度中。
* **物理代码**：`internal/vendors/workbuddy/tasks.go` 的 `packageRemainUsed`（2790 行）。
* **根因分析**：代码约定“只要 `CycleCapacitySize > 0` 就强制只认 `CycleCapacityRemain`”。而在上游真实下发的月度基础包（`sp_tcaca_codebuddy_ide`）中：
  - `CapacitySize: 500`, `CapacityRemain: 500`, `CapacityUsed: 0`
  - `CycleCapacitySize: 500`, `CycleCapacityRemain: 0`, `CycleCapacityUsed: 500`
  代码强行取了 `CycleCapacityRemain: 0`，直接把未动过的 500 点免费基础额度判定为“剩余 0、已用 500”。

### 3. 配额分类粗暴：免费奖励包与基础套餐混为一体
* **问题现象**：用户无法直观分辨哪些是每日赠送的短期积分，哪些是月度基础权益或长期资源包。
* **根因分析**：上游明确有 `SubProductCode`（`sp_tcaca_codebuddyide_bonus_pack` 每日/任务奖励 vs `sp_tcaca_codebuddy_ide` 基础套餐），插件将其扁平累加为一个单值。

### 4. 国际服请求体参数硬编码与分页截断
* **问题现象**：国际服账号可能查出 0 额度，活跃账号长期使用后出现新包被截断。
* **根因分析**：`FetchBalance` 硬编码 `"ProductCode": "p_tcaca"`（国服专用码，国际服不匹配）；同时硬编码 `"PageSize": 100, "PageNumber": 1` 无翻页。

### 5. 模型计费倍率多端点合并策略倒挂
* **问题现象**：免费模型显示为收费，高倍率模型显示为空。
* **物理代码**：`internal/vendors/workbuddy/models.go` 的 `fetchModelsFromUpstream`（501 行）。
* **根因分析**：合并时写死“v3 目录优先，企业端点只补缺失 ID”。但实测发现：
  - `hy4flash-free` 在 v3 中返回 `credits: "x0.03"`，而在实际扣费的企业端点中为 `credits: "x0.00"`（真实免费！）；
  - `default` 模型在 v3 中 `credits` 为 null，而在企业端点中真实记录为 `x2.20 credits`。
  由于 v3 优先且不可覆盖，导致错误倍率覆盖了真实计费倍率。

### 6. 国际服探测 UA 严重过时
* **物理代码**：`internal/vendors/workbuddy/chat.go` 第 565 行 `codeBuddyCLIUA = "CLI/2.63.2 CodeBuddy/2.63.2"`。
* **根因分析**：上游已废弃 `2.63.x` 版本的探测请求。国际服 CLI 最新版本已至 `2.161.0+`，使用旧 UA 导致国际服 `/v3/config` 探测静默失效。

---

## 三、详细技术方案 (Technical Specification)

### 1. 配额计算与资源包过滤 (`tasks.go`)
1. **严格状态过滤**：
   在 `parseBalance` 遍历 `payload.Response.Data.Accounts` 时，增加前置状态检查：
   ```go
   if account.Status != 0 {
       continue // 仅统计 Status == 0 的生效中资源包，剔除 Status 3 等历史过期包
   }
   ```
2. **余额提取双重兼容 (`packageRemainUsed`)**：
   重构单包提取逻辑：当 `CycleCapacitySize > 0` 时，若 `CycleCapacityRemain > 0`，正常使用；若 `CycleCapacityRemain == 0` 但 `CapacityRemain > 0` 且（`CapacityUsed == 0` 或 `CycleCapacityUsed == account.CycleCapacitySize`），判定该包为非周转性基础额度，回退采信 `CapacityRemain`，避免吞噬 500 点基础包。
3. **国际服产品码自适应**：
   在 `FetchBalance` 中，若 `region.IsGlobal()`，不在请求体中强制注入 `"ProductCode": "p_tcaca"`，避免国际服因产品码不匹配返回空列表。
4. **配额多桶展示 (`BuildQuotaResponse`)**：
   在 `QuotaFetchResponse.Groups` 中，将 `sp_tcaca_codebuddyide_bonus_pack` 标注为「活动与签到奖励」，将 `sp_tcaca_codebuddy_ide` 标注为「基础月度套餐」，提供清晰的结构化分类。

### 2. 模型计费倍率与目录合并 (`models.go`)
1. **企业计费端点倍率优先纠偏**：
   在 `fetchModelsFromUpstream` 中，合并 `primaryModels`（v3 目录）与 `fallbackModels`（企业模型目录）时：
   * 基础结构以 v3 为骨架；
   * 若企业端点给出了明确的 `Credits` 且该模型名称/ID 带有 `-free` 标识或 v3 的 `Credits` 缺失/与企业端点冲突，以企业端点的真实计费倍率为准进行就地更新（确保 `hy4flash-free` 修正为 `0.00X`，`default` 补齐 `2.20X`）。
2. **Auto 路由模型标记**：
   对于 `auto` 模型，若 `Credits` 为空，保留展示并赋予说明文本 `浮动倍率`，避免呈现为空白导致误解。

### 3. 探测 UA 现代化对齐 (`chat.go`)
1. 将 `codeBuddyCLIUA` 更新为当前活跃的现代版本字符串：
   ```go
   codeBuddyCLIUA = "CLI/2.179.0 CodeBuddy/2.179.0"
   ```
   确保国际版 `/v3/config` 探测正常通过上游版本校验。

---

## 四、范围边界与非目标 (Scope & Non-Goals)

1. **底层假定**：
   - 假定 1：WorkBuddy 计费系统中 `Status: 0` 代表全部当前可消费资产，`Status: 3` 为已清零/归档资产。
   - 假定 2：企业租户如果配置了 `EnterpriseID`，继续遵循既有的企业用量接口回退链路不变。
2. **明确非目标 (Out of Scope)**：
   - 本次绝对不变更 CPA 核心的路由与网络层逻辑。
   - 本次绝对不修改其他供应商（Qoder、Trae、Cline 等）的现有逻辑。
   - 本次不自制本地独立的图表报表控件，仅对齐标准 `pluginapi.QuotaFetchResponse` 与 `ConsoleModel` 结构。

---

## 五、验收标准 (Acceptance Criteria)

1. **单元测试与回归**：
   - 在 `tasks_test.go` 中新增/更新测试用例：
     - 测试包含 Status 0 与 Status 3 混合的响应，断言总额度仅统计 Status 0，杜绝虚增；
     - 测试包含 `CycleCapacityRemain: 0` 且 `CapacityRemain: 500` 的月度基础包，断言正确识别出 500 点可用额度。
   - 在 `models_test.go` 中验证模型合并时 `hy4flash-free` 正确被修正为免费/0 倍率。
2. **全量通过**：
   - `internal/vendors/workbuddy` 下单测 100% 跑绿（Exit Code 0）。
