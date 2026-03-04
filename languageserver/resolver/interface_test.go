package resolver

import (
	"context"
	"errors"
	"testing"

	"github.com/onflow/cadence/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolverFunc(t *testing.T) {
	expected := "access(all) contract Foo {}"
	fn := ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		return expected, nil
	})

	result, err := fn.ResolveImport(context.Background(), common.StringLocation("Foo"))
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestResolverFunc_Error(t *testing.T) {
	fn := ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		return "", ErrNotFound
	})

	_, err := fn.ResolveImport(context.Background(), common.StringLocation("Missing"))
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCompositeResolver_FirstSucceeds(t *testing.T) {
	r1 := ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		return "from-first", nil
	})
	r2 := ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		t.Fatal("second resolver should not be called")
		return "", nil
	})

	composite := NewCompositeResolver(r1, r2)
	result, err := composite.ResolveImport(context.Background(), common.StringLocation("Foo"))
	require.NoError(t, err)
	assert.Equal(t, "from-first", result)
}

func TestCompositeResolver_FirstFailsSecondSucceeds(t *testing.T) {
	r1 := ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		return "", ErrNotFound
	})
	r2 := ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		return "from-second", nil
	})

	composite := NewCompositeResolver(r1, r2)
	result, err := composite.ResolveImport(context.Background(), common.StringLocation("Foo"))
	require.NoError(t, err)
	assert.Equal(t, "from-second", result)
}

func TestCompositeResolver_AllFail(t *testing.T) {
	r1 := ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		return "", ErrNotFound
	})
	r2 := ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		return "", ErrNotFound
	})

	composite := NewCompositeResolver(r1, r2)
	_, err := composite.ResolveImport(context.Background(), common.StringLocation("Missing"))
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCompositeResolver_NonNotFoundErrorPropagates(t *testing.T) {
	networkErr := errors.New("network timeout")
	r1 := ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		return "", networkErr
	})
	r2 := ResolverFunc(func(ctx context.Context, location common.Location) (string, error) {
		t.Fatal("second resolver should not be called after non-ErrNotFound error")
		return "", nil
	})

	composite := NewCompositeResolver(r1, r2)
	_, err := composite.ResolveImport(context.Background(), common.StringLocation("Foo"))
	assert.ErrorIs(t, err, networkErr)
	assert.NotErrorIs(t, err, ErrNotFound)
}

func TestCompositeResolver_Empty(t *testing.T) {
	composite := NewCompositeResolver()
	_, err := composite.ResolveImport(context.Background(), common.StringLocation("Foo"))
	assert.ErrorIs(t, err, ErrNotFound)
}
