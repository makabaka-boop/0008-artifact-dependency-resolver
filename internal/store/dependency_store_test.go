package store

import (
	"testing"
	"time"

	"artifact-resolver/internal/model"
)

func TestListDependenciesAfterReplacingWithShorterList(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}
	defer st.Close()

	pkg, err := st.CreateArtifact(ArtifactInput{Name: "pkg"})
	if err != nil {
		t.Fatalf("create pkg artifact: %v", err)
	}
	depA, err := st.CreateArtifact(ArtifactInput{Name: "dep-a"})
	if err != nil {
		t.Fatalf("create dep-a artifact: %v", err)
	}
	depB, err := st.CreateArtifact(ArtifactInput{Name: "dep-b"})
	if err != nil {
		t.Fatalf("create dep-b artifact: %v", err)
	}

	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := st.db.Exec(
		`INSERT INTO versions(artifact_id, version, status, created_at, updated_at) VALUES (?,?,?,?,?)`,
		pkg.ID, "1.0.0", "draft", stamp, stamp,
	)
	if err != nil {
		t.Fatalf("create draft version: %v", err)
	}
	versionID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read version id: %v", err)
	}

	if _, err := st.CreateDependency(versionID, depA.ID, ">=1.0.0"); err != nil {
		t.Fatalf("create dep-a declaration: %v", err)
	}
	if _, err := st.CreateDependency(versionID, depB.ID, ">=1.0.0"); err != nil {
		t.Fatalf("create dep-b declaration: %v", err)
	}

	initial, err := st.ListDependencies(versionID)
	if err != nil {
		t.Fatalf("list initial dependencies: %v", err)
	}
	assertDependencyNames(t, initial, []string{"dep-a", "dep-b"})

	if err := st.DeleteDependenciesFor(versionID); err != nil {
		t.Fatalf("replace dependencies by deleting old list: %v", err)
	}
	if _, err := st.CreateDependency(versionID, depA.ID, ">=1.0.0"); err != nil {
		t.Fatalf("create replacement dep-a declaration: %v", err)
	}

	replaced, err := st.ListDependencies(versionID)
	if err != nil {
		t.Fatalf("list dependencies after replacement: %v", err)
	}
	assertDependencyNames(t, replaced, []string{"dep-a"})

	queried, err := st.ListDependencies(versionID)
	if err != nil {
		t.Fatalf("list dependencies again: %v", err)
	}
	assertDependencyNames(t, queried, []string{"dep-a"})

	var persisted int
	if err := st.db.QueryRow(
		`SELECT COUNT(*) FROM dependencies WHERE from_version_id = ?`, versionID,
	).Scan(&persisted); err != nil {
		t.Fatalf("count persisted dependencies: %v", err)
	}
	if persisted != 1 {
		t.Fatalf("persisted dependency count = %d, want 1", persisted)
	}
}

func assertDependencyNames(t *testing.T, deps []model.DependencyTarget, want []string) {
	t.Helper()
	if len(deps) != len(want) {
		t.Fatalf("dependency count = %d, want %d (%v)", len(deps), len(want), want)
	}
	for i, name := range want {
		if deps[i].ToArtifactName != name {
			t.Errorf("dependency %d name = %q, want %q", i, deps[i].ToArtifactName, name)
		}
	}
}
