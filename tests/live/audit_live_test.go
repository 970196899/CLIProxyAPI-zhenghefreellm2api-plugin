//go:build live

package live

// 跨渠道只读活动巡检：把真实凭证目录喂给插件注册表，逐账号调用
// ProbeActivity（只读，不领取），打印每家渠道的协议是否还通。
//
//	FREETIER_AUDIT_AUTHS=/opt/cliproxyapi/auths \
//	  go test -tags live ./tests/live -run TestLiveActivityAudit -v

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"freetier2api-plugin/internal/app"
	"freetier2api-plugin/internal/core"
)

func truncateErr(err error) string {
	text := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(text) > 110 {
		text = text[:110]
	}
	return text
}

func firstLine(text string) string {
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	if len(text) > 90 {
		text = text[:90]
	}
	return text
}

func TestLiveActivityAudit(t *testing.T) {
	dir := os.Getenv("FREETIER_AUDIT_AUTHS")
	if dir == "" {
		t.Skip("FREETIER_AUDIT_AUTHS 未设置")
	}
	configureDirectHost(t)

	cfg := app.DefaultConfig()
	cfg.StateDir = t.TempDir()
	cfg.LogLevel = "error"
	cfg.LogToFile = false
	instance, errNew := app.New(app.Options{Version: "audit", Factories: app.BuiltinFactories()})
	if errNew != nil {
		t.Fatalf("new app: %v", errNew)
	}
	t.Cleanup(func() {
		instance.StopBackgroundWork()
		instance.WaitPluginStreams(0)
	})
	if errApply := instance.ApplyConfig(cfg); errApply != nil {
		t.Fatalf("apply config: %v", errApply)
	}
	if _, errState := instance.LoadState(cfg); errState != nil {
		t.Fatalf("load state: %v", errState)
	}

	vendors := instance.Registry().Vendors()
	entries, errRead := os.ReadDir(dir)
	if errRead != nil {
		t.Fatalf("read auths dir: %v", errRead)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		raw, errFile := os.ReadFile(filepath.Join(dir, name))
		if errFile != nil {
			t.Logf("AUDIT %-46s read-fail %v", name, errFile)
			continue
		}
		var fields map[string]any
		if errJSON := json.Unmarshal(raw, &fields); errJSON != nil {
			t.Logf("AUDIT %-46s bad-json %v", name, errJSON)
			continue
		}
		var matched core.Vendor
		for _, vendor := range vendors {
			if vendor.Match(name, core.ProviderKey, fields) {
				matched = vendor
				break
			}
		}
		if matched == nil {
			t.Logf("AUDIT %-46s unmatched", name)
			continue
		}
		cred, errParse := matched.Parse(raw, name)
		if errParse != nil {
			t.Logf("AUDIT %-46s %-16s parse-fail %v", name, matched.ID(), errParse)
			continue
		}
		// 1) 额度：所有实现 QuotaProvider 的渠道都能只读验证。
		quotaState := "n/a"
		if quotaProbe, okQuota := core.As[core.QuotaProvider](matched); okQuota {
			quotaCtx, quotaCancel := context.WithTimeout(context.Background(), 60*time.Second)
			quota, errQuota := quotaProbe.Quota(quotaCtx, cred)
			quotaCancel()
			if errQuota != nil {
				quotaState = "FAIL " + truncateErr(errQuota)
			} else if quota == nil {
				quotaState = "ok(nil)"
			} else {
				summary := make([]string, 0, len(quota.Summary))
				for _, metric := range quota.Summary {
					summary = append(summary, fmt.Sprintf("%s=%.2f", metric.Key, metric.Value))
				}
				quotaState = fmt.Sprintf("ok groups=%d %s", len(quota.Groups), strings.Join(summary, " "))
			}
		}
		t.Logf("QUOTA %-46s %-16s %s", name, matched.ID(), quotaState)

		probe, okProbe := core.As[core.TaskInspector](matched)
		if !okProbe {
			t.Logf("AUDIT %-46s %-16s no-task-inspector", name, matched.ID())
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		result, errProbe := probe.InspectTasks(ctx, cred)
		cancel()
		if errProbe != nil {
			t.Logf("AUDIT %-46s %-16s FAIL %v", name, matched.ID(), errProbe)
			continue
		}
		titles := make([]string, 0, len(result.Items))
		for _, item := range result.Items {
			titles = append(titles, item.Code)
		}
		t.Logf("AUDIT %-46s %-16s ok n=%d period=%v %v", name, matched.ID(), len(result.Items), result.InPeriod, titles)
	}
}
