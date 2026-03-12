package server2

import (
	"context"

	"github.com/onflow/cadence/ast"
	"github.com/onflow/cadence/common"
	cerrors "github.com/onflow/cadence/errors"
	"github.com/onflow/cadence/parser"
	"github.com/onflow/cadence/sema"

	"github.com/onflow/cadence-tools/languageserver/resolver"
)

// FileKind classifies a Cadence source file.
type FileKind int

const (
	FileKindUnknown     FileKind = iota
	FileKindScript               // script (default for files without contract/transaction)
	FileKindContract             // contract or contract interface
	FileKindTransaction          // transaction
)

// DiagnosticSeverity mirrors LSP severity levels.
type DiagnosticSeverity int

const (
	SeverityError   DiagnosticSeverity = 1
	SeverityWarning DiagnosticSeverity = 2
	SeverityInfo    DiagnosticSeverity = 3
	SeverityHint    DiagnosticSeverity = 4
)

// Diagnostic represents a single error or warning in a document.
type Diagnostic struct {
	StartLine   int                // 1-based
	StartColumn int                // 0-based
	EndLine     int                // 1-based
	EndColumn   int                // 0-based
	Message     string
	Severity    DiagnosticSeverity
}

// AnalysisResult holds the output of analyzing a single document.
type AnalysisResult struct {
	URI         DocumentURI
	Checker     *sema.Checker
	Program     *ast.Program
	Diagnostics []Diagnostic
	FileKind    FileKind
	Cancelled   bool
}

// CanonicalCacheKey produces a stable cache key from a Location.
func CanonicalCacheKey(location common.Location) CacheKey {
	return location.ID()
}

// decideFileKind inspects the parsed program to determine its kind.
// Contract OR contract interface -> FileKindContract (fixes old LSP bug
// where contract interfaces were treated as scripts).
// Transaction -> FileKindTransaction. Otherwise -> FileKindScript.
func decideFileKind(program *ast.Program) FileKind {
	if program.SoleContractDeclaration() != nil ||
		program.SoleContractInterfaceDeclaration() != nil {
		return FileKindContract
	}
	if program.SoleTransactionDeclaration() != nil {
		return FileKindTransaction
	}
	return FileKindScript
}

// hasPosition is the interface for errors that carry position information.
type hasPosition interface {
	StartPosition() ast.Position
	EndPosition(common.MemoryGauge) ast.Position
}

// extractDiagnostics converts Cadence errors into a Diagnostic slice.
func extractDiagnostics(err error) []Diagnostic {
	if err == nil {
		return nil
	}

	var diags []Diagnostic

	// If the error is a parent error (parse errors, checker errors), extract children.
	if parentErr, ok := err.(cerrors.ParentError); ok {
		for _, childErr := range parentErr.ChildErrors() {
			if pos, ok := childErr.(hasPosition); ok {
				start := pos.StartPosition()
				end := pos.EndPosition(nil)
				diags = append(diags, Diagnostic{
					StartLine:   start.Line,
					StartColumn: start.Column,
					EndLine:     end.Line,
					EndColumn:   end.Column,
					Message:     childErr.Error(),
					Severity:    SeverityError,
				})
			} else {
				diags = append(diags, Diagnostic{
					StartLine: 1,
					EndLine:   1,
					Message:   childErr.Error(),
					Severity:  SeverityError,
				})
			}
		}
		return diags
	}

	// Single error with position.
	if pos, ok := err.(hasPosition); ok {
		start := pos.StartPosition()
		end := pos.EndPosition(nil)
		diags = append(diags, Diagnostic{
			StartLine:   start.Line,
			StartColumn: start.Column,
			EndLine:     end.Line,
			EndColumn:   end.Column,
			Message:     err.Error(),
			Severity:    SeverityError,
		})
		return diags
	}

	// Fallback: no position info.
	return []Diagnostic{{
		StartLine: 1,
		EndLine:   1,
		Message:   err.Error(),
		Severity:  SeverityError,
	}}
}

// singleLocationHandler resolves each identifier to its own location.
func singleLocationHandler(identifiers []ast.Identifier, location common.Location) ([]sema.ResolvedLocation, error) {
	if len(identifiers) == 0 {
		return []sema.ResolvedLocation{{Location: location}}, nil
	}
	result := make([]sema.ResolvedLocation, len(identifiers))
	for i, id := range identifiers {
		loc := common.AddressLocation{Name: id.Identifier}
		// Preserve the address from the original location so that
		// address-based imports (e.g. import X from 0xADDR) keep
		// their address for the import resolver.
		if addrLoc, ok := location.(common.AddressLocation); ok {
			loc.Address = addrLoc.Address
		}
		result[i] = sema.ResolvedLocation{
			Location:    loc,
			Identifiers: []ast.Identifier{id},
		}
	}
	return result, nil
}

// Analyze parses and type-checks a document. It respects ctx cancellation.
func Analyze(
	ctx context.Context,
	snap *Snapshot,
	uri DocumentURI,
	importResolver resolver.ImportResolver,
) *AnalysisResult {
	result := &AnalysisResult{URI: uri}

	// 1. Check cancellation before starting.
	if ctx.Err() != nil {
		result.Cancelled = true
		return result
	}

	// 2. Get document from snapshot.
	doc, ok := snap.Documents[uri]
	if !ok {
		result.Diagnostics = []Diagnostic{{
			StartLine: 1,
			EndLine:   1,
			Message:   "document not found",
			Severity:  SeverityError,
		}}
		return result
	}

	// 3. Parse.
	program, parseErr := parser.ParseProgram(nil, []byte(doc.Text), parser.Config{})
	if parseErr != nil {
		result.Diagnostics = append(result.Diagnostics, extractDiagnostics(parseErr)...)
	}

	// 4. If program is nil, return early with parse diagnostics.
	if program == nil {
		return result
	}
	result.Program = program

	// 5. Detect file kind.
	result.FileKind = decideFileKind(program)

	// 6. Check cancellation after parse.
	if ctx.Err() != nil {
		result.Cancelled = true
		return result
	}

	// 7. Build location.
	location := common.StringLocation(uri)

	// 8. Build base value activation with stdlib.
	baseActivation := newBaseValueActivation()

	// 9. Build sema.Config.
	config := &sema.Config{
		AccessCheckMode:            sema.AccessCheckModeStrict,
		PositionInfoEnabled:        true,
		ExtendedElaborationEnabled: true,
		SuggestionsEnabled:         true,
		BaseValueActivationHandler: func(_ common.Location) *sema.VariableActivation {
			return baseActivation
		},
		LocationHandler: singleLocationHandler,
		ImportHandler: func(checker *sema.Checker, importedLocation common.Location, importRange ast.Range) (sema.Import, error) {
			// Check cache first.
			cacheKey := CanonicalCacheKey(importedLocation)
			if entry, found := snap.Cache.Get(cacheKey); found && entry.Valid && entry.Checker != nil {
				snap.DepGraph.AddEdge(CanonicalCacheKey(location), cacheKey)
				return sema.ElaborationImport{
					Elaboration: entry.Checker.Elaboration,
				}, nil
			}

			// Resolve via importResolver.
			if importResolver == nil {
				return nil, &sema.CheckerError{}
			}

			code, err := importResolver.ResolveImport(ctx, importedLocation)
			if err != nil {
				return nil, err
			}

			// Parse the imported code.
			importedProgram, parseErr := parser.ParseProgram(nil, []byte(code), parser.Config{})
			if parseErr != nil || importedProgram == nil {
				return nil, parseErr
			}

			// Create sub-checker from parent.
			subChecker, err := checker.SubChecker(importedProgram, importedLocation)
			if err != nil {
				return nil, err
			}

			// Check the imported program (errors are intentionally ignored;
			// we still cache the elaboration for downstream use).
			_ = subChecker.Check()

			// Cache result.
			snap.Cache.Put(cacheKey, &CheckerEntry{
				Checker: subChecker,
				Valid:   true,
			})

			// Record dependency.
			snap.DepGraph.AddEdge(CanonicalCacheKey(location), cacheKey)

			return sema.ElaborationImport{
				Elaboration: subChecker.Elaboration,
			}, nil
		},
	}

	// 10. Create checker.
	checker, err := sema.NewChecker(program, location, nil, config)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, extractDiagnostics(err)...)
		return result
	}

	// 11. Run type checking.
	checkErr := checker.Check()
	if checkErr != nil {
		result.Diagnostics = append(result.Diagnostics, extractDiagnostics(checkErr)...)
	}

	// 12. Check cancellation after check — discard results if cancelled.
	if ctx.Err() != nil {
		result.Cancelled = true
		return result
	}

	// 13. Cache result.
	snap.Cache.Put(CanonicalCacheKey(location), &CheckerEntry{
		Checker: checker,
		Valid:   true,
	})

	result.Checker = checker
	return result
}
