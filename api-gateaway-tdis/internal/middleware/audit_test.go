package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuditStripsSpoofedIdentity(t *testing.T) {
	var gotID, gotRole string
	h := Audit(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotRole = r.Header.Get("X-User-Id"), r.Header.Get("X-User-Role")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/users/login", nil)
	req.Header.Set("X-User-Id", "otro-usuario")
	req.Header.Set("X-User-Role", "COORDINADOR")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotID != "" || gotRole != "" {
		t.Fatalf("headers de identidad del cliente llegaron al downstream: %q %q", gotID, gotRole)
	}
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")
	if ip := clientIP(req); ip != "1.2.3.4" {
		t.Fatalf("clientIP = %q", ip)
	}
}
