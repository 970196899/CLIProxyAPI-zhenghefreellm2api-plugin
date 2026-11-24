package cline

// 本文件实现 Cline 账号余额与套餐查询。
//
// 协议来自官方客户端（cline/cline）最新源码的账号服务：
//
//	GET /api/v1/users/me                  → 用户档案（含 id / email / organizations）
//	GET /api/v1/users/{id}/balance        → { balance: number, userId }
//	GET /api/v1/users/me/plan             → { plan: {...}, currentPeriodEnd, ... }
//	GET /api/v1/plans?type=individual     → 可订阅套餐清单（这里只取名字）
//
// 旧实现完全没有额度能力，控制台只能显示「-- / --」；补上之后余额和当前
// 套餐都能像其它渠道一样展示。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"freetier2api-plugin/cpasdk/pluginapi"
	"freetier2api-plugin/internal/core"
	"freetier2api-plugin/internal/hostbridge"
)

// accountHTTPTimeout 是账号类只读请求的上限。
const accountHTTPTimeout = 20 * time.Second

const (
	mePath      = "/users/me"
	planPath    = "/users/me/plan"
	balancePath = "/users/%s/balance"
)

// AccountUser 是 /users/me 的关键字段。
type AccountUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// AccountBalance 是余额接口的响应。
//
// Balance 是美元余额（上游按美分计价，接口已折算成美元浮点）。
type AccountBalance struct {
	Balance float64 `json:"balance"`
	UserID  string  `json:"userId"`
}

// CurrentPlan 是 /users/me/plan 的关键字段。
type CurrentPlan struct {
	Plan *struct {
		Name         string `json:"name"`
		DisplayName  string `json:"displayName"`
		Interval     string `json:"interval"`
		PricePerSeat *int64 `json:"pricePerSeatCents"`
	} `json:"plan"`
	CurrentPeriodEnd string `json:"currentPeriodEnd"`
}

// FetchMe 读取当前账号档案。
func FetchMe(ctx context.Context, baseURL string, cred *Credential) (*AccountUser, error) {
	var user AccountUser
	if errGet := getAccountJSON(ctx, baseURL, cred, mePath, &user); errGet != nil {
		return nil, errGet
	}
	return &user, nil
}

// FetchBalance 读取账号余额（美元）。
func FetchBalance(ctx context.Context, baseURL string, cred *Credential, userID string) (*AccountBalance, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("cline: 缺少用户 ID")
	}
	var balance AccountBalance
	path := fmt.Sprintf(balancePath, url.PathEscape(userID))
	if errGet := getAccountJSON(ctx, baseURL, cred, path, &balance); errGet != nil {
		return nil, errGet
	}
	return &balance, nil
}

// FetchCurrentPlan 读取当前套餐；未订阅时上游返回空对象。
func FetchCurrentPlan(ctx context.Context, baseURL string, cred *Credential) (*CurrentPlan, error) {
	var plan CurrentPlan
	if errGet := getAccountJSON(ctx, baseURL, cred, planPath, &plan); errGet != nil {
		return nil, errGet
	}
	return &plan, nil
}

// getAccountJSON 发一次带认证的 GET 并解析 JSON。
//
// 账号类接口不需要 chat 那套 X-CLIENT-* 伪装头，只带 Authorization 与 UA。
func getAccountJSON(ctx context.Context, baseURL string, cred *Credential, path string, out any) error {
	req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+path, nil)
	if errReq != nil {
		return errReq
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+cred.BearerToken())
	req.Header.Set("User-Agent", UserAgent())
	resp, errDo := hostbridge.Client(ctx, accountHTTPTimeout).Do(req)
	if errDo != nil {
		return errDo
	}
	defer func() { _ = resp.Body.Close() }()
	body, errRead := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if errRead != nil {
		return errRead
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cline: %s 返回 %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if errDecode := json.Unmarshal(body, out); errDecode != nil {
		return fmt.Errorf("cline: 解析 %s 失败: %w", path, errDecode)
	}
	return nil
}

// quotaResponse 把余额与套餐拼成宿主可展示的额度结构。
func quotaResponse(balance *AccountBalance, plan *CurrentPlan) *pluginapi.QuotaFetchResponse {
	response := &pluginapi.QuotaFetchResponse{
		Summary: []pluginapi.QuotaMetric{
			{
				Key:      "credit_remain",
				Label:    "余额",
				Value:    balance.Balance,
				Format:   "number",
				Currency: "USD",
			},
		},
	}
	if plan != nil && plan.Plan != nil {
		name := core.FirstNonEmpty(plan.Plan.DisplayName, plan.Plan.Name)
		if name != "" {
			response.Subscription = &pluginapi.QuotaSubscription{Plan: name, TierName: name}
		}
	}
	if plan != nil && strings.TrimSpace(plan.CurrentPeriodEnd) != "" {
		// 到期时间没有专门的字段，拼进档位名里展示（控制台只渲染文本）。
		if response.Subscription == nil {
			response.Subscription = &pluginapi.QuotaSubscription{Plan: "Cline", TierName: "Cline"}
		}
		response.Subscription.TierName = strings.TrimSpace(response.Subscription.TierName + " · 至 " + plan.CurrentPeriodEnd)
	}
	return response
}
