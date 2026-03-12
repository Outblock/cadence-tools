package resolver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/onflow/cadence/common"
)

func TestFlowJSONResolver_ContractsSection(t *testing.T) {
	dir := t.TempDir()

	flowJSON := `{
		"contracts": {
			"FungibleToken": "./contracts/FungibleToken.cdc"
		}
	}`
	writeFile(t, filepath.Join(dir, "flow.json"), flowJSON)

	contractCode := `access(all) contract FungibleToken {}`
	writeFile(t, filepath.Join(dir, "contracts", "FungibleToken.cdc"), contractCode)

	r := NewFlowJSONResolver(dir)
	code, err := r.ResolveImport(context.Background(), common.StringLocation("FungibleToken"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != contractCode {
		t.Fatalf("got %q, want %q", code, contractCode)
	}
}

func TestFlowJSONResolver_ContractsObjectEntry(t *testing.T) {
	dir := t.TempDir()

	flowJSON := `{
		"contracts": {
			"NonFungibleToken": {
				"source": "./cadence/contracts/NonFungibleToken.cdc",
				"aliases": {"mainnet": "0x1d7e57aa55817448"}
			}
		}
	}`
	writeFile(t, filepath.Join(dir, "flow.json"), flowJSON)

	contractCode := `access(all) contract interface NonFungibleToken {}`
	writeFile(t, filepath.Join(dir, "cadence/contracts/NonFungibleToken.cdc"), contractCode)

	r := NewFlowJSONResolver(dir)
	code, err := r.ResolveImport(context.Background(), common.StringLocation("NonFungibleToken"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != contractCode {
		t.Fatalf("got %q, want %q", code, contractCode)
	}
}

func TestFlowJSONResolver_DependenciesSection(t *testing.T) {
	dir := t.TempDir()

	// Mimics what `flow dependencies install` creates
	flowJSON := `{
		"dependencies": {
			"FungibleToken": {
				"source": "mainnet://f233dcee88fe0abe.FungibleToken",
				"hash": "abc123",
				"aliases": {
					"mainnet": "f233dcee88fe0abe",
					"testnet": "9a0766d93b6608b7"
				}
			},
			"ViewResolver": {
				"source": "mainnet://1d7e57aa55817448.ViewResolver",
				"hash": "def456",
				"aliases": {
					"mainnet": "1d7e57aa55817448"
				}
			}
		},
		"networks": {
			"mainnet": "access.mainnet.nodes.onflow.org:9000"
		}
	}`
	writeFile(t, filepath.Join(dir, "flow.json"), flowJSON)

	// Files stored at imports/<address>/<name>.cdc
	ftCode := `import "ViewResolver"\naccess(all) contract FungibleToken {}`
	writeFile(t, filepath.Join(dir, "imports/f233dcee88fe0abe/FungibleToken.cdc"), ftCode)

	vrCode := `access(all) contract interface ViewResolver {}`
	writeFile(t, filepath.Join(dir, "imports/1d7e57aa55817448/ViewResolver.cdc"), vrCode)

	r := NewFlowJSONResolver(dir)

	// Resolve FungibleToken
	code, err := r.ResolveImport(context.Background(), common.StringLocation("FungibleToken"))
	if err != nil {
		t.Fatalf("FungibleToken: unexpected error: %v", err)
	}
	if code != ftCode {
		t.Fatalf("FungibleToken: got %q, want %q", code, ftCode)
	}

	// Resolve ViewResolver (transitive dependency)
	code, err = r.ResolveImport(context.Background(), common.StringLocation("ViewResolver"))
	if err != nil {
		t.Fatalf("ViewResolver: unexpected error: %v", err)
	}
	if code != vrCode {
		t.Fatalf("ViewResolver: got %q, want %q", code, vrCode)
	}
}

func TestFlowJSONResolver_NotFound(t *testing.T) {
	dir := t.TempDir()

	flowJSON := `{"contracts": {"Foo": "./foo.cdc"}}`
	writeFile(t, filepath.Join(dir, "flow.json"), flowJSON)

	r := NewFlowJSONResolver(dir)

	// Unknown contract name
	_, err := r.ResolveImport(context.Background(), common.StringLocation("Unknown"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}

	// File path imports should pass through
	_, err = r.ResolveImport(context.Background(), common.StringLocation("./foo.cdc"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for file path, got: %v", err)
	}

	// Non-string locations should pass through
	_, err = r.ResolveImport(context.Background(), common.AddressLocation{})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for AddressLocation, got: %v", err)
	}
}

func TestFlowJSONResolver_NoFlowJSON(t *testing.T) {
	dir := t.TempDir()

	r := NewFlowJSONResolver(dir)
	_, err := r.ResolveImport(context.Background(), common.StringLocation("Foo"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestFlowJSONResolver_AutoReload(t *testing.T) {
	dir := t.TempDir()

	// Start with empty
	writeFile(t, filepath.Join(dir, "flow.json"), `{"networks": {}}`)

	r := NewFlowJSONResolver(dir)
	_, err := r.ResolveImport(context.Background(), common.StringLocation("NewContract"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}

	// Simulate `flow dependencies install` updating flow.json
	writeFile(t, filepath.Join(dir, "flow.json"), `{
		"dependencies": {
			"NewContract": {
				"source": "mainnet://abc123.NewContract",
				"aliases": {"mainnet": "abc123"}
			}
		}
	}`)
	writeFile(t, filepath.Join(dir, "imports/abc123/NewContract.cdc"), `access(all) contract NewContract {}`)

	// Should auto-detect mtime change and reload
	code, err := r.ResolveImport(context.Background(), common.StringLocation("NewContract"))
	if err != nil {
		t.Fatalf("unexpected error after auto-reload: %v", err)
	}
	if code != `access(all) contract NewContract {}` {
		t.Fatalf("unexpected code: %q", code)
	}
}

func TestExtractAddressFromSource(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"mainnet://f233dcee88fe0abe.FungibleToken", "f233dcee88fe0abe"},
		{"testnet://9a0766d93b6608b7.FlowToken", "9a0766d93b6608b7"},
		{"invalid", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractAddressFromSource(tt.source)
		if got != tt.want {
			t.Errorf("extractAddressFromSource(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
