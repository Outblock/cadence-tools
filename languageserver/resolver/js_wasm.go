//go:build wasm
// +build wasm

package resolver

import (
	"context"
	"fmt"
	"syscall/js"

	"github.com/onflow/cadence/common"
)

// JSResolver resolves imports by calling a JS function synchronously.
// The JS function signature is: (locationID: string) => string | undefined
// It returns the Cadence source code for the given location, or undefined if not found.
type JSResolver struct {
	funcName string
}

// NewJSResolver creates a resolver that calls the named global JS function.
func NewJSResolver(funcName string) *JSResolver {
	return &JSResolver{funcName: funcName}
}

func (r *JSResolver) ResolveImport(_ context.Context, location common.Location) (string, error) {
	fn := js.Global().Get(r.funcName)
	if fn.IsNull() || fn.IsUndefined() {
		return "", ErrNotFound
	}

	result := fn.Invoke(location.ID())
	if result.IsNull() || result.IsUndefined() {
		return "", ErrNotFound
	}

	code := result.String()
	if code == "" {
		return "", fmt.Errorf("empty code for %s", location.ID())
	}
	return code, nil
}
