package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/onflow/cadence/common"
)

// FlowJSONResolver resolves contract-name imports (e.g. import "FungibleToken")
// by reading flow.json. It supports two layouts:
//
//  1. "contracts" section (manual projects): {"contracts": {"Foo": "./path/to/Foo.cdc"}}
//  2. "dependencies" section (flow dependencies install): stored at imports/<address>/<name>.cdc
//
// It auto-reloads flow.json when the file's modification time changes, so
// contracts installed by `flow dependencies install` become available without
// restarting the LSP.
type FlowJSONResolver struct {
	rootDir    string
	configPath string

	mu        sync.Mutex
	contracts map[string]string // contract name → resolved absolute path
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
	Contracts    map[string]json.RawMessage `json:"contracts"`
	Dependencies map[string]depEntry        `json:"dependencies"`
}

// depEntry represents a dependency installed by `flow dependencies install`.
type depEntry struct {
	Source  string            `json:"source"`  // e.g. "mainnet://f233dcee88fe0abe.FungibleToken"
	Aliases map[string]string `json:"aliases"` // network → address
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

	r.contracts = make(map[string]string)

	// 1. Load from "contracts" section (manual project layout).
	for name, raw := range cfg.Contracts {
		// Try as simple string: "ContractName": "./path/to/file.cdc"
		var simplePath string
		if err := json.Unmarshal(raw, &simplePath); err == nil {
			r.contracts[name] = filepath.Join(r.rootDir, simplePath)
			continue
		}
		// Try as object: "ContractName": {"source": "./path/to/file.cdc", ...}
		var entry contractEntry
		if err := json.Unmarshal(raw, &entry); err == nil && entry.Source != "" {
			r.contracts[name] = filepath.Join(r.rootDir, entry.Source)
		}
	}

	// 2. Load from "dependencies" section (flow dependencies install layout).
	// Files are stored at: imports/<address>/<ContractName>.cdc
	// The source field format is: "mainnet://<address>.<ContractName>"
	for name, dep := range cfg.Dependencies {
		if _, exists := r.contracts[name]; exists {
			continue // contracts section takes precedence
		}
		addr := extractAddressFromSource(dep.Source)
		if addr == "" {
			// Try aliases as fallback
			for _, a := range dep.Aliases {
				if a != "" {
					addr = a
					break
				}
			}
		}
		if addr != "" {
			path := filepath.Join(r.rootDir, "imports", addr, fmt.Sprintf("%s.cdc", name))
			r.contracts[name] = path
		}
	}

	return nil
}

// extractAddressFromSource parses "mainnet://f233dcee88fe0abe.FungibleToken" → "f233dcee88fe0abe"
func extractAddressFromSource(source string) string {
	// Format: "network://address.ContractName"
	idx := strings.Index(source, "://")
	if idx < 0 {
		return ""
	}
	rest := source[idx+3:]
	dotIdx := strings.Index(rest, ".")
	if dotIdx < 0 {
		return ""
	}
	return rest[:dotIdx]
}

// ResolveImport resolves a StringLocation that looks like a contract name
// (no .cdc suffix, no / prefix) by looking it up in flow.json.
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
	absPath, ok := r.contracts[name]
	r.mu.Unlock()

	if !ok {
		return "", ErrNotFound
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNotFound
		}
		return "", err
	}

	return string(data), nil
}
