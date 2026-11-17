package app

// 出站请求体改写的正确性回归与性能基准。
//
// 改写曾经是「整份解成 map → 改值 → 序列化」：请求体越大越慢（64KB 实测
// 885µs、6.7k 次分配），而且所有数字都被过成 float64（超过 2^53 的整数静默
// 失真）、字段顺序被重排。现在走原地拼接，这些用例锁住行为。

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestRewritePayloadModelSplicesOnlyTopLevel 顶层改写、嵌套同名键不动、数字逐字保留。
func TestRewritePayloadModelSplicesOnlyTopLevel(t *testing.T) {
	payload := []byte(`{"model":"alias","seed":1234567890123456789,"messages":[{"role":"user","content":"hi","model":"nested"}],"tools":[{"function":{"name":"f","model":"deep"}}]}`)
	want := `{"model":"official/model","seed":1234567890123456789,"messages":[{"role":"user","content":"hi","model":"nested"}],"tools":[{"function":{"name":"f","model":"deep"}}]}`
	got := rewritePayloadModel(payload, "official/model")
	if string(got) != want {
		t.Fatalf("rewrite mismatch\n got: %s\nwant: %s", got, want)
	}
}

// TestRewritePayloadModelKeepsWhitespaceAndOrder 原地拼接不该重排字段或吃掉空白。
func TestRewritePayloadModelKeepsWhitespaceAndOrder(t *testing.T) {
	payload := []byte("{ \"model\" : \"old\" , \"b\":1, \"a\":2 }")
	want := "{ \"model\" : \"new\" , \"b\":1, \"a\":2 }"
	if got := string(rewritePayloadModel(payload, "new")); got != want {
		t.Fatalf("rewrite mismatch\n got: %s\nwant: %s", got, want)
	}
}

// TestRewritePayloadModelStringValueNamedModel 字符串值恰好是 "model" 时不能误判成键。
func TestRewritePayloadModelStringValueNamedModel(t *testing.T) {
	payload := []byte(`{"note":"model","model":"old"}`)
	got := rewritePayloadModel(payload, "new")
	want := `{"note":"model","model":"new"}`
	if string(got) != want {
		t.Fatalf("rewrite mismatch\n got: %s\nwant: %s", got, want)
	}
}

// TestRewritePayloadModelFallbacks 结构不合常规时回落到整份改写，结果仍然正确。
func TestRewritePayloadModelFallbacks(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		model   string
	}{
		{"缺少 model 键", `{"messages":[]}`, "m"},
		{"model 是 null", `{"model":null,"a":1}`, "m"},
		{"model 是数字", `{"model":1,"a":1}`, "m"},
		{"值需要转义", `{"model":"old"}`, `m"quote`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			out := rewritePayloadModel([]byte(testCase.payload), testCase.model)
			var obj map[string]any
			if errUnmarshal := json.Unmarshal(out, &obj); errUnmarshal != nil {
				t.Fatalf("output must stay valid JSON: %v (%s)", errUnmarshal, out)
			}
			if obj["model"] != testCase.model {
				t.Fatalf("model = %v, want %q", obj["model"], testCase.model)
			}
		})
	}
}

// TestRewritePayloadModelInvalidJSON 非法 JSON 原样返回（不改写、不 panic）。
func TestRewritePayloadModelInvalidJSON(t *testing.T) {
	payload := []byte(`{"model":`)
	if got := rewritePayloadModel(payload, "m"); !bytes.Equal(got, payload) {
		t.Fatalf("invalid payload must be returned unchanged, got %s", got)
	}
}

// TestRewritePayloadModelTruncatedString 截断在字符串里的非法 JSON 必须原样
// 返回，而不是在切点计算时越界。
//
// 上游 c2c1e66 引入的快路径用 payload[start+1:i-1] 取键名，扫描到 EOF 时
// i 与 start+1 可能相等，切片变成 payload[n:n-1]，Go 运行期直接 panic
// （slice bounds out of range）。仓库内没有任何 recover，panic 会穿过
// //export cliproxyPluginCall 的 C 边界，abort 整个宿主进程——所有供应商
// 同时中断。这里逐条锁死，任何一条 panic 都算回归。
func TestRewritePayloadModelTruncatedString(t *testing.T) {
	// 每条都是「扫描到 EOF 也没见到闭合引号」的真实截断形态：
	// 快路径判不出安全切点，必须原样返回。
	payloads := []string{
		`{"`,
		`{ "`,
		`{"a":1,"`,
		`{"messages":["`,
		`{"model":"`,
		`{"\`,
		`{"model":"a\`,
	}
	for _, payload := range payloads {
		t.Run(payload, func(t *testing.T) {
			// 不做 recover：真 panic 就让整条用例炸掉，这才是有判别力的断言。
			got := rewritePayloadModel([]byte(payload), "official/model")
			if !bytes.Equal(got, []byte(payload)) {
				t.Fatalf("truncated payload must be returned unchanged\n got: %q\nwant: %q", got, payload)
			}
		})
	}
}

// TestRewritePayloadModelTruncatedNoPanic 截断但字符串本身完整闭合的形态：
// 改写与否都合法，唯一硬要求是**不能 panic**。
//
// 与上一个用例的区别：`{"model":"a","` 的 model 值已经闭合，快路径能安全
// 改写；切点之外那个悬空的 `"` 不影响结果的有效性。panic 才是缺陷。
func TestRewritePayloadModelTruncatedNoPanic(t *testing.T) {
	payloads := []string{
		`{"model":"a","`,
		`{"model":"a"`,
		`{"model":"a",`,
	}
	for _, payload := range payloads {
		t.Run(payload, func(t *testing.T) {
			// 不 recover、不断言内容：只要这一行能返回，就说明没 panic。
			_ = rewritePayloadModel([]byte(payload), "official/model")
		})
	}
}

// TestRewritePayloadModelUnterminatedValue 值字符串未闭合时不得伪造成
// 「改写成功」的畸形 JSON。
func TestRewritePayloadModelUnterminatedValue(t *testing.T) {
	payload := []byte(`{"model":"old`)
	got := rewritePayloadModel(payload, "new")
	if !bytes.Equal(got, payload) {
		t.Fatalf("unterminated value must be returned unchanged\n got: %q\nwant: %q", got, payload)
	}
}

// TestRewritePayloadModelDuplicateModelKey 重复 model 键时走慢路径，
// 保证「最后一个同名键生效」的解析器语义也拿到官方 ID。
func TestRewritePayloadModelDuplicateModelKey(t *testing.T) {
	payload := []byte(`{"model":"a","model":"b"}`)
	got := rewritePayloadModel(payload, "official")
	var obj map[string]any
	if errUnmarshal := json.Unmarshal(got, &obj); errUnmarshal != nil {
		t.Fatalf("output must stay valid JSON: %v (%s)", errUnmarshal, got)
	}
	// encoding/json 取最后一个同名键，因此最终值必须是官方 ID。
	if obj["model"] != "official" {
		t.Fatalf("model = %v, want official (last duplicate key must win)", obj["model"])
	}
}

// benchRequestPayload 造一个带长上下文的请求体。
func benchRequestPayload(size int) []byte {
	var builder strings.Builder
	builder.WriteString(`{"model":"alias","stream":true,"seed":1234567890123456789,"messages":[`)
	for builder.Len() < size {
		builder.WriteString(`{"role":"user","content":"这是一段用于压测的较长中文对话内容，用来模拟真实的长上下文请求体。"},`)
	}
	builder.WriteString(`{"role":"user","content":"hi"}]}`)
	return []byte(builder.String())
}

// BenchmarkRewritePayloadModel 是当前实现（原地拼接）。
func BenchmarkRewritePayloadModel(b *testing.B) {
	payload := benchRequestPayload(64 * 1024)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = rewritePayloadModel(payload, "official/model")
	}
}

// BenchmarkRewritePayloadModelRoundTrip 是旧实现（整份 map 往返），留作对照。
func BenchmarkRewritePayloadModelRoundTrip(b *testing.B) {
	payload := benchRequestPayload(64 * 1024)
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = rewritePayloadModelRoundTrip(payload, "official/model")
	}
}

// BenchmarkStreamEmitRequestMarshal 是每帧一次的出站信封序列化。
func BenchmarkStreamEmitRequestMarshal(b *testing.B) {
	payload := []byte(`{"id":"chatcmpl-9f2c1e","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"一段输出文本"},"finish_reason":null}],"usage":null}`)
	request := &streamEmitRequest{StreamID: "stream-abc", Payload: payload}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		raw, errMarshal := json.Marshal(request)
		if errMarshal != nil {
			b.Fatal(errMarshal)
		}
		_ = raw
	}
}
