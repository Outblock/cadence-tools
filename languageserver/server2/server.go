package server2

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/onflow/cadence/sema"

	"github.com/onflow/cadence-tools/languageserver/protocol"
	"github.com/onflow/cadence-tools/languageserver/resolver"
)

// cancelEntry pairs a cancel function with a unique token so we can
// identify whether the entry in the map still belongs to our goroutine.
type cancelEntry struct {
	cancel context.CancelFunc
	token  uint64
}

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
	cancels   map[DocumentURI]cancelEntry
	nextToken atomic.Uint64

	connMu sync.RWMutex
	conn   protocol.Conn

	// Completion resolution state (protected by mutexes)
	memberResolversMu sync.RWMutex
	memberResolvers   map[DocumentURI]map[string]sema.MemberResolver

	rangesMu sync.RWMutex
	ranges   map[DocumentURI]map[string]sema.Range

	// Code action resolution state
	codeActionsResolversMu sync.RWMutex
	codeActionsResolvers   map[DocumentURI]map[string]CodeActionResolver
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
		config:               config,
		host:                 NewAnalysisHost(capacity),
		debouncer:            debouncer,
		cancels:              make(map[DocumentURI]cancelEntry),
		memberResolvers:      make(map[DocumentURI]map[string]sema.MemberResolver),
		ranges:               make(map[DocumentURI]map[string]sema.Range),
		codeActionsResolvers: make(map[DocumentURI]map[string]CodeActionResolver),
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
			ReferencesProvider:        &protocol.Or_ServerCapabilities_referencesProvider{Value: true},
			SignatureHelpProvider:      &protocol.SignatureHelpOptions{},
			DocumentHighlightProvider: &protocol.Or_ServerCapabilities_documentHighlightProvider{Value: true},
			RenameProvider:            true,
			CodeActionProvider:        true,
			CodeLensProvider:          &protocol.CodeLensOptions{},
			CompletionProvider:        &protocol.CompletionOptions{},
			DocumentSymbolProvider:    &protocol.Or_ServerCapabilities_documentSymbolProvider{Value: true},
			DocumentLinkProvider:      &protocol.DocumentLinkOptions{},
			InlayHintProvider:         true,
			FoldingRangeProvider:      &protocol.Or_ServerCapabilities_foldingRangeProvider{Value: true},
			SelectionRangeProvider:    &protocol.Or_ServerCapabilities_selectionRangeProvider{Value: true},
			WorkspaceSymbolProvider:   &protocol.Or_ServerCapabilities_workspaceSymbolProvider{Value: true},
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

// DidCloseTextDocument handles a document close notification.
func (s *ServerV2) DidCloseTextDocument(_ protocol.Conn, params *protocol.DidCloseTextDocumentParams) error {
	uri := DocumentURI(params.TextDocument.URI)

	// Cancel any in-flight analysis for this URI.
	s.cancelsMu.Lock()
	if entry, ok := s.cancels[uri]; ok {
		entry.cancel()
		delete(s.cancels, uri)
	}
	s.cancelsMu.Unlock()

	// Remove the document and invalidate cache.
	s.host.RemoveDocument(uri)

	// Clean up completion resolver state.
	s.memberResolversMu.Lock()
	delete(s.memberResolvers, uri)
	s.memberResolversMu.Unlock()

	s.rangesMu.Lock()
	delete(s.ranges, uri)
	s.rangesMu.Unlock()

	s.codeActionsResolversMu.Lock()
	delete(s.codeActionsResolvers, uri)
	s.codeActionsResolversMu.Unlock()

	// Publish empty diagnostics to clear any displayed diagnostics.
	s.publishDiagnostics(uri, nil)

	return nil
}

// scheduleCheck cancels any in-flight check for the URI and schedules a new one.
func (s *ServerV2) scheduleCheck(uri DocumentURI) {
	// Cancel any previous in-flight analysis for this URI.
	s.cancelsMu.Lock()
	if entry, ok := s.cancels[uri]; ok {
		entry.cancel()
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
	token := s.nextToken.Add(1)

	s.cancelsMu.Lock()
	s.cancels[uri] = cancelEntry{cancel: cancel, token: token}
	s.cancelsMu.Unlock()

	defer func() {
		cancel()
		s.cancelsMu.Lock()
		if entry, ok := s.cancels[uri]; ok && entry.token == token {
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

func (s *ServerV2) DidChangeConfiguration(_ protocol.Conn, _ *protocol.DidChangeConfigurationParams) (any, error) {
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
