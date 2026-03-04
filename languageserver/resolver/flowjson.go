package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/onflow/cadence/common"
)

// FlowJSONResolver resolves contract-name imports (e.g. import "FungibleToken")
// by reading the contracts section from a flow.json file. This mirrors how the
// Flow CLI's language server resolves string imports in a project workspace.
type FlowJSONResolver struct {
	rootDir string

	once      sync.Once
	contracts map[string]string // contract name → relative source path
	loadErr   error
}

// NewFlowJSONResolver creates a resolver that reads flow.json from rootDir.
// The flow.json is lazily loaded on first ResolveImport call.
func NewFlowJSONResolver(rootDir string) *FlowJSONResolver {
	return &FlowJSONResolver{rootDir: rootDir}
}

// flowJSON is the minimal structure we need from flow.json.
type flowJSON struct {
	Contracts map[string]json.RawMessage `json:"contracts"`
}

// contractEntry handles both simple string paths and object entries.
type contractEntry struct {
	Source string `json:"source"`
}

func (r *FlowJSONResolver) load() {
	configPath := filepath.Join(r.rootDir, "flow.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.loadErr = ErrNotFound
		} else {
			r.loadErr = err
		}
		return
	}

	var cfg flowJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		r.loadErr = err
		return
	}

	r.contracts = make(map[string]string, len(cfg.Contracts))
	for name, raw := range cfg.Contracts {
		// Try as simple string first: "ContractName": "./path/to/file.cdc"
		var simplePath string
		if err := json.Unmarshal(raw, &simplePath); err == nil {
			r.contracts[name] = simplePath
			continue
		}
		// Try as object: "ContractName": {"source": "./path/to/file.cdc", ...}
		var entry contractEntry
		if err := json.Unmarshal(raw, &entry); err == nil && entry.Source != "" {
			r.contracts[name] = entry.Source
		}
	}
}

// ResolveImport resolves a StringLocation that looks like a contract name
// (no .cdc suffix, no / prefix) by looking it up in flow.json's contracts.
func (r *FlowJSONResolver) ResolveImport(_ context.Context, location common.Location) (string, error) {
	strLoc, ok := location.(common.StringLocation)
	if !ok {
		return "", ErrNotFound
	}

	name := string(strLoc)

	// Only handle contract-name imports (not file paths)
	if strings.HasSuffix(name, ".cdc") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, ".") {
		return "", ErrNotFound
	}

	r.once.Do(r.load)
	if r.loadErr != nil {
		return "", r.loadErr
	}

	relPath, ok := r.contracts[name]
	if !ok {
		return "", ErrNotFound
	}

	absPath := filepath.Join(r.rootDir, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNotFound
		}
		return "", err
	}

	return string(data), nil
}

// Reload forces re-reading flow.json on the next ResolveImport call.
// Useful when flow.json is updated (e.g. after flow dependencies install).
func (r *FlowJSONResolver) Reload() {
	r.once = sync.Once{}
	r.contracts = nil
	r.loadErr = nil
}
