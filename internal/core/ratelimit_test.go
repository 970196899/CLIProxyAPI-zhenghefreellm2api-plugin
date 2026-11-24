package core

import "testing"

// TestIsRateLimitMessage 锁定限流判定：只认稳定标记，不对裸数字做匹配。
//
// 误判代价不对称：漏判会让同一账号继续被 429 打；误判会把正常账号整轮跳过。
// 所以 "429" 必须带上下文（[429] / HTTP 429 / 429:）才算。
func TestIsRateLimitMessage(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{`upstream soft_rate [429]: {"error_msg":"Too Many Requests"}`, true},
		{"upstream soft_rate [429]: 上游限流", true},
		{"请求失败：Too Many Requests", true},
		{"rate limit exceeded", true},
		{"上游 HTTP 429", true},
		{"余额 14290 积分", false},
		{"2026-11-24T11:29:00Z 超时", false},
		{"上游 500 Internal Server Error", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsRateLimitMessage(tc.text); got != tc.want {
			t.Errorf("IsRateLimitMessage(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
