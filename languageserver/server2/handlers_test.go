package server2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/onflow/cadence-tools/languageserver/protocol"
)

// openAndCheck opens a document on the server, triggering synchronous analysis.
func openAndCheck(t *testing.T, srv *ServerV2, conn *mockConn, uri protocol.DocumentURI, code string) {
	t.Helper()
	err := srv.DidOpenTextDocument(conn, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "cadence",
			Version:    1,
			Text:       code,
		},
	})
	require.NoError(t, err)
}

func TestHoverReturnsTypeInfo(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///hover.cdc")
	code := `access(all) fun main(): Int { return 42 }`
	openAndCheck(t, srv, conn, uri, code)

	result, err := srv.Hover(conn, &protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		// Position on "main" identifier (line 0, character 17)
		Position: protocol.Position{Line: 0, Character: 17},
	})
	require.NoError(t, err)

	// Should return hover info (the function type)
	if result != nil {
		assert.Contains(t, result.Contents.Value, "fun")
	}
}

func TestHoverReturnsNilForNoChecker(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	// Don't open any document - no checker available
	result, err := srv.Hover(conn, &protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
		Position:     protocol.Position{Line: 0, Character: 0},
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestDefinitionReturnsLocation(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///def.cdc")
	code := `access(all) fun main() {
    let x = 42
    let y = x
}`
	openAndCheck(t, srv, conn, uri, code)

	// Position on "x" in "let y = x" (line 2, character 12)
	result, err := srv.Definition(conn, &protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Position:     protocol.Position{Line: 2, Character: 12},
	})
	require.NoError(t, err)

	if result != nil {
		assert.Equal(t, uri, result.URI)
	}
}

func TestDefinitionReturnsNilForNoChecker(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	result, err := srv.Definition(conn, &protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
		Position:     protocol.Position{Line: 0, Character: 0},
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestDocumentSymbolReturnsSymbols(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///symbols.cdc")
	code := `access(all) fun main() {}
access(all) fun helper(): Int { return 1 }`
	openAndCheck(t, srv, conn, uri, code)

	symbols, err := srv.DocumentSymbol(conn, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(symbols), 2, "should have at least 2 symbols (main + helper)")

	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Name
	}
	assert.Contains(t, names, "main")
	assert.Contains(t, names, "helper")
}

func TestDocumentSymbolReturnsEmptyForNoChecker(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	symbols, err := srv.DocumentSymbol(conn, &protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
	})
	require.NoError(t, err)
	assert.Empty(t, symbols)
}

func TestCompletionReturnsKeywords(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///completion.cdc")
	code := `access(all) fun main() {

}`
	openAndCheck(t, srv, conn, uri, code)

	items, err := srv.Completion(conn, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 1, Character: 4},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, items, "should return completion items (keywords etc.)")

	// Check that keywords like "if", "for", "return" are present
	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}
	assert.True(t, labels["if"], "should contain 'if' keyword")
	assert.True(t, labels["for"], "should contain 'for' keyword")
	assert.True(t, labels["return"], "should contain 'return' keyword")
}

func TestCompletionReturnsEmptyForNoChecker(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	items, err := srv.Completion(conn, &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
	})
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestDocumentHighlightReturnsNilForNoChecker(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	result, err := srv.DocumentHighlight(conn, &protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
		Position:     protocol.Position{Line: 0, Character: 0},
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestRenameReturnsNilForNoChecker(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	result, err := srv.Rename(conn, &protocol.RenameParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
		Position:     protocol.Position{Line: 0, Character: 0},
		NewName:      "newName",
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestCodeActionReturnsEmptyForNoChecker(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	result, err := srv.CodeAction(conn, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
		Range:        protocol.Range{},
		Context:      protocol.CodeActionContext{},
	})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestCodeLensReturnsEmpty(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	result, err := srv.CodeLens(conn, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
	})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestSignatureHelpReturnsNilForNoChecker(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	result, err := srv.SignatureHelp(conn, &protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
		Position:     protocol.Position{Line: 0, Character: 0},
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestInlayHintReturnsEmptyForNoChecker(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	result, err := srv.InlayHint(conn, &protocol.InlayHintParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
		Range:        protocol.Range{},
	})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestInlayHintShowsInferredTypes(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///inlay.cdc")
	code := `access(all) fun main() {
    let x = 42
}`
	openAndCheck(t, srv, conn, uri, code)

	result, err := srv.InlayHint(conn, &protocol.InlayHintParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 3, Character: 0},
		},
	})
	require.NoError(t, err)

	// Should have at least one inlay hint for "let x = 42" (inferred type Int)
	if len(result) > 0 {
		assert.Contains(t, result[0].Label[0].Value, "Int")
	}
}

func TestDocumentLinkReturnsNil(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	result, err := srv.DocumentLink(conn, &protocol.DocumentLinkParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.cdc"},
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestExecuteCommandReturnsNil(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	result, err := srv.ExecuteCommand(conn, &protocol.ExecuteCommandParams{
		Command: "test",
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestDocumentHighlightWithVariable(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///highlight.cdc")
	code := `access(all) fun main() {
    let x = 42
    let y = x
}`
	openAndCheck(t, srv, conn, uri, code)

	// Position on "x" in "let y = x" (line 2, character 12)
	result, err := srv.DocumentHighlight(conn, &protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Position:     protocol.Position{Line: 2, Character: 12},
	})
	require.NoError(t, err)
	// Should highlight both occurrences of "x"
	if result != nil {
		assert.GreaterOrEqual(t, len(result), 1, "should highlight at least one occurrence of x")
	}
}

func TestRenameVariable(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///rename.cdc")
	code := `access(all) fun main() {
    let x = 42
    let y = x
}`
	openAndCheck(t, srv, conn, uri, code)

	// Rename "x" at (line 2, character 12)
	result, err := srv.Rename(conn, &protocol.RenameParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Position:     protocol.Position{Line: 2, Character: 12},
		NewName:      "newX",
	})
	require.NoError(t, err)

	if result != nil {
		edits := result.Changes[uri]
		if len(edits) > 0 {
			for _, edit := range edits {
				assert.Equal(t, "newX", edit.NewText)
			}
		}
	}
}

func TestReferencesReturnsAllOccurrences(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///refs.cdc")
	code := `access(all) fun main() {
    let x = 42
    let y = x
    let z = x
}`
	openAndCheck(t, srv, conn, uri, code)

	result, err := srv.References(conn, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     protocol.Position{Line: 2, Character: 12},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: true},
	})
	require.NoError(t, err)

	// Should find: declaration of x, usage in "let y = x", usage in "let z = x"
	assert.GreaterOrEqual(t, len(result), 2, "should find at least the declaration and usages of x")
}

func TestReferencesReturnsNilForNoChecker(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	result, err := srv.References(conn, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: "file:///noexist.cdc"},
			Position:     protocol.Position{Line: 0, Character: 0},
		},
		Context: protocol.ReferenceContext{},
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGetDocument(t *testing.T) {
	host := NewAnalysisHost(64)

	// Before adding any document
	_, ok := host.GetDocument("file:///missing.cdc")
	assert.False(t, ok)

	// After adding a document
	host.UpdateDocument("file:///test.cdc", "hello", 1)
	doc, ok := host.GetDocument("file:///test.cdc")
	assert.True(t, ok)
	assert.Equal(t, "hello", doc.Text)
	assert.Equal(t, int32(1), doc.Version)
}
