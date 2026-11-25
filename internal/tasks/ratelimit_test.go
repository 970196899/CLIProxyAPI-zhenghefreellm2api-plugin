package tasks

import (
	"testing"
	"time"
)

// TestRateLimitCooldownSkipsAccount 锁定账号级限流冷却：
// 命中 429 → 记 10 分钟冷却；冷却期内跳过；到期自动恢复。
func TestRateLimitCooldownSkipsAccount(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, checkinZone)
	deps := newFakeDeps(now)
	service := &Service{deps: deps}

	service.markRateLimited("acct-1", []autoAllResult{
		{Code: "签到", Status: "error", Message: `upstream soft_rate [429]: {"error_msg":"Too Many Requests"}`},
		{Code: "旅行", Status: "skipped", Message: "上游限流（429），已跳过剩余任务"},
	})
	until, limited := service.rateLimitedUntil("acct-1")
	if !limited {
		t.Fatal("命中 429 后账号应进入冷却")
	}
	if want := now.Add(rateLimitCooldown); !until.Equal(want) {
		t.Fatalf("冷却截止 = %s, want %s", until, want)
	}

	// 普通失败（500）不是限流，不该把账号关进冷却。
	service.markRateLimited("acct-2", []autoAllResult{
		{Code: "签到", Status: "error", Message: "上游 500 Internal Server Error"},
	})
	if _, limited := service.rateLimitedUntil("acct-2"); limited {
		t.Fatal("非限流错误不应触发冷却")
	}

	// 只有 skipped 行（其余任务被限流收尾）没有真错误时不记冷却。
	service.markRateLimited("acct-3", []autoAllResult{
		{Code: "旅行", Status: "skipped", Message: "上游限流（429），已跳过剩余任务"},
	})
	if _, limited := service.rateLimitedUntil("acct-3"); limited {
		t.Fatal("skipped 行不应触发冷却")
	}

	deps.now = now.Add(rateLimitCooldown + time.Second)
	if _, limited := service.rateLimitedUntil("acct-1"); limited {
		t.Fatal("冷却到期后账号应恢复")
	}
}
