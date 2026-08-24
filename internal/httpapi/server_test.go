package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"artifact-resolver/internal/service"
	"artifact-resolver/internal/store"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewServer(service.New(st)).Handler()
}

func doReq(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestHealthz(t *testing.T) {
	h := newTestServer(t)
	w := doReq(t, h, "GET", "/healthz", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("healthz status = %d", w.Code)
	}
}

func TestArtifactLifecycle(t *testing.T) {
	h := newTestServer(t)
	w := doReq(t, h, "POST", "/artifacts", map[string]string{"name": "mylib", "description": "d"})
	if w.Code != http.StatusCreated {
		t.Fatalf("create artifact status = %d, body=%s", w.Code, w.Body.String())
	}
	// 重名 409。
	w = doReq(t, h, "POST", "/artifacts", map[string]string{"name": "mylib"})
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate artifact status = %d, want 409", w.Code)
	}
	// 非法 semver 400。
	w = doReq(t, h, "POST", "/artifacts/mylib/versions", map[string]string{"version": "x"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid version status = %d, want 400", w.Code)
	}
}

func TestErrorEnvelope(t *testing.T) {
	h := newTestServer(t)
	w := doReq(t, h, "GET", "/artifacts/nope", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Fatalf("expected error envelope, got %s", w.Body.String())
	}
}
