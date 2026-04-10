package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func newEmptyStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	store, err := NewStore(path)
	require.NoError(t, err)
	return store, path
}

func sampleManifest(id, name string) Manifest {
	return Manifest{
		ID:      id,
		Name:    name,
		URL:     "https://example.com",
		Version: "1.0.0",
		Capabilities: []Capability{{
			Action: "PICK", Path: "/pick",
			Properties: CapabilityProps{MimeTypes: []string{"*/*"}},
		}},
	}
}

func TestStore_Upsert_Create(t *testing.T) {
	store, _ := newEmptyStore(t)

	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	got, ok := store.Get("app-1")
	require.True(t, ok)
	require.Equal(t, "App One", got.Name)
	require.Len(t, store.List(), 1)
}

func TestStore_Upsert_Replace(t *testing.T) {
	store, _ := newEmptyStore(t)

	require.NoError(t, store.Upsert(sampleManifest("app-1", "Original")))
	require.NoError(t, store.Upsert(sampleManifest("app-1", "Replaced")))

	got, ok := store.Get("app-1")
	require.True(t, ok)
	require.Equal(t, "Replaced", got.Name)
	require.Len(t, store.List(), 1, "replace, not insert")
}

func TestStore_Upsert_ValidationFails(t *testing.T) {
	store, _ := newEmptyStore(t)

	bad := sampleManifest("bad", "Bad")
	bad.ID = ""
	err := store.Upsert(bad)
	require.Error(t, err)
	require.Contains(t, err.Error(), "manifest.id is required")
	require.Empty(t, store.List(), "invalid manifest must not land in store")
}

func TestStore_Delete_Existing(t *testing.T) {
	store, _ := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	existed, err := store.Delete("app-1")
	require.NoError(t, err)
	require.True(t, existed)
	_, ok := store.Get("app-1")
	require.False(t, ok)
	require.Empty(t, store.List())
}

func TestStore_Delete_NonExistent_NoOp(t *testing.T) {
	store, _ := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	// Capture file mtime before.
	info1, err := os.Stat(store.path)
	require.NoError(t, err)

	// Delete non-existent id: must be no-op, no disk write.
	existed, err := store.Delete("does-not-exist")
	require.NoError(t, err)
	require.False(t, existed)

	// File mtime should be unchanged (no persist call).
	info2, err := os.Stat(store.path)
	require.NoError(t, err)
	require.Equal(t, info1.ModTime(), info2.ModTime(),
		"non-existent Delete must not trigger a disk write")

	// Seed manifest still present.
	_, ok := store.Get("app-1")
	require.True(t, ok)
}

func TestStore_Delete_Idempotent(t *testing.T) {
	store, _ := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	existed, err := store.Delete("app-1")
	require.NoError(t, err)
	require.True(t, existed)

	existed, err = store.Delete("app-1")
	require.NoError(t, err)
	require.False(t, existed)
}

func TestStore_Get_NotFound(t *testing.T) {
	store, _ := newEmptyStore(t)
	m, ok := store.Get("never")
	require.False(t, ok)
	require.Empty(t, m.ID)
}

func TestStore_List_SortedByID(t *testing.T) {
	store, _ := newEmptyStore(t)
	// Insert in reverse-alphabetical order.
	require.NoError(t, store.Upsert(sampleManifest("z-app", "Z")))
	require.NoError(t, store.Upsert(sampleManifest("m-app", "M")))
	require.NoError(t, store.Upsert(sampleManifest("a-app", "A")))

	list := store.List()
	require.Len(t, list, 3)
	require.Equal(t, "a-app", list[0].ID)
	require.Equal(t, "m-app", list[1].ID)
	require.Equal(t, "z-app", list[2].ID)
}

func TestStore_List_ReturnsCopy(t *testing.T) {
	store, _ := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "Original")))

	list := store.List()
	list[0].Name = "MUTATED BY CALLER"

	got, _ := store.Get("app-1")
	require.Equal(t, "Original", got.Name, "mutating returned slice must not affect store")
}

func TestStore_ConcurrentAccess(t *testing.T) {
	store, _ := newEmptyStore(t)

	// Seed 5 manifests.
	for i := 0; i < 5; i++ {
		require.NoError(t, store.Upsert(sampleManifest(fmt.Sprintf("app-%d", i), "App")))
	}

	var wg sync.WaitGroup

	// 10 writer goroutines × 10 upserts each.
	for w := 0; w < 10; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				m := sampleManifest(fmt.Sprintf("writer-%d-%d", w, i), "Writer")
				_ = store.Upsert(m) // ignore errors — goal is to exercise the mutex
			}
		}(w)
	}

	// 10 reader goroutines × 50 iterations of List and Get.
	for r := 0; r < 10; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = store.List()
				_, _ = store.Get("app-0")
			}
		}()
	}

	wg.Wait()

	require.GreaterOrEqual(t, len(store.List()), 105,
		"5 seed + 100 writer manifests at minimum")
}

func TestStore_Upsert_PersistFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	store, err := NewStore(path)
	require.NoError(t, err)

	// Seed so we can distinguish "rolled back to prior" from "wiped to empty".
	seed := sampleManifest("seed-app", "Seed")
	require.NoError(t, store.Upsert(seed))

	// Make the directory unwritable.
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	// New manifest: persist should fail, rollback, error "registry unchanged".
	newApp := sampleManifest("new-app", "New")
	err = store.Upsert(newApp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "registry unchanged")

	_, found := store.Get("new-app")
	require.False(t, found, "new-app must not be present after failed persist")

	got, found := store.Get("seed-app")
	require.True(t, found, "seed-app must still be present")
	require.Equal(t, "Seed", got.Name)

	// Update-failure path.
	modified := seed
	modified.Name = "Modified"
	err = store.Upsert(modified)
	require.Error(t, err)
	require.Contains(t, err.Error(), "registry unchanged")
	got, _ = store.Get("seed-app")
	require.Equal(t, "Seed", got.Name, "seed-app must have rolled back to original")
}
