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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"freetier2api-plugin/internal/app"
	"freetier2api-plugin/internal/core"
)

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
		probe, okProbe := core.As[core.TaskInspector](matched)
		if !okProbe {
			t.Logf("AUDIT %-46s %-16s no-task-inspector", name, matched.ID())
			continue
		}
		cred, errParse := matched.Parse(raw, name)
		if errParse != nil {
			t.Logf("AUDIT %-46s %-16s parse-fail %v", name, matched.ID(), errParse)
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
