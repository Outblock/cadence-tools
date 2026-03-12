package server2

import (
	"context"
	"testing"

	"github.com/onflow/cadence/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/onflow/cadence-tools/languageserver/resolver"
)

// newTestSnapshot creates a Snapshot with a single document and default cache/depgraph.
func newTestSnapshot(uri DocumentURI, text string) *Snapshot {
	return &Snapshot{
		Revision: 1,
		Documents: map[DocumentURI]Document{
			uri: {Text: text, Version: 1},
		},
		Cache:    NewLRUCheckerCache(64),
		DepGraph: NewDependencyGraph(),
	}
}

func TestAnalyze_ValidScript(t *testing.T) {
	uri := "file:///test/script.cdc"
	code := `access(all) fun main(): Int { return 42 }`
	snap := newTestSnapshot(uri, code)

	result := Analyze(context.Background(), snap, uri, nil)

	assert.False(t, result.Cancelled)
	assert.NotNil(t, result.Program)
	assert.NotNil(t, result.Checker)
	assert.Empty(t, result.Diagnostics)
	assert.Equal(t, FileKindScript, result.FileKind)
	assert.Equal(t, uri, result.URI)
}

func TestAnalyze_ParseError(t *testing.T) {
	uri := "file:///test/bad.cdc"
	code := `access(all) fun main( {`
	snap := newTestSnapshot(uri, code)

	result := Analyze(context.Background(), snap, uri, nil)

	assert.False(t, result.Cancelled)
	require.NotEmpty(t, result.Diagnostics)
	// Parse error diagnostics should have a message.
	for _, d := range result.Diagnostics {
		assert.NotEmpty(t, d.Message)
		assert.Equal(t, SeverityError, d.Severity)
	}
}

func TestAnalyze_SemanticError(t *testing.T) {
	uri := "file:///test/type_error.cdc"
	code := `access(all) fun main(): Int { return "hello" }`
	snap := newTestSnapshot(uri, code)

	result := Analyze(context.Background(), snap, uri, nil)

	assert.False(t, result.Cancelled)
	require.NotEmpty(t, result.Diagnostics)
	// Should report a type mismatch error.
	found := false
	for _, d := range result.Diagnostics {
		if d.Severity == SeverityError && d.Message != "" {
			found = true
		}
	}
	assert.True(t, found, "expected at least one semantic error diagnostic")
}

func TestAnalyze_CancelledContext(t *testing.T) {
	uri := "file:///test/script.cdc"
	code := `access(all) fun main(): Int { return 42 }`
	snap := newTestSnapshot(uri, code)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result := Analyze(ctx, snap, uri, nil)

	assert.True(t, result.Cancelled)
}

func TestAnalyze_CachesCheckerAfterSuccess(t *testing.T) {
	uri := "file:///test/script.cdc"
	code := `access(all) fun main(): Int { return 42 }`
	snap := newTestSnapshot(uri, code)

	result := Analyze(context.Background(), snap, uri, nil)
	require.NotNil(t, result.Checker)

	// Verify it was cached.
	cacheKey := CanonicalCacheKey(common.StringLocation(uri))
	entry, found := snap.Cache.Get(cacheKey)
	assert.True(t, found)
	assert.True(t, entry.Valid)
	assert.Equal(t, result.Checker, entry.Checker)
}

func TestAnalyze_FileKindContract(t *testing.T) {
	uri := "file:///test/contract.cdc"
	code := `access(all) contract Foo {}`
	snap := newTestSnapshot(uri, code)

	result := Analyze(context.Background(), snap, uri, nil)

	assert.Equal(t, FileKindContract, result.FileKind)
	assert.NotNil(t, result.Program)
}

func TestAnalyze_FileKindTransaction(t *testing.T) {
	uri := "file:///test/tx.cdc"
	code := `transaction { prepare(acct: &Account) {} execute {} }`
	snap := newTestSnapshot(uri, code)

	result := Analyze(context.Background(), snap, uri, nil)

	assert.Equal(t, FileKindTransaction, result.FileKind)
	assert.NotNil(t, result.Program)
}

func TestAnalyze_FileKindContractInterface(t *testing.T) {
	uri := "file:///test/iface.cdc"
	code := `access(all) contract interface IFoo {}`
	snap := newTestSnapshot(uri, code)

	result := Analyze(context.Background(), snap, uri, nil)

	// Contract interfaces should be classified as contracts, not scripts.
	// This is the bug fix: old LSP treated contract interfaces as scripts.
	assert.Equal(t, FileKindContract, result.FileKind)
	assert.NotNil(t, result.Program)
}

func TestAnalyze_DocumentNotFound(t *testing.T) {
	snap := newTestSnapshot("file:///other.cdc", "")

	result := Analyze(context.Background(), snap, "file:///missing.cdc", nil)

	require.NotEmpty(t, result.Diagnostics)
	assert.Contains(t, result.Diagnostics[0].Message, "document not found")
}

func TestDecideFileKind(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected FileKind
	}{
		{"script", `access(all) fun main() {}`, FileKindScript},
		{"contract", `access(all) contract Foo {}`, FileKindContract},
		{"contract interface", `access(all) contract interface IFoo {}`, FileKindContract},
		{"transaction", `transaction { execute {} }`, FileKindTransaction},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri := "file:///test.cdc"
			snap := newTestSnapshot(uri, tt.code)
			result := Analyze(context.Background(), snap, uri, nil)
			assert.Equal(t, tt.expected, result.FileKind)
		})
	}
}

func TestCanonicalCacheKey(t *testing.T) {
	loc := common.StringLocation("file:///test.cdc")
	key := CanonicalCacheKey(loc)
	assert.Equal(t, loc.ID(), key)
}

func TestAnalyze_WithImportResolver(t *testing.T) {
	mainURI := "file:///test/main.cdc"
	mainCode := `
		import Foo from 0x1
		access(all) fun main() {}
	`
	snap := newTestSnapshot(mainURI, mainCode)

	importedCode := `access(all) contract Foo {}`

	res := resolver.ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		return importedCode, nil
	})

	result := Analyze(context.Background(), snap, mainURI, res)

	// The analysis should complete. There may be some diagnostics depending on
	// how the import resolution works, but it should not panic.
	assert.False(t, result.Cancelled)
	assert.NotNil(t, result.Program)
}
