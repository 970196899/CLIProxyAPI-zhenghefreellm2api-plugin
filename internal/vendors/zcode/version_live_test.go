//go:build live

package zcode

// 版本对齐实况探针：同一凭证、同一端点，只改 app_version，对比上游返回
// 是否变化（模型目录 / 计费余额 / 业务码）。用于回答「升版本号会不会改变
// 上游下发的目录或额度口径」——静态比对版本号回答不了这个问题。
//
//	ZCODE_LIVE_CREDS=/opt/cliproxyapi/auths/zcodeglobal-xxx.json \
//	  go test -tags live ./internal/vendors/zcode -run TestLiveVersionAlign -v

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"freetier2api-plugin/internal/hostbridge"
)

// 对比用的版本号：前者是历史声明版本，后者是升版候选。
var liveVersionCandidates = []string{"3.16.3", ClientAppVersion}

// liveDirectHost 把插件宿主桥接到真实 HTTP，便于在服务器上直接跑探针。
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
		httpReq, errNew := http.NewRequest(req.Method, req.URL, nil)
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
	path := os.Getenv("ZCODE_LIVE_CREDS")
	if path == "" {
		t.Skip("ZCODE_LIVE_CREDS 未设置")
	}
	raw, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("read creds: %v", errRead)
	}
	cred, errParse := ParseCredential(raw)
	if errParse != nil {
		t.Fatalf("parse creds: %v", errParse)
	}
	liveDirectHost(t)
	ctx := context.Background()

	// 1) 模型目录：同一端点只改 app_version（该接口不需要鉴权）。
	for _, version := range liveVersionCandidates {
		url := ZCodeOrigin + PathClientConfigs + "?app_version=" + version
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("User-Agent", "ZCode/"+version)
		resp, errDo := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if errDo != nil {
			t.Logf("CONFIGS %-7s err=%v", version, errDo)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		models := parseConfigModels(body)
		ids := make([]string, 0, len(models))
		for _, m := range models {
			ids = append(ids, m.ID)
		}
		t.Logf("CONFIGS %-7s http=%d bytes=%d models=%d %v", version, resp.StatusCode, len(body), len(models), ids)
	}

	// 2) 计费余额：同一凭证，只改 app_version。
	for _, version := range liveVersionCandidates {
		code, body, errReq := billingRequest(ctx, cred, http.MethodGet,
			PathBillingBalance+"?app_version="+version, nil)
		if errReq != nil {
			t.Logf("BALANCE %-7s err=%v", version, errReq)
			continue
		}
		balances := objectRows(dataMap(body)["balances"])
		plans := planRows(body)
		totalRemain := 0.0
		for _, row := range balances {
			if remain, okRemain := row["remaining_units"].(float64); okRemain {
				totalRemain += remain
			}
		}
		t.Logf("BALANCE %-7s code=%d balances=%d plans=%d remaining=%.0f", version, code, len(balances), len(plans), totalRemain)
	}
}
