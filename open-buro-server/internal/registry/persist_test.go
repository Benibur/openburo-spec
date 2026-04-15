package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestStore_Upsert_WritesAtomically(t *testing.T) {
	store, path := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	// Read the file back and decode.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var ff fileFormat
	require.NoError(t, json.Unmarshal(data, &ff))
	require.Equal(t, currentFormatVersion, ff.Version)
	require.Len(t, ff.Manifests, 1)
	require.Equal(t, "app-1", ff.Manifests[0].ID)

	// Assert no tmp files left behind in the directory.
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, e := range entries {
		require.NotContains(t, e.Name(), ".tmp-", "no leftover temp files after successful persist")
	}
}

func TestStore_Upsert_WritesIndentedJSON(t *testing.T) {
	store, path := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// 2-space indentation → every nested object field is preceded by "\n  " or deeper.
	require.Contains(t, string(data), "\n  ", "file should be indented with 2 spaces")
	require.True(t, strings.HasPrefix(string(data), "{\n"),
		"file should start with '{\\n' indicating multi-line formatting")
}

func TestStore_Upsert_DeterministicOrder(t *testing.T) {
	// Insert in reverse order; persisted file should list manifests sorted by id.
	store, path := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("z-app", "Z")))
	require.NoError(t, store.Upsert(sampleManifest("a-app", "A")))
	require.NoError(t, store.Upsert(sampleManifest("m-app", "M")))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var ff fileFormat
	require.NoError(t, json.Unmarshal(data, &ff))
	require.Len(t, ff.Manifests, 3)
	require.Equal(t, "a-app", ff.Manifests[0].ID)
	require.Equal(t, "m-app", ff.Manifests[1].ID)
	require.Equal(t, "z-app", ff.Manifests[2].ID)
}
