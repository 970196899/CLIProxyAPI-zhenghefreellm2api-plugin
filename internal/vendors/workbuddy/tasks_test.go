package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"freetier2api-plugin/cpasdk/pluginabi"
	"freetier2api-plugin/internal/core"
	"freetier2api-plugin/internal/hostbridge"
)

// 本文件锁定每日活跃奖励（国际版签到）的关键判据。
//
// 信标与热力图解析都是「错了不报错、只是静默不入账」的链路：
// 事件字段名写错上游返回 200 但丢事件，热力图形态解析错会把未点亮
// 误判成已点亮。因此用单测把字段名与两种响应形态锁住。

func testGlobalCredential(uid string) *Credential {
	credential := &Credential{
		AccessToken:  "at",
		RefreshToken: "rt",
		UID:          uid,
		Nickname:     "global tester",
		Domain:       "www.workbuddy.ai",
	}
	credential.SetRealm(RegionGlobal)
	return credential
}

// TestParseGrowthHeatmapCells 锁定 cells 数组形态。
func TestParseGrowthHeatmapCells(t *testing.T) {
	data := json.RawMessage(`{"cells":[
		{"date":"2026-10-16","score":0},
		{"date":"2026-10-17","score":30},
		{"date":"2026-10-18","score":0}
	]}`)
	today, lit, err := parseGrowthHeatmap(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if today.Date != "2026-10-18" || today.Score != 0 {
		t.Fatalf("today = %+v, want last cell 2026-10-18/0", today)
	}
	if len(lit) != 1 || lit[0] != "2026-10-17" {
		t.Fatalf("lit = %v, want [2026-10-17]", lit)
	}
}

// TestParseGrowthHeatmapByDate 锁定日期为键的 map 形态。
func TestParseGrowthHeatmapByDate(t *testing.T) {
	data := json.RawMessage(`{"2026-10-16":{"score":0},"2026-10-18":{"score":5},"2026-10-17":{"score":30}}`)
	today, lit, err := parseGrowthHeatmap(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if today.Date != "2026-10-18" || today.Score != 5 {
		t.Fatalf("today = %+v, want 2026-10-18/5", today)
	}
	if len(lit) != 2 || lit[0] != "2026-10-17" || lit[1] != "2026-10-18" {
		t.Fatalf("lit = %v, want sorted [2026-10-17 2026-10-18]", lit)
	}
}

// TestParseGrowthHeatmapEmpty 空热力图必须报错，不能误判成「未点亮」。
func TestParseGrowthHeatmapEmpty(t *testing.T) {
	if _, _, err := parseGrowthHeatmap(json.RawMessage(`{}`)); err == nil {
		t.Fatal("empty heatmap must error")
	}
	if _, _, err := parseGrowthHeatmap(json.RawMessage(`{"cells":[]}`)); err == nil {
		t.Fatal("empty cells must error")
	}
}

// TestParseClaimResultRejectsMalformedPayloads 领奖响应解析失败必须显式报错，
// 不能把 malformed JSON/HTML 静默当成 0 奖励成功。
func TestParseClaimResultRejectsMalformedPayloads(t *testing.T) {
	for _, raw := range []string{
		`{`,
		`<html><body>upstream error</body></html>`,
		`{"unexpected":true}`,
		`null`,
	} {
		t.Run(raw, func(t *testing.T) {
			credit, energy, errParse := parseClaimResult(json.RawMessage(raw))
			if errParse == nil {
				t.Fatalf("parseClaimResult(%q) returned nil error with %d/%d reward", raw, credit, energy)
			}
			if credit != 0 || energy != 0 {
				t.Fatalf("malformed claim result returned reward %d/%d", credit, energy)
			}
		})
	}
}

func TestParseClaimResult(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		raw          string
		wantCredit   int64
		wantEnergy   int64
		wantNilError bool
	}{
		{"reward", `{"credit":12,"energy":3}`, 12, 3, true},
		{"already claimed", `{"already_claimed":true}`, 0, 0, true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			credit, energy, errParse := parseClaimResult(json.RawMessage(testCase.raw))
			if (errParse == nil) != testCase.wantNilError {
				t.Fatalf("error = %v, want nil=%v", errParse, testCase.wantNilError)
			}
			if credit != testCase.wantCredit || energy != testCase.wantEnergy {
				t.Fatalf("reward = %d/%d, want %d/%d", credit, energy, testCase.wantCredit, testCase.wantEnergy)
			}
		})
	}
}

// TestTaskSpecsForGlobalRegistersDailyBonus 国际版必须注册每日活跃奖励族。
func TestTaskSpecsForGlobalRegistersDailyBonus(t *testing.T) {
	specs := TaskSpecsFor(RegionGlobal)
	if len(specs) != 1 {
		t.Fatalf("global specs = %d, want 1", len(specs))
	}
	spec := specs[0]
	if spec.Kind != TaskKindDailyBonus || spec.Route != "/dailybonus" || !spec.Manual {
		t.Fatalf("spec = %+v", spec)
	}
	if len(spec.DefaultHours) != len(globalDailyBonusHours) {
		t.Fatalf("default hours = %v, want %v", spec.DefaultHours, globalDailyBonusHours)
	}
	// CN 不注册该族（CN 走 checkin 族）。
	for _, cnSpec := range TaskSpecsFor(RegionCN) {
		if cnSpec.Kind == TaskKindDailyBonus {
			t.Fatal("CN must not register the daily bonus family")
		}
	}
}

// testTaskCredential 构造一个测试凭证。
func testTaskCredential(uid string) *Credential {
	credential := &Credential{
		AccessToken:  "at",
		RefreshToken: "rt",
		UID:          uid,
		Nickname:     "测试账号",
		Domain:       "www.codebuddy.cn",
	}
	credential.SetRealm(RegionCN)
	return credential
}

// TestInNightWindow 验证夜猫子窗口判定。
//
// 窗口外跑夜猫子任务既消耗额度又不计分，因此边界必须准确。
func TestInNightWindow(t *testing.T) {
	cases := []struct {
		hour int
		want bool
	}{
		{0, true}, {7, true}, {8, false}, {12, false},
		{22, false}, {23, true},
	}
	for _, testCase := range cases {
		now := time.Date(2026, 9, 26, testCase.hour, 30, 0, 0, time.Local)
		if got := InNightWindow(now); got != testCase.want {
			t.Fatalf("InNightWindow(%02d:30) = %v, want %v", testCase.hour, got, testCase.want)
		}
	}
}

// TestParseBalanceMatchesUpstreamShape 验证余额解析对齐上游的真实契约。
//
// 上游该接口用 PascalCase 字段、双层信封（Response.Data.Accounts），
// 且 Cycle 期套餐优先。早期实现按 camelCase 解析，结果恒为 0。
func TestParseBalanceMatchesUpstreamShape(t *testing.T) {
	raw := `{"Response":{"Data":{"Accounts":[
	  {"PackageName":"基础套餐","CycleCapacitySize":1000,"CycleCapacityRemain":400,"CycleCapacityUsed":600},
	  {"PackageName":"加油包","CapacitySize":500,"CapacityRemain":500}
	]}}}`
	var payload billingResourceResponse
	if errUnmarshal := json.Unmarshal([]byte(raw), &payload); errUnmarshal != nil {
		t.Fatalf("unmarshal: %v", errUnmarshal)
	}
	balance := parseBalance(payload)

	// Cycle 套餐：1000 总量、400 剩余；加油包：500/500。合计 1500 / 900。
	if balance.Total != 1500 {
		t.Fatalf("total = %v, want 1500", balance.Total)
	}
	if balance.Remain != 900 {
		t.Fatalf("remain = %v, want 900", balance.Remain)
	}
	if len(balance.Packages) != 2 {
		t.Fatalf("packages = %d, want 2", len(balance.Packages))
	}
	if balance.Packages[0].Name != "基础套餐" || balance.Packages[0].Remain != 400 {
		t.Fatalf("first package = %+v", balance.Packages[0])
	}
}

// TestPackageRemainUsedCycleWins 验证 Cycle 期套餐的取数规则。
func TestPackageRemainUsedCycleWins(t *testing.T) {
	// CycleUsed 大于 size-remain 时，应以 CycleUsed 为准反推 remain。
	remain, used, size := packageRemainUsed(billingResourceAccount{
		CycleCapacitySize: 1000, CycleCapacityRemain: 900, CycleCapacityUsed: 300,
	})
	if size != 1000 || used != 300 || remain != 700 {
		t.Fatalf("cycle rule: remain=%d used=%d size=%d, want 700/300/1000", remain, used, size)
	}
	// 无 Cycle 期时回落 Capacity 三字段。
	remain, used, size = packageRemainUsed(billingResourceAccount{
		CapacitySize: 500, CapacityRemain: 200,
	})
	if size != 500 || used != 300 || remain != 200 {
		t.Fatalf("capacity fallback: remain=%d used=%d size=%d, want 200/300/500", remain, used, size)
	}
	// remain 超出 size 时钳到 size（上游偶发脏数据）。
	remain, _, size = packageRemainUsed(billingResourceAccount{
		CycleCapacitySize: 100, CycleCapacityRemain: 999,
	})
	if remain != 100 || size != 100 {
		t.Fatalf("clamp: remain=%d size=%d, want 100/100", remain, size)
	}
}

// claimHost 是只回一种上游响应的假宿主：任务列表 + 领奖。
type claimHost struct {
	taskBody  string
	claimBody string
}

// installHostHTTP 用 responder 应答所有 host.http.do 调用，并在用例结束时复位。
func installHostHTTP(t *testing.T, responder func(method, rawURL string) (status int, body string)) {
	t.Helper()
	hostbridge.Configure(func(_ string, method string, payload any) (json.RawMessage, error) {
		if method != pluginabi.MethodHostHTTPDo {
			return nil, fmt.Errorf("test host: unsupported method %s", method)
		}
		raw, errMarshal := json.Marshal(payload)
		if errMarshal != nil {
			return nil, errMarshal
		}
		var req struct {
			Method string `json:"method"`
			URL    string `json:"url"`
		}
		if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
			return nil, errDecode
		}
		status, body := responder(req.Method, req.URL)
		return json.Marshal(map[string]any{
			"StatusCode": status,
			"Headers":    map[string][]string{"Content-Type": {"application/json"}},
			"Body":       []byte(body),
		})
	})
	t.Cleanup(func() { hostbridge.Configure(nil) })
}

func installClaimHost(t *testing.T, host claimHost) {
	t.Helper()
	installHostHTTP(t, func(_, rawURL string) (int, string) {
		if strings.Contains(rawURL, "/claim") {
			return http.StatusOK, host.claimBody
		}
		return http.StatusOK, host.taskBody
	})
}

// TestRunGrowthTaskSurfacesClaimFailure 锁定领奖解析失败必须变成任务的错误结果。
func TestRunGrowthTaskSurfacesClaimFailure(t *testing.T) {
	// 任务已达标 → runGrowthTask 直接走领奖分支，不执行自动动作。
	installClaimHost(t, claimHost{
		taskBody:  `{"code":0,"data":[{"task_code":"chat_5","progress":{"current":5,"target":5}}]}`,
		claimBody: `<html><body>upstream error</body></html>`,
	})
	client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
	cred := testGlobalCredential("uid-claim")
	cred.SetRealm(RegionCN)

	_, _, _, errRun := runGrowthTask(context.Background(), client, cred, "chat_5")
	if errRun == nil {
		t.Fatal("领奖响应不可解析时必须报错，不能报成「任务已达标」")
	}
	if !strings.Contains(errRun.Error(), "领奖失败") {
		t.Fatalf("error = %v, want 领奖失败", errRun)
	}
}

// TestRunGrowthTaskReportsClaimedReward 锁定正常路径不受影响。
func TestRunGrowthTaskReportsClaimedReward(t *testing.T) {
	installClaimHost(t, claimHost{
		taskBody:  `{"code":0,"data":[{"task_code":"chat_5","progress":{"current":5,"target":5}}]}`,
		claimBody: `{"code":0,"data":{"credit":12,"energy":3}}`,
	})
	client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
	cred := testGlobalCredential("uid-claim")
	cred.SetRealm(RegionCN)

	message, credit, energy, errRun := runGrowthTask(context.Background(), client, cred, "chat_5")
	if errRun != nil {
		t.Fatalf("正常领奖不应报错: %v", errRun)
	}
	if credit != 12 || energy != 3 {
		t.Fatalf("reward = %d/%d, want 12/3", credit, energy)
	}
	if !strings.Contains(message, "已领奖") {
		t.Fatalf("message = %q, want 已领奖", message)
	}
}

// balanceHost 按 URL 回不同的额度响应，并记录被请求过的地址。
type balanceHost struct {
	personalBody   string
	enterpriseBody string
	paths          []string
}

func installBalanceHost(t *testing.T, host *balanceHost) {
	t.Helper()
	installHostHTTP(t, func(_, rawURL string) (int, string) {
		host.paths = append(host.paths, rawURL)
		if strings.Contains(rawURL, "enterprise-user-usage") {
			return http.StatusOK, host.enterpriseBody
		}
		return http.StatusOK, host.personalBody
	})
}

func (h *balanceHost) called(substr string) bool {
	for _, path := range h.paths {
		if strings.Contains(path, substr) {
			return true
		}
	}
	return false
}

// TestFetchBalanceFallsBackToEnterpriseUsage 企业账号在个人接口上拿到
// 200 + 空 Accounts，必须回退到企业用量接口，否则面板恒为 0 / 0。
func TestFetchBalanceFallsBackToEnterpriseUsage(t *testing.T) {
	host := &balanceHost{
		personalBody:   `{"code":0,"msg":"OK","data":{"Response":{"Data":{"TotalCount":0,"TotalDosage":0,"Accounts":null}}}}`,
		enterpriseBody: `{"code":0,"msg":"OK","data":{"credit":4.11,"limitNum":2000,"cycleStartTime":"2026-10-01 00:00:00","cycleEndTime":"2026-10-31 23:59:59","cycleResetTime":"2026-11-01 00:00:00"}}`,
	}
	installBalanceHost(t, host)
	client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
	cred := testTaskCredential("uid-enterprise-balance")
	cred.EnterpriseID = "fysxmvfpz6dc"

	balance, errFetch := client.FetchBalance(cred)
	if errFetch != nil {
		t.Fatalf("fetch balance: %v", errFetch)
	}
	if !balance.Enterprise {
		t.Fatal("企业账号的余额必须标记为 enterprise")
	}
	if balance.Total != 2000 || math.Abs(balance.Remain-1995.89) > 0.0001 {
		t.Fatalf("balance = %v/%v, want 1995.89/2000", balance.Remain, balance.Total)
	}
	if !host.called("/v2/billing/meter/get-user-resource") {
		t.Fatal("个人接口必须先被尝试（企业接口只是回退）")
	}
	if !host.called("get-enterprise-user-usage") {
		t.Fatal("企业账号必须回退到企业用量接口")
	}
}

// TestFetchBalanceKeepsEmptyForNonEnterpriseAccount 非企业账号的空额度是真
// 空额度，不能去打企业接口。
func TestFetchBalanceKeepsEmptyForNonEnterpriseAccount(t *testing.T) {
	host := &balanceHost{
		personalBody:   `{"code":0,"data":{"Response":{"Data":{"Accounts":null}}}}`,
		enterpriseBody: `{"code":0,"data":{"credit":1,"limitNum":100}}`,
	}
	installBalanceHost(t, host)
	client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
	cred := testTaskCredential("uid-personal-balance")

	balance, errFetch := client.FetchBalance(cred)
	if errFetch != nil {
		t.Fatalf("fetch balance: %v", errFetch)
	}
	if balance.Total != 0 || balance.Enterprise {
		t.Fatalf("balance = %+v, want empty personal quota", balance)
	}
	if host.called("enterprise-user-usage") {
		t.Fatal("非企业账号不应请求企业用量接口")
	}
}

// TestFetchBalancePrefersPersonalPackages 企业账号若个人接口确实有额度，
// 以个人资源包为准，不再叠加企业额度。
func TestFetchBalancePrefersPersonalPackages(t *testing.T) {
	host := &balanceHost{
		personalBody:   `{"code":0,"data":{"Response":{"Data":{"Accounts":[{"PackageName":"加油包","CapacitySize":500,"CapacityRemain":500}]}}}}`,
		enterpriseBody: `{"code":0,"data":{"credit":1,"limitNum":100}}`,
	}
	installBalanceHost(t, host)
	client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
	cred := testTaskCredential("uid-enterprise-personal")
	cred.EnterpriseID = "ent-1"

	balance, errFetch := client.FetchBalance(cred)
	if errFetch != nil {
		t.Fatalf("fetch balance: %v", errFetch)
	}
	if balance.Enterprise || balance.Total != 500 || balance.Remain != 500 {
		t.Fatalf("balance = %+v, want personal 500/500", balance)
	}
	if host.called("enterprise-user-usage") {
		t.Fatal("个人额度非空时不应请求企业接口")
	}
}

// TestParseEnterpriseBalanceKeepsFractionalCredit 企业已用量是小数，
// 截断成整数会让「消耗 0.9 积分」在面板上完全不可见。
func TestParseEnterpriseBalanceKeepsFractionalCredit(t *testing.T) {
	balance, errParse := parseEnterpriseBalance(json.RawMessage(`{
	  "credit":4.11,"limitNum":2000,
	  "cycleStartTime":"2026-10-01 00:00:00",
	  "cycleEndTime":"2026-10-31 23:59:59",
	  "cycleResetTime":"2026-11-01 00:00:00"}`))
	if errParse != nil {
		t.Fatalf("parse: %v", errParse)
	}
	if !balance.Enterprise || balance.Total != 2000 {
		t.Fatalf("balance = %+v", balance)
	}
	if math.Abs(balance.Remain-1995.89) > 0.0001 {
		t.Fatalf("remain = %v, want 1995.89", balance.Remain)
	}
	if len(balance.Packages) != 1 {
		t.Fatalf("packages = %d, want 1", len(balance.Packages))
	}
	pkg := balance.Packages[0]
	if pkg.Name != "企业额度" {
		t.Fatalf("package name = %q", pkg.Name)
	}
	if pkg.ExpireAt != 0 {
		t.Fatal("企业额度到点回满，不能设 ExpireAt（会被显示成即将过期）")
	}
	if pkg.ResetAt == 0 {
		t.Fatal("企业额度必须带周期回满时刻")
	}
	if !strings.Contains(pkg.Period, "2026-10-01") || !strings.Contains(pkg.Period, "2026-10-31") {
		t.Fatalf("period = %q", pkg.Period)
	}
}

// TestParseEnterpriseBalanceRejectsMissingLimit 缺 limitNum 的响应不能当成
// 0 / 0 展示：那正是本 issue 要消除的假象。
func TestParseEnterpriseBalanceRejectsMissingLimit(t *testing.T) {
	for _, raw := range []string{`{}`, `{"credit":1}`, `null`, `<html>error</html>`} {
		if _, errParse := parseEnterpriseBalance(json.RawMessage(raw)); errParse == nil {
			t.Fatalf("parseEnterpriseBalance(%s) 必须报错", raw)
		}
	}
}

// TestBuildQuotaResponseEnterprisePlan 企业额度不能显示成个人 CN 套餐。
func TestBuildQuotaResponseEnterprisePlan(t *testing.T) {
	cred := testTaskCredential("uid-enterprise-quota")
	cred.EnterpriseID = "ent-1"
	balance, errParse := parseEnterpriseBalance(json.RawMessage(`{
	  "credit":4.11,"limitNum":2000,
	  "cycleStartTime":"2026-10-01 00:00:00",
	  "cycleEndTime":"2026-10-31 23:59:59",
	  "cycleResetTime":"2026-11-01 00:00:00"}`))
	if errParse != nil {
		t.Fatalf("parse: %v", errParse)
	}
	response := BuildQuotaResponse(cred, balance)
	if response.Subscription == nil || response.Subscription.Plan != "enterprise" {
		t.Fatalf("subscription = %+v, want plan=enterprise", response.Subscription)
	}
	if response.Subscription.TierName != "企业额度" {
		t.Fatalf("tier = %q", response.Subscription.TierName)
	}
	metrics := map[string]float64{}
	for _, metric := range response.Summary {
		metrics[metric.Key] = metric.Value
	}
	if math.Abs(metrics["credit_used"]-4.11) > 0.0001 {
		t.Fatalf("credit_used = %v, want 4.11", metrics["credit_used"])
	}
	if math.Abs(metrics["credit_remain"]-1995.89) > 0.0001 {
		t.Fatalf("credit_remain = %v, want 1995.89", metrics["credit_remain"])
	}
	if len(response.Groups) != 1 || len(response.Groups[0].Buckets) != 1 {
		t.Fatalf("groups = %+v, want 1 bucket", response.Groups)
	}
	bucket := response.Groups[0].Buckets[0]
	if bucket.Window != "企业额度" {
		t.Fatalf("window = %q", bucket.Window)
	}
	if !strings.Contains(bucket.Description, "1995.89 / 2000") || !strings.Contains(bucket.Description, "2026-10-01") {
		t.Fatalf("description = %q", bucket.Description)
	}
	if bucket.ResetTime == "" {
		t.Fatal("企业额度应展示周期回满时刻")
	}
}

// TestBuildQuotaResponsePersonalDescription 个人额度是整数，不能显示成
// 400.00 / 1000.00 这种形态。
func TestBuildQuotaResponsePersonalDescription(t *testing.T) {
	var payload billingResourceResponse
	raw := `{"Response":{"Data":{"Accounts":[{"PackageName":"基础套餐","CycleCapacitySize":1000,"CycleCapacityRemain":400}]}}}`
	if errUnmarshal := json.Unmarshal([]byte(raw), &payload); errUnmarshal != nil {
		t.Fatalf("unmarshal: %v", errUnmarshal)
	}
	response := BuildQuotaResponse(testTaskCredential("uid-plain-quota"), parseBalance(payload))
	if len(response.Groups) != 1 || len(response.Groups[0].Buckets) != 1 {
		t.Fatalf("groups = %+v", response.Groups)
	}
	if got := response.Groups[0].Buckets[0].Description; got != "400 / 1000" {
		t.Fatalf("description = %q, want 400 / 1000", got)
	}
}

// TestGlobalDailyBonusUsesUpstreamStreak 锁定国际版每日活跃奖励的连签口径。
//
// 天数必须来自上游连登接口：本地按热力图推算在断签/补签后必然分叉（历史
// bug 就是拿 len(lit) 当连签，只增不减）。上游没给时返回 0，让上层保留旧值，
// 而不是猜一个「至少今天 1 天」。
func TestGlobalDailyBonusUsesUpstreamStreak(t *testing.T) {
	today := time.Now().In(cstZone).Format("2006-01-02")
	cases := []struct {
		name        string
		heatmap     string
		streakBody  string
		streakFails bool
		wantAlready bool
		wantStreak  int64
		wantBeacon  int
	}{
		{
			name:       "今日未点亮：上报信标并透传上游天数",
			heatmap:    `{"code":0,"data":{"cells":[{"date":"2020-01-01","score":1}]}}`,
			streakBody: `{"code":0,"data":{"streak":{"days":7}}}`,
			wantStreak: 7, wantBeacon: 1,
		},
		{
			name:        "连登接口失败：返回 0，不猜 1",
			heatmap:     `{"code":0,"data":{"cells":[{"date":"2020-01-01","score":1}]}}`,
			streakFails: true,
			wantStreak:  0, wantBeacon: 1,
		},
		{
			name:        "今日已点亮：不重复上报",
			heatmap:     `{"code":0,"data":{"cells":[{"date":"` + today + `","score":1}]}}`,
			streakBody:  `{"code":0,"data":{"streak":{"days":3}}}`,
			wantAlready: true, wantStreak: 3, wantBeacon: 0,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			beacons := 0
			installHostHTTP(t, func(_, rawURL string) (int, string) {
				switch {
				case strings.Contains(rawURL, "/activity/growth/heatmap"):
					return http.StatusOK, testCase.heatmap
				case strings.Contains(rawURL, "/activity/growth/streak"):
					if testCase.streakFails {
						return http.StatusInternalServerError, `{"code":500,"message":"boom"}`
					}
					return http.StatusOK, testCase.streakBody
				case strings.Contains(rawURL, reportPath):
					beacons++
					return http.StatusOK, `{"code":0}`
				}
				return http.StatusOK, `{}`
			})
			client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
			result, errBonus := client.GlobalDailyBonus(context.Background(), testGlobalCredential("uid-streak"))
			if errBonus != nil {
				t.Fatalf("GlobalDailyBonus: %v", errBonus)
			}
			if result == nil || result.Already != testCase.wantAlready || result.Streak != testCase.wantStreak {
				t.Fatalf("result = %+v, want already=%v streak=%d", result, testCase.wantAlready, testCase.wantStreak)
			}
			if beacons != testCase.wantBeacon {
				t.Fatalf("信标上报 %d 次, want %d", beacons, testCase.wantBeacon)
			}
		})
	}
}

// TestWaitGrowthClaimableChecksBeforeWaiting 锁定「先查后等」：
// 任务在第一次查询里就达标时不许先睡 3 秒，也不该发起第二次列表请求。
func TestWaitGrowthClaimableChecksBeforeWaiting(t *testing.T) {
	listCalls := 0
	installHostHTTP(t, func(_, rawURL string) (int, string) {
		if strings.Contains(rawURL, "/claim") {
			return http.StatusOK, `{"code":0,"data":{"credit":1,"energy":1}}`
		}
		listCalls++
		return http.StatusOK, `{"code":0,"data":[{"task_code":"chat_5","progress":{"current":5,"target":5}}]}`
	})
	client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
	cred := testGlobalCredential("uid-wait-hit")
	cred.SetRealm(RegionCN)

	start := time.Now()
	if !waitGrowthClaimable(context.Background(), client, cred, "chat_5") {
		t.Fatal("任务已达标时 waitGrowthClaimable 必须返回 true")
	}
	if listCalls != 1 {
		t.Fatalf("list calls = %d, want 1（先查后等，不该多轮）", listCalls)
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("耗时 %v：达标任务不该先睡 3 秒", elapsed)
	}
}

// TestWaitGrowthClaimableKeepsPollingWhenTaskNotVisible 锁定复审 P2-2：
// 动作刚做完、任务还没出现在列表里时不能立刻判失败，否则白做任务拿不到奖励。
func TestWaitGrowthClaimableKeepsPollingWhenTaskNotVisible(t *testing.T) {
	listCalls := 0
	installHostHTTP(t, func(_, rawURL string) (int, string) {
		if strings.Contains(rawURL, "/claim") {
			return http.StatusOK, `{"code":0,"data":{"credit":1,"energy":1}}`
		}
		listCalls++
		if listCalls == 1 {
			// 第一次：列表里还没有这个任务（上游列表最终一致）。
			return http.StatusOK, `{"code":0,"data":[]}`
		}
		return http.StatusOK, `{"code":0,"data":[{"task_code":"chat_5","progress":{"current":5,"target":5}}]}`
	})
	client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
	cred := testGlobalCredential("uid-wait-miss")
	cred.SetRealm(RegionCN)

	if !waitGrowthClaimable(context.Background(), client, cred, "chat_5") {
		t.Fatal("任务只是暂时不在列表里，必须继续轮询而不是立即判失败")
	}
	if listCalls < 2 {
		t.Fatalf("list calls = %d, want >= 2（task==nil 时必须继续等）", listCalls)
	}
}

// TestWaitGrowthClaimableStopsOnClaimed 锁定终态：已领奖的任务不再等待。
func TestWaitGrowthClaimableStopsOnClaimed(t *testing.T) {
	listCalls := 0
	installHostHTTP(t, func(_, rawURL string) (int, string) {
		listCalls++
		return http.StatusOK, `{"code":0,"data":[{"task_code":"chat_5","progress":{"current":5,"target":5},"claimed":true}]}`
	})
	client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
	cred := testGlobalCredential("uid-wait-claimed")
	cred.SetRealm(RegionCN)

	if waitGrowthClaimable(context.Background(), client, cred, "chat_5") {
		t.Fatal("已领奖的任务不该再报可领")
	}
	if listCalls != 1 {
		t.Fatalf("list calls = %d, want 1（终态应立即返回）", listCalls)
	}
}

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
	client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
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
	client := NewClient(newRuntime(core.Env{}), Options{Context: context.Background()})
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

// TestPackageRemainUsedRejectsNonBasePackWithoutMatchingSize 锁定回退规则的
// 结构约束：Capacity 与 Cycle 两本账的 size 不相等，说明它们描述的不是
// 同一笔发放，不能互证。
//
// 反例形态：终身额度包 Capacity 5000/3000/0，同时 Cycle 500/0/500（本月用尽）。
// 若只看「Cycle 清零 + Capacity 有余额 + Capacity 未扣减」，会把从未动用的
// 3000 点终身额度当成可用余额，且把 Total 抬高 4500——这正是本次修复要
// 消灭的那类虚增，只是方向相反。
func TestPackageRemainUsedRejectsNonBasePackWithoutMatchingSize(t *testing.T) {
	remain, used, size := packageRemainUsed(billingResourceAccount{
		CapacitySize: 5000, CapacityRemain: 3000, CapacityUsed: 0,
		CycleCapacitySize: 500, CycleCapacityRemain: 0, CycleCapacityUsed: 500,
	})
	if remain != 0 || used != 500 || size != 500 {
		t.Fatalf("lifetime pack: remain=%d used=%d size=%d, want 0/500/500（两账 size 不同，不得回退）", remain, used, size)
	}
}
