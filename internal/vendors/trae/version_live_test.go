//go:build live

package trae

// 版本对齐实况探针：同一凭证、同一端点，只改 IDE 版本号 / 构建号，看上游
// 下发的模型目录是否变化。上游按 X-Ide-Version-Code 裁剪目录，静态比对
// 版本号回答不了「升版会不会少模型」。
//
//	TRAE_LIVE_CREDS=/opt/cliproxyapi/auths/traecn-xxx.json \
//	  go test -tags live ./internal/vendors/trae -run TestLiveVersionAlign -v

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"freetier2api-plugin/internal/hostbridge"
)

type liveVersionCase struct {
	version string
	code    string
	note    string
}

var liveVersionCases = []liveVersionCase{
	{IDEVersionCN, IDEVersionCodeCN, "当前声明"},
	{IDEVersionCN, "20261125", "只抬构建号"},
	{"3.6.20", "20261125", "抬到最新客户端"},
}

func liveDirectHost(t *testing.T) {
	t.Helper()
	hostbridge.Configure(func(_ string, method string, payload any) (json.RawMessage, error) {
		raw, errMarshal := json.Marshal(payload)
		if errMarshal != nil {
			return nil, errMarshal
		}
		var req struct {
			Method  string              `json:"method"`
			URL     string              `json:"url"`
			Headers map[string][]string `json:"headers"`
			Body    []byte              `json:"body"`
		}
		if errDecode := json.Unmarshal(raw, &req); errDecode != nil {
			return nil, errDecode
		}
		var bodyReader io.Reader
		if len(req.Body) > 0 {
			bodyReader = strings.NewReader(string(req.Body))
		}
		httpReq, errNew := http.NewRequest(req.Method, req.URL, bodyReader)
		if errNew != nil {
			return nil, errNew
		}
		for k, vs := range req.Headers {
			for _, v := range vs {
				httpReq.Header.Add(k, v)
			}
		}
		resp, errDo := (&http.Client{Timeout: 60 * time.Second}).Do(httpReq)
		if errDo != nil {
			return nil, errDo
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return json.Marshal(map[string]any{
			"status_code": resp.StatusCode,
			"headers":     resp.Header,
			"body":        body,
		})
	})
}

func TestLiveVersionAlign(t *testing.T) {
	path := os.Getenv("TRAE_LIVE_CREDS")
	if path == "" {
		t.Skip("TRAE_LIVE_CREDS 未设置")
	}
	raw, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("read creds: %v", errRead)
	}
	cred, errParse := ParseCredential(raw, RegionCN)
	if errParse != nil {
		t.Fatalf("parse creds: %v", errParse)
	}
	liveDirectHost(t)
	ctx := context.Background()

	for _, tc := range liveVersionCases {
		cfg := ConfigFor(RegionCN)
		cfg.IDEVersion = tc.version
		cfg.IDEVersionCode = tc.code
		entry, okCatalog := fetchCatalog(ctx, cred, cfg)
		ids := make([]string, 0, len(entry.metadata))
		for _, model := range entry.metadata {
			ids = append(ids, model.ID)
		}
		t.Logf("CATALOG %-8s %-9s (%-12s) ok=%t models=%d %v",
			tc.version, tc.code, tc.note, okCatalog, len(ids), ids)

		quota, errQuota := FetchQuota(ctx, cred, cfg)
		if errQuota != nil {
			t.Logf("QUOTA   %-8s %-9s err=%v", tc.version, tc.code, errQuota)
			continue
		}
		summary := ""
		for _, metric := range quota.Summary {
			summary += metric.Key + "=" + strconv.FormatFloat(metric.Value, 'f', 2, 64) + " "
		}
		t.Logf("QUOTA   %-8s %-9s %s", tc.version, tc.code, summary)
	}
}
