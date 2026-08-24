package resolver

import (
	"errors"
	"testing"

	"artifact-resolver/internal/model"
)

var errNotFound = errors.New("not found")

func buildCatalog() Catalog {
	// 制品 liba, libb, libc
	liba := model.Artifact{ID: 1, Name: "liba"}
	libb := model.Artifact{ID: 2, Name: "libb"}
	libc := model.Artifact{ID: 3, Name: "libc"}

	versions := map[int64][]model.Version{
		1: {
			{ID: 10, ArtifactID: 1, Version: "1.0.0", Status: model.StatusPublished},
			{ID: 11, ArtifactID: 1, Version: "1.2.0", Status: model.StatusPublished},
		},
		2: {
			{ID: 20, ArtifactID: 2, Version: "1.0.0", Status: model.StatusPublished},
		},
		3: {
			{ID: 30, ArtifactID: 3, Version: "0.5.0", Status: model.StatusPublished},
		},
	}
	deps := map[int64][]model.DependencyTarget{
		// liba 1.2.0 依赖 libb ^1.0.0
		11: {
			{Dependency: model.Dependency{ID: 100, FromVersionID: 11, ToArtifactID: 2, Constraint: "^1.0.0"}, ToArtifactName: "libb"},
		},
		// libb 1.0.0 依赖 libc >=0.4.0
		20: {
			{Dependency: model.Dependency{ID: 101, FromVersionID: 20, ToArtifactID: 3, Constraint: ">=0.4.0"}, ToArtifactName: "libc"},
		},
	}

	byName := map[string]model.Artifact{"liba": liba, "libb": libb, "libc": libc}

	return Catalog{
		PublishedVersions: func(id int64) ([]model.Version, error) {
			var out []model.Version
			for _, v := range versions[id] {
				if v.Status == model.StatusPublished {
					out = append(out, v)
				}
			}
			return out, nil
		},
		AllPublishedVersions: func(id int64) ([]model.Version, error) {
			var out []model.Version
			for _, v := range versions[id] {
				if v.Status == model.StatusPublished {
					out = append(out, v)
				}
			}
			return out, nil
		},
		ArtifactByName: func(name string) (model.Artifact, error) {
			a, ok := byName[name]
			if !ok {
				return model.Artifact{}, errNotFound
			}
			return a, nil
		},
		DependenciesFor: func(id int64) ([]model.DependencyTarget, error) {
			return deps[id], nil
		},
	}
}

func TestResolveHighestVersion(t *testing.T) {
	e := New(buildCatalog())
	res := e.Resolve([]ManifestItem{{Name: "liba", Constraint: "^1.0.0"}})
	if res.Status != "succeeded" {
		t.Fatalf("expected succeeded, got %s: %v", res.Status, res.Diagnostics)
	}
	if len(res.Graph) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(res.Graph))
	}
	if res.Graph[0].Version != "1.2.0" {
		t.Errorf("expected liba 1.2.0, got %s", res.Graph[0].Version)
	}
}

func TestResolveMissingDependency(t *testing.T) {
	e := New(buildCatalog())
	res := e.Resolve([]ManifestItem{{Name: "nonexistent", Constraint: "^1.0.0"}})
	if res.Status != "failed" {
		t.Fatalf("expected failed, got %s", res.Status)
	}
	if len(res.Diagnostics) == 0 || res.Diagnostics[0].Type != "MISSING" {
		t.Fatalf("expected MISSING diagnostic, got %v", res.Diagnostics)
	}
}

func TestResolveCycle(t *testing.T) {
	liba := model.Artifact{ID: 1, Name: "liba"}
	libb := model.Artifact{ID: 2, Name: "libb"}
	versions := map[int64][]model.Version{
		1: {{ID: 10, ArtifactID: 1, Version: "1.0.0", Status: model.StatusPublished}},
		2: {{ID: 20, ArtifactID: 2, Version: "1.0.0", Status: model.StatusPublished}},
	}
	deps := map[int64][]model.DependencyTarget{
		10: {{Dependency: model.Dependency{ToArtifactID: 2, Constraint: "^1.0.0"}, ToArtifactName: "libb"}},
		20: {{Dependency: model.Dependency{ToArtifactID: 1, Constraint: "^1.0.0"}, ToArtifactName: "liba"}},
	}
	cat := Catalog{
		PublishedVersions:    func(id int64) ([]model.Version, error) { return versions[id], nil },
		AllPublishedVersions: func(id int64) ([]model.Version, error) { return versions[id], nil },
		ArtifactByName: func(name string) (model.Artifact, error) {
			if name == "liba" {
				return liba, nil
			}
			if name == "libb" {
				return libb, nil
			}
			return model.Artifact{}, errNotFound
		},
		DependenciesFor: func(id int64) ([]model.DependencyTarget, error) { return deps[id], nil },
	}
	e := New(cat)
	res := e.Resolve([]ManifestItem{{Name: "liba", Constraint: "^1.0.0"}})
	if res.Status != "failed" {
		t.Fatalf("expected failed, got %s", res.Status)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Type == "CYCLE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected CYCLE diagnostic, got %v", res.Diagnostics)
	}
}

func TestResolveConflict(t *testing.T) {
	e := New(buildCatalog())
	res := e.Resolve([]ManifestItem{
		{Name: "liba", Constraint: "^1.0.0"},
		{Name: "liba", Constraint: "^2.0.0"},
	})
	if res.Status != "failed" {
		t.Fatalf("expected failed, got %s", res.Status)
	}
	found := false
	for _, d := range res.Diagnostics {
		if d.Type == "CONFLICT" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected CONFLICT diagnostic, got %v", res.Diagnostics)
	}
}
