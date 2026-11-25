package trae

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const recoveryRequest = `{"model":"DeepSeek-V4-Pro","tools":[{"type":"function","function":{"name":"probe_echo","parameters":{"type":"object","properties":{"value":{"type":"string"}}}}}]}`
const jsonInvocation = `{"name":"probe_echo","arguments":{"value":"sample"}}`
const dsmlInvocation = `<｜DSML｜function_calls>
<｜DSML｜invoke name="probe_echo">
<｜DSML｜parameter name="value" string="true">sample</｜DSML｜parameter>
<｜DSML｜parameter name="count" string="false">2</｜DSML｜parameter>
</｜DSML｜invoke>
</｜DSML｜function_calls>`

func TestTraeRecoversToolText(t *testing.T) {
	stubHost(t)
	cases := []struct {
		name, output, wantArgs string
	}{
		{"JSON", jsonInvocation, `{"value":"sample"}`},
		{"single-string call", `probe_echo("sample")`, `{"value":"sample"}`},
		{"DSML", dsmlInvocation, `{"count":2,"value":"sample"}`},
		{"unknown tool", strings.ReplaceAll(jsonInvocation, "probe_echo", "other"), ""},
		{"normal prose", "Example: " + jsonInvocation, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case EpModels:
					_, _ = io.WriteString(w, catalogFixture)
				case EpChat:
					w.Header().Set("Content-Type", "text/event-stream")
					for _, part := range []string{tc.output[:1], tc.output[1:]} {
						data, _ := json.Marshal(map[string]string{"response": part})
						_, _ = fmt.Fprintf(w, "event: output\ndata: %s\n\n", data)
					}
					_, _ = io.WriteString(w, "event: done\ndata: {\"finish_reason\":\"stop\"}\n\n")
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			req := ChatRequest{Credential: testCredential(), Profile: testConfig(t, server.URL), Body: []byte(recoveryRequest)}
			type openAIMessage struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Index    *int   `json:"index"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			}
			for _, stream := range []bool{false, true} {
				var text, finish string
				var got openAIMessage
				consume := func(raw []byte) error {
					var response struct {
						Choices []struct {
							Message openAIMessage
							Delta   openAIMessage
							Finish  string `json:"finish_reason"`
						}
					}
					if err := json.Unmarshal(raw, &response); err != nil {
						return err
					}
					for _, c := range response.Choices {
						message := c.Message
						if stream {
							message = c.Delta
						}
						text += message.Content
						if len(message.ToolCalls) > 0 {
							got.ToolCalls = append(got.ToolCalls, message.ToolCalls...)
						}
						if c.Finish != "" {
							finish = c.Finish
						}
					}
					return nil
				}
				if stream {
					if err := ChatStream(context.Background(), "", req, consume); err != nil {
						t.Fatal(err)
					}
				} else {
					raw, err := Chat(context.Background(), "", req)
					if err != nil {
						t.Fatal(err)
					}
					if err := consume(raw); err != nil {
						t.Fatal(err)
					}
				}
				if tc.wantArgs == "" {
					if text != tc.output || finish != "stop" || len(got.ToolCalls) != 0 {
						t.Fatalf("stream=%t: unexpected content %q, finish %q, calls %+v", stream, text, finish, got.ToolCalls)
					}
					continue
				}
				if text != "" || finish != "tool_calls" || len(got.ToolCalls) != 1 {
					t.Fatalf("stream=%t: content %q, finish %q, calls %+v", stream, text, finish, got.ToolCalls)
				}
				call := got.ToolCalls[0]
				if call.ID == "" || call.Type != "function" || (stream && (call.Index == nil || *call.Index != 0)) ||
					call.Function.Name != "probe_echo" || call.Function.Arguments != tc.wantArgs {
					t.Fatalf("stream=%t: invalid tool call: %+v", stream, call)
				}
			}
		})
	}
}

func TestTraeToolTextRespectsRequest(t *testing.T) {
	tests := []struct {
		name, payload, model, output string
	}{
		{"no tools", `{}`, "deepseek", jsonInvocation},
		{"other model", recoveryRequest, "glm-5.2", jsonInvocation},
		{"tool_choice none", strings.TrimSuffix(recoveryRequest, "}") + `,"tool_choice":"none"}`, "deepseek", jsonInvocation},
		{"JSON response requested", strings.TrimSuffix(recoveryRequest, "}") + `,"response_format":{"type":"json_object"}}`, "deepseek", jsonInvocation},
		{"bad arguments", recoveryRequest, "deepseek", strings.Replace(jsonInvocation, `{"value":"sample"}`, `[]`, 1)},
		{"multiple positional arguments", recoveryRequest, "deepseek", `probe_echo("sample","extra")`},
		{"ambiguous schema", strings.Replace(recoveryRequest, `"value":{"type":"string"}`, `"value":{"type":"string"},"other":{"type":"string"}`, 1), "deepseek", `probe_echo("sample")`},
		{"truncated marker", recoveryRequest, "deepseek", strings.TrimSuffix(dsmlInvocation, "</｜DSML｜function_calls>")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if text, calls := newToolText([]byte(tt.payload), tt.model).recover(tt.output); text != tt.output || len(calls) != 0 {
				t.Fatalf("unexpected conversion: %q, %+v", text, calls)
			}
		})
	}
}
