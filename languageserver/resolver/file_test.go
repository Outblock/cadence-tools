package resolver_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/onflow/cadence/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/onflow/cadence-tools/languageserver/resolver"
)

func TestFileResolver_ResolvesExistingFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "hello.cdc")
	content := `access(all) contract Hello {}`
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))

	r := resolver.NewFileResolver()
	result, err := r.ResolveImport(context.Background(), common.StringLocation(filePath))
	require.NoError(t, err)
	assert.Equal(t, content, result)
}

func TestFileResolver_MissingFileReturnsErrNotFound(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "nonexistent.cdc")

	r := resolver.NewFileResolver()
	_, err := r.ResolveImport(context.Background(), common.StringLocation(filePath))
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolver.ErrNotFound))
}

func TestFileResolver_NonStringLocationReturnsErrNotFound(t *testing.T) {
	r := resolver.NewFileResolver()
	_, err := r.ResolveImport(context.Background(), common.IdentifierLocation("SomeContract"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolver.ErrNotFound))
}

func TestFileResolver_StringLocationWithoutFilePathReturnsErrNotFound(t *testing.T) {
	r := resolver.NewFileResolver()

	// No .cdc suffix, no / prefix — not a file path
	_, err := r.ResolveImport(context.Background(), common.StringLocation("SomeContract"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolver.ErrNotFound))
}
