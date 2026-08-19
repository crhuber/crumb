package sync

import (
	"reflect"
	"testing"

	"crumb/pkg/storage"
)

func entry(value string, updated string) storage.SecretEntry {
	return storage.SecretEntry{Value: value, Updated: updated}
}

func TestMerge_UnchangedOnBothSides(t *testing.T) {
	base := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	local := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	remote := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}

	merged, conflicts := Merge(base, local, remote)

	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", conflicts)
	}
	if !reflect.DeepEqual(merged, base) {
		t.Fatalf("expected merged to equal base, got %v", merged)
	}
}

func TestMerge_LocalOnlyChange(t *testing.T) {
	base := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	local := storage.SecretStore{"/a": entry("2", "2026-01-02T00:00:00Z")}
	remote := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}

	merged, conflicts := Merge(base, local, remote)

	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", conflicts)
	}
	if merged["/a"].Value != "2" {
		t.Fatalf("expected local's change to win, got %q", merged["/a"].Value)
	}
}

func TestMerge_RemoteOnlyChange(t *testing.T) {
	base := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	local := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	remote := storage.SecretStore{"/a": entry("3", "2026-01-03T00:00:00Z")}

	merged, conflicts := Merge(base, local, remote)

	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", conflicts)
	}
	if merged["/a"].Value != "3" {
		t.Fatalf("expected remote's change to win, got %q", merged["/a"].Value)
	}
}

// The actual regression this feature exists to fix: two machines editing
// *different* keys while both offline must not clobber each other.
func TestMerge_DifferentKeysBothChangeIndependently(t *testing.T) {
	base := storage.SecretStore{
		"/a": entry("1", "2026-01-01T00:00:00Z"),
		"/b": entry("1", "2026-01-01T00:00:00Z"),
	}
	local := storage.SecretStore{
		"/a": entry("local-edit", "2026-01-02T00:00:00Z"),
		"/b": entry("1", "2026-01-01T00:00:00Z"),
	}
	remote := storage.SecretStore{
		"/a": entry("1", "2026-01-01T00:00:00Z"),
		"/b": entry("remote-edit", "2026-01-03T00:00:00Z"),
	}

	merged, conflicts := Merge(base, local, remote)

	if len(conflicts) != 0 {
		t.Fatalf("expected no true conflicts, got %v", conflicts)
	}
	if merged["/a"].Value != "local-edit" {
		t.Fatalf("expected /a to keep local's edit, got %q", merged["/a"].Value)
	}
	if merged["/b"].Value != "remote-edit" {
		t.Fatalf("expected /b to keep remote's edit, got %q", merged["/b"].Value)
	}
}

func TestMerge_SameKeyConflictNewerWins(t *testing.T) {
	base := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	local := storage.SecretStore{"/a": entry("local", "2026-01-02T00:00:00Z")}
	remote := storage.SecretStore{"/a": entry("remote", "2026-01-05T00:00:00Z")}

	merged, conflicts := Merge(base, local, remote)

	if len(conflicts) != 1 || conflicts[0] != "/a" {
		t.Fatalf("expected /a to be reported as a conflict, got %v", conflicts)
	}
	if merged["/a"].Value != "remote" {
		t.Fatalf("expected the newer (remote) write to win, got %q", merged["/a"].Value)
	}
}

func TestMerge_SameKeyConflictLocalNewerWins(t *testing.T) {
	base := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	local := storage.SecretStore{"/a": entry("local", "2026-01-09T00:00:00Z")}
	remote := storage.SecretStore{"/a": entry("remote", "2026-01-05T00:00:00Z")}

	merged, _ := Merge(base, local, remote)

	if merged["/a"].Value != "local" {
		t.Fatalf("expected the newer (local) write to win, got %q", merged["/a"].Value)
	}
}

func TestMerge_DeleteVersusEdit_EditWins(t *testing.T) {
	base := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	local := storage.SecretStore{} // deleted locally
	remote := storage.SecretStore{"/a": entry("edited", "2026-01-05T00:00:00Z")}

	merged, conflicts := Merge(base, local, remote)

	if len(conflicts) != 1 {
		t.Fatalf("expected /a to be reported as a conflict, got %v", conflicts)
	}
	got, ok := merged["/a"]
	if !ok || got.Value != "edited" {
		t.Fatalf("expected the concurrent edit to survive a concurrent delete, got %v (present=%v)", got, ok)
	}
}

func TestMerge_DeletedOnBothSidesStaysDeleted(t *testing.T) {
	base := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	local := storage.SecretStore{}
	remote := storage.SecretStore{}

	merged, _ := Merge(base, local, remote)

	if _, ok := merged["/a"]; ok {
		t.Fatalf("expected /a to remain deleted, got %v", merged["/a"])
	}
}

// First-ever sync: no base yet (empty store), nothing on the server either.
func TestMerge_FirstSyncNoBase(t *testing.T) {
	base := storage.SecretStore{}
	local := storage.SecretStore{"/a": entry("new", "2026-01-01T00:00:00Z")}
	remote := storage.SecretStore{}

	merged, conflicts := Merge(base, local, remote)

	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", conflicts)
	}
	if merged["/a"].Value != "new" {
		t.Fatalf("expected local's new secret to survive, got %v", merged)
	}
}

func TestStoresEqual_SameContentDifferentUpdated(t *testing.T) {
	a := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	b := storage.SecretStore{"/a": entry("1", "2026-02-02T00:00:00Z")}

	if !storesEqual(a, b) {
		t.Fatalf("expected stores with same Value/Expires but different Updated to be equal")
	}
}

func TestStoresEqual_DifferentValue(t *testing.T) {
	a := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	b := storage.SecretStore{"/a": entry("2", "2026-01-01T00:00:00Z")}

	if storesEqual(a, b) {
		t.Fatalf("expected stores with different values to be unequal")
	}
}

func TestStoresEqual_DifferentKeys(t *testing.T) {
	a := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}
	b := storage.SecretStore{"/b": entry("1", "2026-01-01T00:00:00Z")}

	if storesEqual(a, b) {
		t.Fatalf("expected stores with different keys to be unequal")
	}
}

func TestStoresEqual_DifferentLength(t *testing.T) {
	a := storage.SecretStore{
		"/a": entry("1", "2026-01-01T00:00:00Z"),
		"/b": entry("1", "2026-01-01T00:00:00Z"),
	}
	b := storage.SecretStore{"/a": entry("1", "2026-01-01T00:00:00Z")}

	if storesEqual(a, b) {
		t.Fatalf("expected stores of different length to be unequal")
	}
}

func TestStoresEqual_BothEmpty(t *testing.T) {
	if !storesEqual(storage.SecretStore{}, storage.SecretStore{}) {
		t.Fatalf("expected two empty stores to be equal")
	}
}

func TestMerge_BothCreateSameKeyFresh(t *testing.T) {
	base := storage.SecretStore{}
	local := storage.SecretStore{"/a": entry("local", "2026-01-02T00:00:00Z")}
	remote := storage.SecretStore{"/a": entry("remote", "2026-01-05T00:00:00Z")}

	merged, conflicts := Merge(base, local, remote)

	if len(conflicts) != 1 {
		t.Fatalf("expected a conflict when both sides freshly create the same key, got %v", conflicts)
	}
	if merged["/a"].Value != "remote" {
		t.Fatalf("expected the newer creation to win, got %q", merged["/a"].Value)
	}
}
