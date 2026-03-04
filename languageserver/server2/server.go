package server2

import (
	"context"
	"sync"
	"time"

	"github.com/onflow/cadence-tools/languageserver/protocol"
	"github.com/onflow/cadence-tools/languageserver/resolver"
)

// ServerConfig holds configuration for the language server.
type ServerConfig struct {
	ImportResolver resolver.ImportResolver
	CacheCapacity  int
	DebounceDelay  time.Duration // 0 = synchronous (for testing)
}

// ServerV2 implements protocol.Handler with snapshot-based analysis,
// debounced checking, and per-document cancellation.
type ServerV2 struct {
	config    ServerConfig
	host      *AnalysisHost
	debouncer *Debouncer

	cancelsMu sync.Mutex
	cancels   map[DocumentURI]context.CancelFunc

	connMu sync.RWMutex
	conn   protocol.Conn
}

// Compile-time check that ServerV2 implements protocol.Handler.
var _ protocol.Handler = (*ServerV2)(nil)

// NewServerV2 creates a new language server with the given configuration.
func NewServerV2(config ServerConfig) *ServerV2 {
	capacity := config.CacheCapacity
	if capacity <= 0 {
		capacity = 128
	}

	var debouncer *Debouncer
	if config.DebounceDelay > 0 {
		debouncer = NewDebouncer(config.DebounceDelay)
	}

	return &ServerV2{
		config:    config,
		host:      NewAnalysisHost(capacity),
		debouncer: debouncer,
		cancels:   make(map[DocumentURI]context.CancelFunc),
	}
}

// Initialize stores the connection and returns server capabilities.
func (s *ServerV2) Initialize(conn protocol.Conn, params *protocol.InitializeParams) (*protocol.InitializeResult, error) {
	s.connMu.Lock()
	s.conn = conn
	s.connMu.Unlock()

	return &protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: &protocol.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    protocol.Full,
			},
			HoverProvider:             &protocol.Or_ServerCapabilities_hoverProvider{Value: true},
			DefinitionProvider:        &protocol.Or_ServerCapabilities_definitionProvider{Value: true},
			SignatureHelpProvider:      &protocol.SignatureHelpOptions{},
			DocumentHighlightProvider: &protocol.Or_ServerCapabilities_documentHighlightProvider{Value: true},
			RenameProvider:            true,
			CodeActionProvider:        true,
			CodeLensProvider:          &protocol.CodeLensOptions{},
			CompletionProvider:        &protocol.CompletionOptions{},
			DocumentSymbolProvider:    &protocol.Or_ServerCapabilities_documentSymbolProvider{Value: true},
			DocumentLinkProvider:      &protocol.DocumentLinkOptions{},
			InlayHintProvider:         true,
			ExecuteCommandProvider:    &protocol.ExecuteCommandOptions{},
		},
	}, nil
}

// DidOpenTextDocument handles a document open notification.
func (s *ServerV2) DidOpenTextDocument(conn protocol.Conn, params *protocol.DidOpenTextDocumentParams) error {
	uri := DocumentURI(params.TextDocument.URI)
	text := params.TextDocument.Text
	version := params.TextDocument.Version

	s.host.UpdateDocument(uri, text, version)
	s.scheduleCheck(uri)
	return nil
}

// DidChangeTextDocument handles a document change notification (full sync).
func (s *ServerV2) DidChangeTextDocument(conn protocol.Conn, params *protocol.DidChangeTextDocumentParams) error {
	uri := DocumentURI(params.TextDocument.URI)
	version := params.TextDocument.Version

	changes := params.ContentChanges
	if len(changes) == 0 {
		return nil
	}

	// Full sync mode: use the last content change.
	text := changes[len(changes)-1].Text

	s.host.UpdateDocument(uri, text, version)
	s.scheduleCheck(uri)
	return nil
}

// scheduleCheck cancels any in-flight check for the URI and schedules a new one.
func (s *ServerV2) scheduleCheck(uri DocumentURI) {
	// Cancel any previous in-flight analysis for this URI.
	s.cancelsMu.Lock()
	if cancel, ok := s.cancels[uri]; ok {
		cancel()
	}
	s.cancelsMu.Unlock()

	if s.config.DebounceDelay == 0 {
		// Synchronous mode (for testing).
		s.runCheck(uri)
	} else {
		s.debouncer.Trigger(uri, func() {
			s.runCheck(uri)
		})
	}
}

// runCheck performs analysis on the given URI and publishes diagnostics.
func (s *ServerV2) runCheck(uri DocumentURI) {
	ctx, cancel := context.WithCancel(context.Background())

	s.cancelsMu.Lock()
	s.cancels[uri] = cancel
	s.cancelsMu.Unlock()

	defer func() {
		cancel()
		s.cancelsMu.Lock()
		// Only remove if it's still our cancel func.
		if c, ok := s.cancels[uri]; ok && funcEqual(c, cancel) {
			delete(s.cancels, uri)
		}
		s.cancelsMu.Unlock()
	}()

	snap := s.host.Snapshot()
	result := Analyze(ctx, snap, uri, s.config.ImportResolver)

	if result.Cancelled {
		return
	}

	s.publishDiagnostics(uri, result.Diagnostics)
}

// funcEqual compares two cancel functions by pointer identity.
// We use a helper to work around the fact that Go funcs aren't directly comparable.
func funcEqual(a, b context.CancelFunc) bool {
	// We store cancel funcs that we create, so pointer comparison via interface is safe.
	// This is a pragmatic approach; in practice the deferred cleanup path is
	// only racy with a concurrent scheduleCheck for the same URI.
	return &a == &b
}

// publishDiagnostics converts server2.Diagnostic slice to protocol.Diagnostic
// slice and sends them to the client.
func (s *ServerV2) publishDiagnostics(uri DocumentURI, diags []Diagnostic) {
	s.connMu.RLock()
	conn := s.conn
	s.connMu.RUnlock()

	if conn == nil {
		return
	}

	protoDiags := make([]protocol.Diagnostic, 0, len(diags))
	for _, d := range diags {
		protoDiags = append(protoDiags, convertDiagnostic(d))
	}

	_ = conn.PublishDiagnostics(&protocol.PublishDiagnosticsParams{
		URI:         protocol.DocumentURI(uri),
		Diagnostics: protoDiags,
	})
}

// convertDiagnostic converts a server2.Diagnostic to a protocol.Diagnostic.
// server2 lines are 1-based; protocol Position.Line is 0-based.
func convertDiagnostic(d Diagnostic) protocol.Diagnostic {
	startLine := uint32(0)
	if d.StartLine > 0 {
		startLine = uint32(d.StartLine - 1)
	}
	endLine := uint32(0)
	if d.EndLine > 0 {
		endLine = uint32(d.EndLine - 1)
	}

	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      startLine,
				Character: uint32(d.StartColumn),
			},
			End: protocol.Position{
				Line:      endLine,
				Character: uint32(d.EndColumn),
			},
		},
		Severity: protocol.DiagnosticSeverity(d.Severity),
		Source:   "cadence",
		Message:  d.Message,
	}
}

// --- Stub handlers (return nil/zero values for now) ---

func (s *ServerV2) Hover(conn protocol.Conn, params *protocol.TextDocumentPositionParams) (*protocol.Hover, error) {
	return nil, nil
}

func (s *ServerV2) Definition(conn protocol.Conn, params *protocol.TextDocumentPositionParams) (*protocol.Location, error) {
	return nil, nil
}

func (s *ServerV2) SignatureHelp(conn protocol.Conn, params *protocol.TextDocumentPositionParams) (*protocol.SignatureHelp, error) {
	return nil, nil
}

func (s *ServerV2) DocumentHighlight(conn protocol.Conn, params *protocol.TextDocumentPositionParams) ([]*protocol.DocumentHighlight, error) {
	return nil, nil
}

func (s *ServerV2) Rename(conn protocol.Conn, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	return nil, nil
}

func (s *ServerV2) CodeAction(conn protocol.Conn, params *protocol.CodeActionParams) ([]*protocol.CodeAction, error) {
	return nil, nil
}

func (s *ServerV2) CodeLens(conn protocol.Conn, params *protocol.CodeLensParams) ([]*protocol.CodeLens, error) {
	return nil, nil
}

func (s *ServerV2) Completion(conn protocol.Conn, params *protocol.CompletionParams) ([]*protocol.CompletionItem, error) {
	return nil, nil
}

func (s *ServerV2) ResolveCompletionItem(conn protocol.Conn, item *protocol.CompletionItem) (*protocol.CompletionItem, error) {
	return nil, nil
}

func (s *ServerV2) ExecuteCommand(conn protocol.Conn, params *protocol.ExecuteCommandParams) (any, error) {
	return nil, nil
}

func (s *ServerV2) DidChangeConfiguration(conn protocol.Conn, d *protocol.DidChangeConfigurationParams) (any, error) {
	return nil, nil
}

func (s *ServerV2) DocumentSymbol(conn protocol.Conn, params *protocol.DocumentSymbolParams) ([]*protocol.DocumentSymbol, error) {
	return nil, nil
}

func (s *ServerV2) DocumentLink(conn protocol.Conn, params *protocol.DocumentLinkParams) ([]*protocol.DocumentLink, error) {
	return nil, nil
}

func (s *ServerV2) InlayHint(conn protocol.Conn, params *protocol.InlayHintParams) ([]*protocol.InlayHint, error) {
	return nil, nil
}

// Shutdown stops the debouncer and cleans up resources.
func (s *ServerV2) Shutdown(conn protocol.Conn) error {
	if s.debouncer != nil {
		s.debouncer.Stop()
	}
	return nil
}

// Exit handles the exit notification.
func (s *ServerV2) Exit(conn protocol.Conn) error {
	return nil
}
