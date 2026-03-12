package resolver

import (
	"context"
	"errors"

	"github.com/onflow/cadence/common"
)

// ErrNotFound is returned when no resolver can satisfy an import.
var ErrNotFound = errors.New("import not found")

// ImportResolver resolves a Cadence import location to its source code.
type ImportResolver interface {
	ResolveImport(ctx context.Context, location common.Location) (string, error)
}

// ResolverFunc adapts a plain function to the ImportResolver interface.
type ResolverFunc func(ctx context.Context, location common.Location) (string, error)

// ResolveImport calls the underlying function.
func (f ResolverFunc) ResolveImport(ctx context.Context, location common.Location) (string, error) {
	return f(ctx, location)
}

// CompositeResolver tries a series of resolvers in order and returns the
// first successful result. If a resolver returns ErrNotFound, the next
// resolver is tried. Any other error is propagated immediately.
type CompositeResolver struct {
	resolvers []ImportResolver
}

// NewCompositeResolver creates a CompositeResolver that delegates to the
// given resolvers in order.
func NewCompositeResolver(resolvers ...ImportResolver) *CompositeResolver {
	return &CompositeResolver{resolvers: resolvers}
}

// ResolveImport tries each resolver in order. It returns the first non-ErrNotFound
// result. If all resolvers return ErrNotFound (or there are no resolvers),
// it returns ErrNotFound.
func (c *CompositeResolver) ResolveImport(ctx context.Context, location common.Location) (string, error) {
	for _, r := range c.resolvers {
		result, err := r.ResolveImport(ctx, location)
		if err == nil {
			return result, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return "", err
		}
	}
	return "", ErrNotFound
}
