package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"artifact-resolver/internal/model"
	"artifact-resolver/internal/resolver"
	"artifact-resolver/internal/service"
)

func createArtifactForStateTest(t *testing.T, h http.Handler, name string) model.Artifact {
	t.Helper()
	w := doReq(t, h, http.MethodPost, "/artifacts", map[string]string{"name": name})
	if w.Code != http.StatusCreated {
		t.Fatalf("create %s status = %d, body=%s", name, w.Code, w.Body.String())
	}
	var art model.Artifact
	if err := json.Unmarshal(w.Body.Bytes(), &art); err != nil {
		t.Fatalf("decode artifact %s: %v", name, err)
	}
	return art
}

func createVersionForStateTest(t *testing.T, h http.Handler, name, version string) model.Version {
	t.Helper()
	w := doReq(t, h, http.MethodPost, "/artifacts/"+name+"/versions", map[string]string{"version": version})
	if w.Code != http.StatusCreated {
		t.Fatalf("create %s@%s status = %d, body=%s", name, version, w.Code, w.Body.String())
	}
	var v model.Version
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode version %s@%s: %v", name, version, err)
	}
	return v
}

func publishVersionForStateTest(t *testing.T, h http.Handler, name, version string) {
	t.Helper()
	w := doReq(t, h, http.MethodPost, "/artifacts/"+name+"/versions/"+version+"/publish", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("publish %s@%s status = %d, body=%s", name, version, w.Code, w.Body.String())
	}
}

func assertSingleGraphNode(t *testing.T, graph []resolver.Node, name, version string) {
	t.Helper()
	if len(graph) != 1 {
		t.Fatalf("graph length = %d, want one node (%s@%s): %#v", len(graph), name, version, graph)
	}
	if graph[0].Name != name || graph[0].Version != version || graph[0].Depth != 0 {
		t.Fatalf("graph node = %#v, want %s@%s at depth 0", graph[0], name, version)
	}
}

func TestResolveRequestsKeepIndependentResults(t *testing.T) {
	h := newTestServer(t)
	a := createArtifactForStateTest(t, h, "a")
	b := createArtifactForStateTest(t, h, "b")
	aVersion := createVersionForStateTest(t, h, "a", "1.0.0")
	bVersion := createVersionForStateTest(t, h, "b", "1.0.0")
	publishVersionForStateTest(t, h, "a", "1.0.0")
	publishVersionForStateTest(t, h, "b", "1.0.0")

	first := doReq(t, h, http.MethodPost, "/resolve", map[string]any{
		"manifest": []service.ManifestItem{{Name: "a", Constraint: "^1.0.0"}},
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first resolve status = %d, body=%s", first.Code, first.Body.String())
	}
	var firstOut service.ResolveOutput
	if err := json.Unmarshal(first.Body.Bytes(), &firstOut); err != nil {
		t.Fatalf("decode first resolve: %v", err)
	}
	if firstOut.Status != "succeeded" {
		t.Fatalf("first resolve status field = %q", firstOut.Status)
	}
	assertSingleGraphNode(t, firstOut.Graph, "a", "1.0.0")

	second := doReq(t, h, http.MethodPost, "/resolve", map[string]any{
		"manifest": []service.ManifestItem{{Name: "b", Constraint: "^1.0.0"}},
	})
	if second.Code != http.StatusOK {
		t.Fatalf("second resolve status = %d, body=%s", second.Code, second.Body.String())
	}
	var secondOut service.ResolveOutput
	if err := json.Unmarshal(second.Body.Bytes(), &secondOut); err != nil {
		t.Fatalf("decode second resolve: %v", err)
	}
	if secondOut.Status != "succeeded" {
		t.Fatalf("second resolve status field = %q, diagnostics=%#v", secondOut.Status, secondOut.Diagnostics)
	}
	assertSingleGraphNode(t, secondOut.Graph, "b", "1.0.0")

	resolution := doReq(t, h, http.MethodGet, fmt.Sprintf("/resolutions/%d", secondOut.RequestID), nil)
	if resolution.Code != http.StatusOK {
		t.Fatalf("get second resolution status = %d, body=%s", resolution.Code, resolution.Body.String())
	}
	var detail struct {
		Request model.ResolutionRequest `json:"request"`
		Nodes   []model.ResolutionNode  `json:"nodes"`
	}
	if err := json.Unmarshal(resolution.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode second resolution: %v", err)
	}
	if detail.Request.Status != model.ResSucceeded {
		t.Fatalf("stored second resolution status = %q", detail.Request.Status)
	}
	var storedGraph []resolver.Node
	if err := json.Unmarshal([]byte(detail.Request.GraphJSON), &storedGraph); err != nil {
		t.Fatalf("decode stored graph: %v", err)
	}
	assertSingleGraphNode(t, storedGraph, "b", "1.0.0")
	if len(detail.Nodes) != 1 || detail.Nodes[0].ArtifactID != b.ID || detail.Nodes[0].VersionID != bVersion.ID {
		t.Fatalf("stored resolution nodes = %#v, want only b@1.0.0 (artifact %d, version %d)", detail.Nodes, b.ID, bVersion.ID)
	}
	if detail.Nodes[0].ArtifactID == a.ID || detail.Nodes[0].VersionID == aVersion.ID {
		t.Fatalf("stored resolution nodes contain first request node: %#v", detail.Nodes)
	}

	lockfile := doReq(t, h, http.MethodGet, "/lockfiles/"+secondOut.LockfileRef, nil)
	if lockfile.Code != http.StatusOK {
		t.Fatalf("get second lockfile status = %d, body=%s", lockfile.Code, lockfile.Body.String())
	}
	var lock service.LockfileOutput
	if err := json.Unmarshal(lockfile.Body.Bytes(), &lock); err != nil {
		t.Fatalf("decode second lockfile: %v", err)
	}
	if !lock.Verified {
		t.Fatal("second lockfile is not verified")
	}
	if len(lock.Lockfile.Entries) != 1 || lock.Lockfile.Entries[0].Name != "b" || lock.Lockfile.Entries[0].Version != "1.0.0" {
		t.Fatalf("second lockfile entries = %#v, want only b@1.0.0", lock.Lockfile.Entries)
	}
}

func TestResolveRequestsRefreshSelectionsAndDiagnostics(t *testing.T) {
	t.Run("selected version", func(t *testing.T) {
		h := newTestServer(t)
		createArtifactForStateTest(t, h, "a")
		createVersionForStateTest(t, h, "a", "1.0.0")
		secondVersion := createVersionForStateTest(t, h, "a", "2.0.0")
		publishVersionForStateTest(t, h, "a", "1.0.0")
		publishVersionForStateTest(t, h, "a", "2.0.0")

		first := doReq(t, h, http.MethodPost, "/resolve", map[string]any{
			"manifest": []service.ManifestItem{{Name: "a", Constraint: "1.0.0"}},
		})
		if first.Code != http.StatusOK {
			t.Fatalf("first pinned resolve status = %d, body=%s", first.Code, first.Body.String())
		}

		second := doReq(t, h, http.MethodPost, "/resolve", map[string]any{
			"manifest": []service.ManifestItem{{Name: "a", Constraint: "2.0.0"}},
		})
		if second.Code != http.StatusOK {
			t.Fatalf("second pinned resolve status = %d, body=%s", second.Code, second.Body.String())
		}
		var out service.ResolveOutput
		if err := json.Unmarshal(second.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode second pinned resolve: %v", err)
		}
		if out.Status != "succeeded" {
			t.Errorf("second pinned resolve status field = %q, diagnostics=%#v", out.Status, out.Diagnostics)
		}
		assertSingleGraphNode(t, out.Graph, "a", "2.0.0")

		detailResponse := doReq(t, h, http.MethodGet, fmt.Sprintf("/resolutions/%d", out.RequestID), nil)
		if detailResponse.Code != http.StatusOK {
			t.Fatalf("get second pinned resolution status = %d, body=%s", detailResponse.Code, detailResponse.Body.String())
		}
		var detail struct {
			Nodes []model.ResolutionNode `json:"nodes"`
		}
		if err := json.Unmarshal(detailResponse.Body.Bytes(), &detail); err != nil {
			t.Fatalf("decode second pinned resolution: %v", err)
		}
		if len(detail.Nodes) != 1 || detail.Nodes[0].VersionID != secondVersion.ID {
			t.Fatalf("second pinned resolution nodes = %#v, want only version id %d", detail.Nodes, secondVersion.ID)
		}
	})

	t.Run("diagnostics", func(t *testing.T) {
		h := newTestServer(t)
		createArtifactForStateTest(t, h, "b")
		createVersionForStateTest(t, h, "b", "1.0.0")
		publishVersionForStateTest(t, h, "b", "1.0.0")

		failed := doReq(t, h, http.MethodPost, "/resolve", map[string]any{
			"manifest": []service.ManifestItem{{Name: "missing", Constraint: "^1.0.0"}},
		})
		if failed.Code != http.StatusOK {
			t.Fatalf("failed resolve HTTP status = %d, body=%s", failed.Code, failed.Body.String())
		}
		var failedOut service.ResolveOutput
		if err := json.Unmarshal(failed.Body.Bytes(), &failedOut); err != nil {
			t.Fatalf("decode failed resolve: %v", err)
		}
		if failedOut.Status != "failed" || len(failedOut.Diagnostics) != 1 || failedOut.Diagnostics[0].Type != "MISSING" {
			t.Fatalf("failed resolve = %#v, want one MISSING diagnostic", failedOut)
		}

		succeeded := doReq(t, h, http.MethodPost, "/resolve", map[string]any{
			"manifest": []service.ManifestItem{{Name: "b", Constraint: "^1.0.0"}},
		})
		if succeeded.Code != http.StatusOK {
			t.Fatalf("resolve after diagnostic status = %d, body=%s", succeeded.Code, succeeded.Body.String())
		}
		var succeededOut service.ResolveOutput
		if err := json.Unmarshal(succeeded.Body.Bytes(), &succeededOut); err != nil {
			t.Fatalf("decode resolve after diagnostic: %v", err)
		}
		if succeededOut.Status != "succeeded" {
			t.Errorf("resolve after diagnostic status field = %q", succeededOut.Status)
		}
		if len(succeededOut.Diagnostics) != 0 {
			t.Errorf("resolve after diagnostic returned stale diagnostics: %#v", succeededOut.Diagnostics)
		}
		assertSingleGraphNode(t, succeededOut.Graph, "b", "1.0.0")
		if succeededOut.LockfileRef == "" {
			t.Error("successful resolve after diagnostic has no lockfile reference")
		}
	})
}

func TestDependencyReplacementAuditUsesCurrentSnapshot(t *testing.T) {
	h := newTestServer(t)
	createArtifactForStateTest(t, h, "a")
	b := createArtifactForStateTest(t, h, "b")
	createArtifactForStateTest(t, h, "root")
	createVersionForStateTest(t, h, "a", "1.0.0")
	createVersionForStateTest(t, h, "b", "1.0.0")
	rootVersion := createVersionForStateTest(t, h, "root", "1.0.0")
	publishVersionForStateTest(t, h, "a", "1.0.0")
	publishVersionForStateTest(t, h, "b", "1.0.0")

	// Establish the previous dependency through the same handler path that writes its audit record.
	initial := doReq(t, h, http.MethodPut, "/artifacts/root/versions/1.0.0/dependencies", map[string]any{
		"dependencies": []service.DependencyItem{{Name: "b", Constraint: "^1.0.0"}},
	})
	if initial.Code != http.StatusOK {
		t.Fatalf("initial dependency replacement status = %d, body=%s", initial.Code, initial.Body.String())
	}

	seed := doReq(t, h, http.MethodPost, "/resolve", map[string]any{
		"manifest": []service.ManifestItem{{Name: "a", Constraint: "^1.0.0"}},
	})
	if seed.Code != http.StatusOK {
		t.Fatalf("seed resolve status = %d, body=%s", seed.Code, seed.Body.String())
	}

	replaced := doReq(t, h, http.MethodPut, "/artifacts/root/versions/1.0.0/dependencies", map[string]any{
		"dependencies": []service.DependencyItem{},
	})
	if replaced.Code != http.StatusOK {
		t.Fatalf("dependency replacement status = %d, body=%s", replaced.Code, replaced.Body.String())
	}
	var replacedBody struct {
		Dependencies []model.DependencyTarget `json:"dependencies"`
	}
	if err := json.Unmarshal(replaced.Body.Bytes(), &replacedBody); err != nil {
		t.Fatalf("decode dependency replacement: %v", err)
	}
	if len(replacedBody.Dependencies) != 0 {
		t.Fatalf("replacement dependencies = %#v, want empty list", replacedBody.Dependencies)
	}

	changes := doReq(t, h, http.MethodGet, fmt.Sprintf("/changes?entity_type=dependency&entity_id=%d", rootVersion.ID), nil)
	if changes.Code != http.StatusOK {
		t.Fatalf("get dependency changes status = %d, body=%s", changes.Code, changes.Body.String())
	}
	var records []model.ChangeRecord
	if err := json.Unmarshal(changes.Body.Bytes(), &records); err != nil {
		t.Fatalf("decode dependency changes: %v", err)
	}
	if len(records) < 2 || records[0].Action != "replace-resolve" {
		t.Fatalf("dependency audit records = %#v, want latest replace-resolve record", records)
	}
	var audited []resolver.Node
	if err := json.Unmarshal([]byte(records[0].AfterJSON), &audited); err != nil {
		t.Fatalf("decode audited graph: %v", err)
	}
	assertSingleGraphNode(t, audited, "b", "1.0.0")
	if audited[0].Name != b.Name {
		t.Fatalf("audited graph node = %#v, want dependency target %q", audited[0], b.Name)
	}
}
