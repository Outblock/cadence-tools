package server2

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/onflow/cadence-tools/languageserver/protocol"
)

// mockConn implements protocol.Conn and records calls to PublishDiagnostics.
type mockConn struct {
	mu          sync.Mutex
	diagnostics []*protocol.PublishDiagnosticsParams
}

func (m *mockConn) Notify(method string, params any) error         { return nil }
func (m *mockConn) ShowMessage(params *protocol.ShowMessageParams) {}
func (m *mockConn) ShowMessageRequest(params *protocol.ShowMessageRequestParams) (*protocol.MessageActionItem, error) {
	return nil, nil
}
func (m *mockConn) LogMessage(params *protocol.LogMessageParams)                 {}
func (m *mockConn) RegisterCapability(params *protocol.RegistrationParams) error { return nil }

func (m *mockConn) PublishDiagnostics(params *protocol.PublishDiagnosticsParams) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.diagnostics = append(m.diagnostics, params)
	return nil
}

func (m *mockConn) getDiagnostics() []*protocol.PublishDiagnosticsParams {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*protocol.PublishDiagnosticsParams, len(m.diagnostics))
	copy(cp, m.diagnostics)
	return cp
}

func newTestServer() *ServerV2 {
	return NewServerV2(ServerConfig{
		CacheCapacity: 64,
		DebounceDelay: 0, // synchronous for testing
	})
}

func TestInitializeReturnsCapabilities(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}

	result, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)
	require.NotNil(t, result)

	caps := result.Capabilities

	// Text document sync should be configured for full sync with open/close.
	syncOpts, ok := caps.TextDocumentSync.(*protocol.TextDocumentSyncOptions)
	require.True(t, ok, "TextDocumentSync should be *TextDocumentSyncOptions")
	assert.True(t, syncOpts.OpenClose)
	assert.Equal(t, protocol.Full, syncOpts.Change)

	// All providers should be enabled.
	assert.NotNil(t, caps.HoverProvider)
	assert.NotNil(t, caps.DefinitionProvider)
	assert.NotNil(t, caps.SignatureHelpProvider)
	assert.NotNil(t, caps.DocumentHighlightProvider)
	assert.NotNil(t, caps.CodeLensProvider)
	assert.NotNil(t, caps.CompletionProvider)
	assert.NotNil(t, caps.DocumentSymbolProvider)
	assert.NotNil(t, caps.DocumentLinkProvider)
	assert.NotNil(t, caps.ExecuteCommandProvider)
	assert.NotNil(t, caps.RenameProvider)
	assert.NotNil(t, caps.CodeActionProvider)
	assert.NotNil(t, caps.InlayHintProvider)

	assert.Equal(t, []string{"."}, caps.CompletionProvider.TriggerCharacters)
	assert.True(t, caps.CompletionProvider.ResolveProvider)
	assert.Equal(t, []string{"("}, caps.SignatureHelpProvider.TriggerCharacters)
}

func TestDidOpenTriggersDiagnostics(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}

	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///test.cdc")
	err = srv.DidOpenTextDocument(conn, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "cadence",
			Version:    1,
			Text:       "access(all) fun main() {}",
		},
	})
	require.NoError(t, err)

	diags := conn.getDiagnostics()
	require.Len(t, diags, 1, "should publish diagnostics once")
	assert.Equal(t, uri, diags[0].URI)
}

func TestDidOpenValidCodeEmptyDiagnostics(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}

	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///valid.cdc")
	err = srv.DidOpenTextDocument(conn, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "cadence",
			Version:    1,
			Text:       "access(all) fun main() {}",
		},
	})
	require.NoError(t, err)

	diags := conn.getDiagnostics()
	require.Len(t, diags, 1)
	assert.Empty(t, diags[0].Diagnostics, "valid code should produce no diagnostics")
}

func TestDidOpenInvalidCodeProducesDiagnostics(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}

	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///invalid.cdc")
	err = srv.DidOpenTextDocument(conn, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "cadence",
			Version:    1,
			Text:       "this is not valid cadence code {{{{",
		},
	})
	require.NoError(t, err)

	diags := conn.getDiagnostics()
	require.Len(t, diags, 1)
	assert.NotEmpty(t, diags[0].Diagnostics, "invalid code should produce diagnostics")

	// Check that severity is error.
	for _, d := range diags[0].Diagnostics {
		assert.Equal(t, protocol.SeverityError, d.Severity)
		assert.Equal(t, "cadence", d.Source)
	}
}

func TestDidChangeUpdatesAndPublishes(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}

	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///change.cdc")

	// Open with valid code.
	err = srv.DidOpenTextDocument(conn, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "cadence",
			Version:    1,
			Text:       "access(all) fun main() {}",
		},
	})
	require.NoError(t, err)

	// Change to invalid code.
	err = srv.DidChangeTextDocument(conn, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
			Version:                2,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{
			{Text: "invalid code {{{{"},
		},
	})
	require.NoError(t, err)

	diags := conn.getDiagnostics()
	require.Len(t, diags, 2, "should have 2 publish calls (open + change)")

	// First publish (open): should be empty diagnostics.
	assert.Empty(t, diags[0].Diagnostics)
	// Second publish (change to invalid): should have diagnostics.
	assert.NotEmpty(t, diags[1].Diagnostics)
}

func TestDiagnosticLineConversion(t *testing.T) {
	// server2 Diagnostic is 1-based for lines, protocol is 0-based.
	d := Diagnostic{
		StartLine:   5,
		StartColumn: 3,
		EndLine:     5,
		EndColumn:   10,
		Message:     "test error",
		Severity:    SeverityError,
	}

	pd := convertDiagnostic(d)
	assert.Equal(t, uint32(4), pd.Range.Start.Line, "1-based line 5 -> 0-based line 4")
	assert.Equal(t, uint32(3), pd.Range.Start.Character)
	assert.Equal(t, uint32(4), pd.Range.End.Line)
	assert.Equal(t, uint32(10), pd.Range.End.Character)
	assert.Equal(t, protocol.SeverityError, pd.Severity)
	assert.Equal(t, "cadence", pd.Source)
	assert.Equal(t, "test error", pd.Message)
}

func TestCancelsMapCleanedUpAfterCheck(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///cleanup.cdc")
	openAndCheck(t, srv, conn, uri, "access(all) fun main() {}")

	srv.cancelsMu.Lock()
	count := len(srv.cancels)
	srv.cancelsMu.Unlock()

	assert.Equal(t, 0, count, "cancels map should be empty after check completes")
}

func TestDidCloseRemovesDocument(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}
	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	uri := protocol.DocumentURI("file:///close.cdc")

	err = srv.DidOpenTextDocument(conn, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: "cadence",
			Version:    1,
			Text:       "access(all) fun main() {}",
		},
	})
	require.NoError(t, err)

	_, ok := srv.getDocument(DocumentURI(uri))
	require.True(t, ok, "document should exist after open")

	err = srv.DidCloseTextDocument(conn, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
	require.NoError(t, err)

	_, ok = srv.getDocument(DocumentURI(uri))
	assert.False(t, ok, "document should be removed after close")

	checker := srv.checkerForDocument(DocumentURI(uri))
	assert.Nil(t, checker, "checker should be cleared after close")
}

func TestShutdownStopsDebouncer(t *testing.T) {
	srv := newTestServer()
	conn := &mockConn{}

	_, err := srv.Initialize(conn, &protocol.InitializeParams{})
	require.NoError(t, err)

	err = srv.Shutdown(conn)
	assert.NoError(t, err)
}
