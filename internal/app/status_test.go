package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"myclaw/internal/session"
)

func TestStatusHandlerReturnsSessionSnapshot(t *testing.T) {
	manager := session.NewManager(nil)
	main := manager.GetOrCreateMain("main")
	if _, err := manager.AppendMessage(main.ID, "user", "hello"); err != nil {
		t.Fatalf("append message: %v", err)
	}
	child := manager.CreateChild("main", "agent:main:child:test")

	req := httptest.NewRequest(http.MethodGet, "/statusz", nil)
	rec := httptest.NewRecorder()

	StatusHandler(manager).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatal("empty status body")
	}
	if want := `"key":"agent:main:main"`; !contains(body, want) {
		t.Fatalf("body missing %s: %s", want, body)
	}
	if want := `"message_count":1`; !contains(body, want) {
		t.Fatalf("body missing %s: %s", want, body)
	}
	if want := child.Key; !contains(body, want) {
		t.Fatalf("body missing child session %s: %s", want, body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
