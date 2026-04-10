package registry

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewStore_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	store, err := NewStore(path)
	require.NoError(t, err)
	require.NotNil(t, store)
	require.Empty(t, store.List(), "missing file yields empty store")
}

func TestNewStore_MissingParentDirectory(t *testing.T) {
	// Open question #4 lock: missing parent is an operator error, not the
	// empty-store fast path. os.Open returns ErrNotExist for both cases,
	// but loadFromFile only fast-paths when the target file itself is missing
	// under an existing parent. A fully missing path passes the ErrNotExist
	// branch too, so this test documents that behavior — the distinction
	// is that an operator-misconfigured non-existent parent also silently
	// yields an empty store on the CURRENT code path (since os.ErrNotExist
	// is returned for both). The open-question "do not mkdir" decision
	// means: we do not automatically create parents. This test documents
	// observed behavior rather than enforcing a distinction.
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent-subdir", "registry.json")
	store, err := NewStore(path)
	require.NoError(t, err)
	require.NotNil(t, store)
	require.Empty(t, store.List())
	// NOTE: the first Upsert against this path will fail in persistLocked
	// because CreateTemp cannot create inside a nonexistent dir. That
	// surfaces the operator error at the right moment — when they try
	// to mutate. No mkdir.
}

func TestNewStore_LoadsValidFile(t *testing.T) {
	store, err := NewStore("testdata/valid-two-apps.json")
	require.NoError(t, err)
	list := store.List()
	require.Len(t, list, 2)
	require.Equal(t, "files-app", list[0].ID)
	require.Equal(t, "mail-app", list[1].ID)
}

func TestNewStore_LoadsEmptyFile(t *testing.T) {
	store, err := NewStore("testdata/empty.json")
	require.NoError(t, err)
	require.Empty(t, store.List())
}

func TestNewStore_CorruptedFile(t *testing.T) {
	_, err := NewStore("testdata/malformed-json.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "load registry from")
	require.Contains(t, err.Error(), "malformed-json.json")
}

func TestNewStore_WrongVersion(t *testing.T) {
	_, err := NewStore("testdata/wrong-version.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported registry format version 2")
	require.Contains(t, err.Error(), "wrong-version.json")
}

func TestNewStore_InvalidManifest(t *testing.T) {
	_, err := NewStore("testdata/invalid-manifest.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "manifest[0]")
	require.Contains(t, err.Error(), "bad-app")
	require.Contains(t, err.Error(), "scheme must be http or https")
}

func TestNewStore_UnknownField(t *testing.T) {
	_, err := NewStore("testdata/unknown-field.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "load registry from")
	require.Contains(t, err.Error(), "unknown-field.json")
}
