package resolver

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/onflow/cadence/common"
)

// FileResolver resolves StringLocation imports pointing to local .cdc files.
type FileResolver struct{}

// NewFileResolver creates a new FileResolver.
func NewFileResolver() *FileResolver {
	return &FileResolver{}
}

// ResolveImport reads a local file. Returns ErrNotFound for:
//   - non-StringLocation types (AddressLocation, IdentifierLocation, etc.)
//   - StringLocations that don't look like file paths (no .cdc suffix and no / prefix)
//   - files that don't exist on disk (os.ErrNotExist -> ErrNotFound)
func (r *FileResolver) ResolveImport(_ context.Context, location common.Location) (string, error) {
	strLoc, ok := location.(common.StringLocation)
	if !ok {
		return "", ErrNotFound
	}

	path := string(strLoc)

	// Only treat it as a file path if it ends with .cdc or starts with /
	if !strings.HasSuffix(path, ".cdc") && !strings.HasPrefix(path, "/") {
		return "", ErrNotFound
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNotFound
		}
		return "", err
	}

	return string(data), nil
}
