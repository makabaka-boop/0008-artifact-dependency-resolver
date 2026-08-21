package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

func TestResolveCanceledContextDoesNotPersistSucceededIncompleteResolution(t *testing.T) {
	h := newTestServer(t)

	mustStatus := func(method, path string, body any, want int) {
		t.Helper()
		w := doReq(t, h, method, path, body)
		if w.Code != want {
			t.Fatalf("%s %s status = %d, want %d, body=%s", method, path, w.Code, want, w.Body.String())
		}
	}

	for _, name := range []string{"cancel-root", "cancel-middle", "cancel-leaf"} {
		mustStatus(http.MethodPost, "/artifacts", map[string]string{"name": name}, http.StatusCreated)
		mustStatus(http.MethodPost, "/artifacts/"+name+"/versions", map[string]string{"version": "1.0.0"}, http.StatusCreated)
	}
	mustStatus(http.MethodPut, "/artifacts/cancel-root/versions/1.0.0/dependencies", map[string]any{
		"dependencies": []map[string]string{{"name": "cancel-middle", "constraint": "^1.0.0"}},
	}, http.StatusOK)
	mustStatus(http.MethodPut, "/artifacts/cancel-middle/versions/1.0.0/dependencies", map[string]any{
		"dependencies": []map[string]string{{"name": "cancel-leaf", "constraint": "^1.0.0"}},
	}, http.StatusOK)
	for _, name := range []string{"cancel-leaf", "cancel-middle", "cancel-root"} {
		mustStatus(http.MethodPost, "/artifacts/"+name+"/versions/1.0.0/publish", nil, http.StatusOK)
	}

	var requestBody bytes.Buffer
	if err := json.NewEncoder(&requestBody).Encode(map[string]any{
		"manifest": []map[string]string{{"name": "cancel-root", "constraint": "^1.0.0"}},
	}); err != nil {
		t.Fatalf("encode resolve request: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/resolve", &requestBody).WithContext(ctx)
	resolveResponse := httptest.NewRecorder()
	h.ServeHTTP(resolveResponse, req)
	if resolveResponse.Code != http.StatusOK {
		t.Fatalf("POST /resolve status = %d, want 200 resolution result, body=%s", resolveResponse.Code, resolveResponse.Body.String())
	}
	var resolveBody struct {
		Status      string `json:"status"`
		Diagnostics []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"diagnostics"`
		LockfileRef string `json:"lockfile_ref"`
	}
	if err := json.Unmarshal(resolveResponse.Body.Bytes(), &resolveBody); err != nil {
		t.Fatalf("decode POST /resolve response: %v", err)
	}
	if resolveBody.Status != "failed" {
		t.Errorf("POST /resolve status field = %q, want failed; body=%s", resolveBody.Status, resolveResponse.Body.String())
	}
	if len(resolveBody.Diagnostics) == 0 || resolveBody.Diagnostics[0].Type == "" || !strings.Contains(strings.ToLower(resolveBody.Diagnostics[0].Message), "cancel") {
		t.Errorf("POST /resolve diagnostics = %#v, want a typed context-cancellation diagnostic", resolveBody.Diagnostics)
	}
	if resolveBody.LockfileRef != "" {
		t.Errorf("POST /resolve lockfile_ref = %q, want empty for canceled resolution", resolveBody.LockfileRef)
	}

	listResponse := doReq(t, h, http.MethodGet, "/resolutions", nil)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("GET /resolutions status = %d, body=%s", listResponse.Code, listResponse.Body.String())
	}
	var resolutions []struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &resolutions); err != nil {
		t.Fatalf("decode GET /resolutions response: %v", err)
	}
	if len(resolutions) != 1 {
		t.Fatalf("GET /resolutions returned %d records, want 1; body=%s", len(resolutions), listResponse.Body.String())
	}
	requestID := resolutions[0].ID

	detailResponse := doReq(t, h, http.MethodGet, fmt.Sprintf("/resolutions/%d", requestID), nil)
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("GET /resolutions/%d status = %d, body=%s", requestID, detailResponse.Code, detailResponse.Body.String())
	}
	var detailBody struct {
		Request struct {
			Status       string `json:"status"`
			ErrorCode    string `json:"error_code"`
			ErrorMessage string `json:"error_message"`
		} `json:"request"`
		Nodes []json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("decode resolution detail: %v", err)
	}
	if detailBody.Request.Status != "failed" {
		t.Errorf("persisted resolution status = %q, want failed", detailBody.Request.Status)
	}
	if detailBody.Request.ErrorCode == "" || !strings.Contains(strings.ToLower(detailBody.Request.ErrorMessage), "cancel") {
		t.Errorf("persisted resolution error = (%q, %q), want a typed context-cancellation error", detailBody.Request.ErrorCode, detailBody.Request.ErrorMessage)
	}
	if len(detailBody.Nodes) != 0 {
		t.Errorf("persisted resolution nodes = %d, want 0 for request canceled before resolution", len(detailBody.Nodes))
	}

	lockfileResponse := doReq(t, h, http.MethodGet, fmt.Sprintf("/resolutions/%d/lockfile", requestID), nil)
	if lockfileResponse.Code != http.StatusNotFound {
		t.Errorf("GET /resolutions/%d/lockfile status = %d, want 404; body=%s", requestID, lockfileResponse.Code, lockfileResponse.Body.String())
	} else {
		var errorBody struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(lockfileResponse.Body.Bytes(), &errorBody); err != nil {
			t.Fatalf("decode lockfile error response: %v", err)
		}
		if errorBody.Error.Code != "NOT_FOUND" || !strings.Contains(strings.ToLower(errorBody.Error.Message), "lockfile") {
			t.Errorf("lockfile error = (%q, %q), want NOT_FOUND with a lockfile message", errorBody.Error.Code, errorBody.Error.Message)
		}
	}
}
