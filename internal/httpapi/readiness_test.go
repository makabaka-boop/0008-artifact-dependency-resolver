package httpapi

import (
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

func decodeReadiness(t *testing.T, w *httptest.ResponseRecorder) service.ReadinessOutput {
	t.Helper()
	var out service.ReadinessOutput
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode readiness response: %v; body=%s", err, w.Body.String())
	}
	return out
}

func assertLibV2Blocker(t *testing.T, out service.ReadinessOutput) {
	t.Helper()
	if out.Ready {
		t.Fatalf("readiness unexpectedly true: %+v", out)
	}
	for _, blocker := range out.Blockers {
		if strings.Contains(blocker.Reason, `lib`) && strings.Contains(blocker.Reason, `^2.0.0`) {
			return
		}
	}
	t.Fatalf("missing lib ^2.0.0 blocker: %+v", out.Blockers)
}

func assertReadyWithoutBlockers(t *testing.T, out service.ReadinessOutput) {
	t.Helper()
	if !out.Ready {
		t.Fatalf("readiness = false: %+v", out)
	}
	if len(out.Blockers) != 0 {
		t.Fatalf("unexpected blockers: %+v", out.Blockers)
	}
}

func TestReadinessChecksDoNotLeakAcrossDraftVersions(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "readiness.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	svc := service.New(st)
	for _, name := range []string{"demo", "lib"} {
		if _, err := svc.CreateArtifact(ctx, name, ""); err != nil {
			t.Fatalf("create artifact %s: %v", name, err)
		}
	}
	for _, v := range []struct {
		name, version string
	}{
		{"lib", "1.0.0"},
		{"demo", "1.0.0"},
		{"demo", "1.1.0"},
	} {
		if _, err := svc.CreateVersion(ctx, v.name, v.version); err != nil {
			t.Fatalf("create version %s@%s: %v", v.name, v.version, err)
		}
	}
	if _, err := svc.ReplaceDependencies(ctx, "demo", "1.0.0", []service.DependencyItem{{Name: "lib", Constraint: "^2.0.0"}}); err != nil {
		t.Fatalf("replace 1.0.0 dependencies: %v", err)
	}
	if _, err := svc.ReplaceDependencies(ctx, "demo", "1.1.0", []service.DependencyItem{{Name: "lib", Constraint: "^1.0.0"}}); err != nil {
		t.Fatalf("replace 1.1.0 dependencies: %v", err)
	}
	if _, err := svc.PublishVersion(ctx, "lib", "1.0.0"); err != nil {
		t.Fatalf("publish lib@1.0.0: %v", err)
	}

	h := NewServer(svc).Handler()
	path := "/artifacts/demo/versions/%s/readiness"

	first := doReq(t, h, http.MethodGet, fmt.Sprintf(path, "1.0.0"), nil)
	if first.Code != http.StatusOK {
		t.Fatalf("1.0.0 readiness status = %d, body=%s", first.Code, first.Body.String())
	}
	assertLibV2Blocker(t, decodeReadiness(t, first))

	second := doReq(t, h, http.MethodGet, fmt.Sprintf(path, "1.1.0"), nil)
	if second.Code != http.StatusOK {
		t.Fatalf("1.1.0 readiness status = %d, body=%s", second.Code, second.Body.String())
	}
	assertReadyWithoutBlockers(t, decodeReadiness(t, second))

	repeat := doReq(t, h, http.MethodGet, fmt.Sprintf(path, "1.1.0"), nil)
	if repeat.Code != http.StatusOK {
		t.Fatalf("repeated 1.1.0 readiness status = %d, body=%s", repeat.Code, repeat.Body.String())
	}
	assertReadyWithoutBlockers(t, decodeReadiness(t, repeat))

	hAfterRestart := NewServer(service.New(st)).Handler()
	afterRestart := doReq(t, hAfterRestart, http.MethodGet, fmt.Sprintf(path, "1.1.0"), nil)
	if afterRestart.Code != http.StatusOK {
		t.Fatalf("post-restart 1.1.0 readiness status = %d, body=%s", afterRestart.Code, afterRestart.Body.String())
	}
	assertReadyWithoutBlockers(t, decodeReadiness(t, afterRestart))
}
