package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"artifact-resolver/internal/errcode"
	"artifact-resolver/internal/model"
	"artifact-resolver/internal/resolver"
	"artifact-resolver/internal/service"
	"artifact-resolver/internal/store"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	_, _, handler := newTestStack(t)
	return handler
}

func newTestStack(t *testing.T) (*store.Store, *service.Service, http.Handler) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	svc := service.New(st)
	return st, svc, NewServer(svc).Handler()
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

func TestReplaceDependenciesFailurePreservesPreviousStateAcrossReaders(t *testing.T) {
	st, svc, h := newTestStack(t)
	ctx := context.Background()

	for _, name := range []string{"app", "stable-lib", "candidate-lib"} {
		if _, err := svc.CreateArtifact(ctx, name, ""); err != nil {
			t.Fatalf("create artifact %s: %v", name, err)
		}
	}
	if _, err := svc.CreateVersion(ctx, "stable-lib", "1.0.0"); err != nil {
		t.Fatalf("create stable-lib version: %v", err)
	}
	if _, err := svc.PublishVersion(ctx, "stable-lib", "1.0.0"); err != nil {
		t.Fatalf("publish stable-lib version: %v", err)
	}
	appVersion, err := svc.CreateVersion(ctx, "app", "1.0.0")
	if err != nil {
		t.Fatalf("create app version: %v", err)
	}
	if _, err := svc.ReplaceDependencies(ctx, "app", "1.0.0", []service.DependencyItem{
		{Name: "stable-lib", Constraint: "^1.0.0"},
	}); err != nil {
		t.Fatalf("seed previous dependencies: %v", err)
	}

	dependencyPath := "/artifacts/app/versions/1.0.0/dependencies"
	w := doReq(t, h, http.MethodPut, dependencyPath, map[string]any{
		"dependencies": []map[string]string{
			{"name": "candidate-lib", "constraint": "^1.0.0"},
			{"name": "candidate-lib", "constraint": ">=2.0.0"},
		},
	})
	if w.Code != http.StatusInternalServerError {
		t.Errorf("conflicting replacement status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	var failure struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &failure); err != nil {
		t.Fatalf("decode replacement error: %v", err)
	}
	if failure.Error.Code != string(errcode.CodeInternal) {
		t.Errorf("replacement error code = %q, want %q", failure.Error.Code, errcode.CodeInternal)
	}
	if !strings.Contains(failure.Error.Message, "duplicate dependency") {
		t.Errorf("replacement error message = %q, want duplicate dependency detail", failure.Error.Message)
	}

	assertPrevious := func(reader string, dependencies []model.DependencyTarget) {
		t.Helper()
		if len(dependencies) != 1 {
			t.Errorf("%s dependencies = %v, want one previous dependency", reader, dependencies)
			return
		}
		got := dependencies[0]
		if got.ToArtifactName != "stable-lib" || got.Constraint != "^1.0.0" {
			t.Errorf("%s dependency = %s %s, want stable-lib ^1.0.0", reader, got.ToArtifactName, got.Constraint)
		}
	}

	w = doReq(t, h, http.MethodGet, dependencyPath, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list dependencies status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var listed struct {
		Dependencies []model.DependencyTarget `json:"dependencies"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode listed dependencies: %v", err)
	}
	assertPrevious("GET API", listed.Dependencies)

	serviceDependencies, err := svc.ListDependencies(ctx, "app", "1.0.0")
	if err != nil {
		t.Fatalf("service list dependencies: %v", err)
	}
	assertPrevious("service", serviceDependencies)

	storedDependencies, err := st.ListDependencies(appVersion.ID)
	if err != nil {
		t.Fatalf("store list dependencies: %v", err)
	}
	assertPrevious("store", storedDependencies)

	engine := resolver.New(resolver.Catalog{
		PublishedVersions:    st.PublishedVersions,
		AllPublishedVersions: st.AllPublishedVersions,
		ArtifactByName:       st.GetArtifactByName,
		DependenciesFor:      st.ListDependencies,
	})
	state, err := engine.RefreshDependencies(appVersion.ID)
	if err != nil {
		t.Fatalf("resolver refresh dependencies: %v", err)
	}
	assertPrevious("resolver", state.Dependencies)
	if state.Resolution.Status != "succeeded" || len(state.Resolution.Diagnostics) != 0 {
		t.Errorf("resolver status = %s, diagnostics=%v; want succeeded without diagnostics", state.Resolution.Status, state.Resolution.Diagnostics)
	}
	if len(state.Resolution.Graph) != 1 || state.Resolution.Graph[0].Name != "stable-lib" {
		t.Errorf("resolver graph = %v, want only stable-lib", state.Resolution.Graph)
	}

	w = doReq(t, h, http.MethodGet, "/artifacts/app/versions/1.0.0/readiness", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("readiness status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var readiness service.ReadinessOutput
	if err := json.Unmarshal(w.Body.Bytes(), &readiness); err != nil {
		t.Fatalf("decode readiness: %v", err)
	}
	if !readiness.Ready || len(readiness.Blockers) != 0 {
		t.Errorf("readiness = ready:%t blockers:%v, want ready with no blockers", readiness.Ready, readiness.Blockers)
	}

	if _, err := svc.PublishVersion(ctx, "app", "1.0.0"); err != nil {
		t.Fatalf("publish app version: %v", err)
	}
	w = doReq(t, h, http.MethodPost, "/resolve", map[string]any{
		"manifest": []map[string]string{{"name": "app", "constraint": "1.0.0"}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("resolve status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resolution service.ResolveOutput
	if err := json.Unmarshal(w.Body.Bytes(), &resolution); err != nil {
		t.Fatalf("decode resolution: %v", err)
	}
	if resolution.Status != "succeeded" || len(resolution.Diagnostics) != 0 {
		t.Errorf("resolution status = %s, diagnostics=%v; want succeeded without diagnostics", resolution.Status, resolution.Diagnostics)
	}
	if len(resolution.Graph) != 2 || resolution.Graph[0].Name != "app" || resolution.Graph[1].Name != "stable-lib" {
		t.Errorf("resolution graph = %v, want app followed by stable-lib", resolution.Graph)
	}
	if resolution.LockfileRef == "" {
		t.Errorf("resolution lockfile_ref is empty, want a lockfile for the restored graph")
	}

	w = doReq(t, h, http.MethodGet, "/resolutions/"+strconv.FormatInt(resolution.RequestID, 10)+"/lockfile", nil)
	if w.Code != http.StatusOK {
		t.Errorf("lockfile status = %d, want 200; body=%s", w.Code, w.Body.String())
		return
	}
	var lockfile service.LockfileOutput
	if err := json.Unmarshal(w.Body.Bytes(), &lockfile); err != nil {
		t.Fatalf("decode lockfile: %v", err)
	}
	if !lockfile.Verified {
		t.Error("lockfile is not verified")
	}
	entries := lockfile.Lockfile.Entries
	if len(entries) != 2 || entries[0].Name != "app" || entries[1].Name != "stable-lib" {
		t.Errorf("lockfile entries = %v, want app followed by stable-lib", entries)
	}
}
