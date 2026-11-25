package raccoon

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"freetier2api-plugin/cpasdk/pluginapi"
	"freetier2api-plugin/internal/auth"
	"freetier2api-plugin/internal/core"
	"freetier2api-plugin/internal/hostbridge/hostbridgetest"
)

func TestLoginCallbackCompletion(t *testing.T) {
	hostbridgetest.UseLoopbackHost(t)
	var exchanges atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != LoginCodePath {
			http.NotFound(w, r)
			return
		}
		exchanges.Add(1)
		fmt.Fprint(w, `{"code":0,"data":{"access_token":"test-token","refresh_token":"test-refresh","user_id":"test-user"}}`)
	}))
	defer upstream.Close()

	adapter := Instances(core.Env{Logins: auth.NewSessionRegistry()})[0].(*adapter)
	adapter.authBase = upstream.URL
	ctx := context.Background()
	const code = "test-code"

	t.Run("loopback hands credentials to the host poll", func(t *testing.T) {
		const state = "loopback-state"
		const sessionID = VendorID + "_" + state
		StoreLoginSession(adapter.Env.Logins, sessionID, &PendingLogin{State: state})
		if _, err := adapter.receiveLoginCallback(sessionID, code, "wrong-state"); err == nil {
			t.Fatal("accepted a mismatched callback state")
		}
		before, err := adapter.LoginPoll(ctx, sessionID)
		if err != nil || before.Status != pluginapi.AuthLoginStatusPending {
			t.Fatalf("poll before valid callback = %+v, %v", before, err)
		}
		if _, err := adapter.receiveLoginCallback(sessionID, code, state); err != nil {
			t.Fatal(err)
		}
		if got := exchanges.Load(); got != 0 {
			t.Fatalf("loopback tried to exchange the code before polling: %d exchanges", got)
		}

		result, err := adapter.LoginPoll(ctx, sessionID)
		if err != nil || result.Status != pluginapi.AuthLoginStatusSuccess {
			t.Fatalf("poll after callback = %+v, %v", result, err)
		}
		if result.Auth.FileName != core.FileNameFor(VendorID, "test-user") || len(result.Auth.StorageJSON) == 0 {
			t.Fatal("successful poll did not return persistable credentials")
		}

		after, err := adapter.LoginPoll(ctx, sessionID)
		if err != nil || after.Status != pluginapi.AuthLoginStatusPending {
			t.Fatalf("overlapping poll must not invalidate the host's pending commit: %+v, %v", after, err)
		}
		if got := exchanges.Load(); got != 1 {
			t.Fatalf("expected one exchange, got %d", got)
		}
	})

	t.Run("manual and automatic completion share one exchange", func(t *testing.T) {
		const state = "manual-state"
		const sessionID = VendorID + "_" + state
		StoreLoginSession(adapter.Env.Logins, sessionID, &PendingLogin{State: state})
		if _, err := adapter.receiveLoginCallback(sessionID, code, state); err != nil {
			t.Fatal(err)
		}
		callbackURL := CallbackURL() + "?code=" + code + "&state=" + state
		result, err := adapter.SubmitCallback(ctx, sessionID, callbackURL)
		if err != nil || result.Status != pluginapi.AuthLoginStatusSuccess || len(result.Auth.StorageJSON) == 0 {
			t.Fatalf("manual callback = %+v, %v", result, err)
		}
		after, err := adapter.LoginPoll(ctx, sessionID)
		if err != nil || after.Status != pluginapi.AuthLoginStatusPending {
			t.Fatalf("poll after manual completion = %+v, %v", after, err)
		}
		if _, err := adapter.SubmitCallback(ctx, sessionID, callbackURL); err == nil {
			t.Fatal("duplicate manual callback succeeded")
		}
		if got := exchanges.Load(); got != 2 {
			t.Fatalf("expected one exchange per session, got %d", got)
		}
	})

	t.Run("concurrent polls return one credential without failing the session", func(t *testing.T) {
		const state = "concurrent-state"
		const sessionID = VendorID + "_" + state
		StoreLoginSession(adapter.Env.Logins, sessionID, &PendingLogin{State: state})
		if _, err := adapter.receiveLoginCallback(sessionID, code, state); err != nil {
			t.Fatal(err)
		}
		statuses := make(chan pluginapi.AuthLoginStatus, 2)
		start := make(chan struct{})
		for i := 0; i < 2; i++ {
			go func() {
				<-start
				response, err := adapter.LoginPoll(ctx, sessionID)
				if err != nil {
					t.Error(err)
					statuses <- pluginapi.AuthLoginStatusError
					return
				}
				statuses <- response.Status
			}()
		}
		close(start)
		got := map[pluginapi.AuthLoginStatus]int{<-statuses: 1}
		got[<-statuses]++
		if got[pluginapi.AuthLoginStatusSuccess] != 1 || got[pluginapi.AuthLoginStatusPending] != 1 {
			t.Fatalf("concurrent poll statuses = %v, want one success and one pending", got)
		}
		if got := exchanges.Load(); got != 3 {
			t.Fatalf("expected one exchange per session, got %d", got)
		}
	})
}
