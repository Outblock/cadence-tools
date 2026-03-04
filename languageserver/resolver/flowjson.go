package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/onflow/cadence/common"
)

// FlowJSONResolver resolves contract-name imports (e.g. import "FungibleToken")
// by reading the contracts section from a flow.json file. This mirrors how the
// Flow CLI's language server resolves string imports in a project workspace.
//
// It auto-reloads flow.json when the file's modification time changes, so
// contracts installed by `flow dependencies install` become available without
// restarting the LSP.
type FlowJSONResolver struct {
	rootDir    string
	configPath string

	mu        sync.Mutex
	contracts map[string]string // contract name → relative source path
	loadErr   error
	lastMod   time.Time
}

// NewFlowJSONResolver creates a resolver that reads flow.json from rootDir.
func NewFlowJSONResolver(rootDir string) *FlowJSONResolver {
	return &FlowJSONResolver{
		rootDir:    rootDir,
		configPath: filepath.Join(rootDir, "flow.json"),
	}
}

// flowJSON is the minimal structure we need from flow.json.
type flowJSON struct {
	Contracts map[string]json.RawMessage `json:"contracts"`
}

// contractEntry handles object entries with a "source" field.
type contractEntry struct {
	Source string `json:"source"`
}

// ensureLoaded reloads flow.json if it has been modified since last load.
func (r *FlowJSONResolver) ensureLoaded() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, err := os.Stat(r.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.contracts = nil
			r.loadErr = ErrNotFound
			r.lastMod = time.Time{}
			return ErrNotFound
		}
		return err
	}

	// Skip reload if mtime hasn't changed.
	if info.ModTime().Equal(r.lastMod) && r.contracts != nil {
		return r.loadErr
	}

	// Reload.
	r.lastMod = info.ModTime()
	r.loadErr = nil

	data, err := os.ReadFile(r.configPath)
	if err != nil {
		r.loadErr = err
		return err
	}

	var cfg flowJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		r.loadErr = err
		return err
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
	return nil
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

	if err := r.ensureLoaded(); err != nil {
		return "", err
	}

	r.mu.Lock()
	relPath, ok := r.contracts[name]
	r.mu.Unlock()

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
