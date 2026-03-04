package server2

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/onflow/cadence/ast"
	"github.com/onflow/cadence/common"
	"github.com/onflow/cadence/sema"

	"github.com/onflow/cadence-tools/languageserver/conversion"
	"github.com/onflow/cadence-tools/languageserver/protocol"

	linter "github.com/onflow/cadence-tools/lint"
)

// CompletionItemData is the data attached to a CompletionItem so
// ResolveCompletionItem can look up the member resolver or range.
type CompletionItemData struct {
	URI protocol.DocumentURI `json:"uri"`
	ID  string               `json:"id"`
}

// --- Helper methods ---

// checkerForDocument retrieves the cached checker for the given URI.
// The cache key must match the one used by Analyze: CanonicalCacheKey(common.StringLocation(uri)).
func (s *ServerV2) checkerForDocument(uri DocumentURI) *sema.Checker {
	cacheKey := CanonicalCacheKey(common.StringLocation(uri))
	entry, ok := s.host.Cache().Get(cacheKey)
	if !ok || entry.Checker == nil {
		return nil
	}
	return entry.Checker
}

// getDocument retrieves the document for the given URI from the AnalysisHost.
func (s *ServerV2) getDocument(uri DocumentURI) (Document, bool) {
	return s.host.GetDocument(uri)
}

// --- Hover ---

func (s *ServerV2) Hover(
	_ protocol.Conn,
	params *protocol.TextDocumentPositionParams,
) (*protocol.Hover, error) {

	uri := DocumentURI(params.TextDocument.URI)
	checker := s.checkerForDocument(uri)
	if checker == nil {
		return nil, nil
	}

	position := conversion.ProtocolToSemaPosition(params.Position)
	occurrence := checker.PositionInfo.Occurrences.Find(position)
	if occurrence == nil {
		// Try the preceding position
		if position.Column > 0 {
			previousPosition := position
			previousPosition.Column -= 1
			occurrence = checker.PositionInfo.Occurrences.Find(previousPosition)
		}
	}

	if occurrence == nil || occurrence.Origin == nil {
		return nil, nil
	}

	var markup strings.Builder
	_, _ = fmt.Fprintf(
		&markup,
		"**Type**\n\n```cadence\n%s\n```\n",
		documentType(occurrence.Origin.Type),
	)

	docString := occurrence.Origin.DocString
	if docString != "" {
		_, _ = fmt.Fprintf(
			&markup,
			"\n**Documentation**\n\n%s\n",
			docString,
		)
	}

	contents := protocol.MarkupContent{
		Kind:  protocol.Markdown,
		Value: markup.String(),
	}
	return &protocol.Hover{
		Contents: contents,
		Range: conversion.SemaToProtocolRange(
			occurrence.StartPos,
			occurrence.EndPos,
		),
	}, nil
}

func documentType(ty sema.Type) string {
	if functionType, ok := ty.(*sema.FunctionType); ok {
		return documentFunctionType(functionType)
	}
	return ty.QualifiedString()
}

func documentFunctionType(ty *sema.FunctionType) string {
	var builder strings.Builder
	builder.WriteString("fun ")
	if len(ty.TypeParameters) > 0 {
		builder.WriteRune('<')
		for i, typeParameter := range ty.TypeParameters {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(typeParameter.QualifiedString())
		}
		builder.WriteRune('>')
	}
	builder.WriteRune('(')
	for i, parameter := range ty.Parameters {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(parameter.QualifiedString())
	}
	builder.WriteString(")")

	if ty.ReturnTypeAnnotation.Type != sema.VoidType {
		builder.WriteString(": ")
		builder.WriteString(ty.ReturnTypeAnnotation.QualifiedString())
	}
	return builder.String()
}

// --- Definition ---

func (s *ServerV2) Definition(
	_ protocol.Conn,
	params *protocol.TextDocumentPositionParams,
) (*protocol.Location, error) {

	uri := DocumentURI(params.TextDocument.URI)
	checker := s.checkerForDocument(uri)
	if checker == nil {
		return nil, nil
	}

	position := conversion.ProtocolToSemaPosition(params.Position)
	occurrence := checker.PositionInfo.Occurrences.Find(position)

	if occurrence == nil {
		return nil, nil
	}

	origin := occurrence.Origin
	if origin == nil || origin.StartPos == nil || origin.EndPos == nil {
		return nil, nil
	}

	return &protocol.Location{
		URI: protocol.DocumentURI(uri),
		Range: conversion.ASTToProtocolRange(
			*origin.StartPos,
			*origin.EndPos,
		),
	}, nil
}

// --- SignatureHelp ---

func (s *ServerV2) SignatureHelp(
	_ protocol.Conn,
	params *protocol.TextDocumentPositionParams,
) (*protocol.SignatureHelp, error) {

	uri := DocumentURI(params.TextDocument.URI)
	checker := s.checkerForDocument(uri)
	if checker == nil {
		return nil, nil
	}

	position := conversion.ProtocolToSemaPosition(params.Position)
	invocation := checker.PositionInfo.FunctionInvocations.Find(position)

	if invocation == nil {
		return nil, nil
	}

	functionType := invocation.FunctionType

	signatureLabelParts := make([]string, 0, len(functionType.Parameters))
	argumentLabels := functionType.ArgumentLabels()

	for i, parameter := range functionType.Parameters {
		argumentLabel := argumentLabels[i]
		typeAnnotation := parameter.TypeAnnotation.QualifiedString()

		var signatureLabelPart string
		if argumentLabel == sema.ArgumentLabelNotRequired {
			signatureLabelPart = typeAnnotation
		} else {
			signatureLabelPart = fmt.Sprintf("%s: %s", argumentLabel, typeAnnotation)
		}

		signatureLabelParts = append(signatureLabelParts, signatureLabelPart)
	}

	signatureLabel := fmt.Sprintf(
		"(%s): %s",
		strings.Join(signatureLabelParts, ", "),
		functionType.ReturnTypeAnnotation.QualifiedString(),
	)

	signatureParameters := make([]protocol.ParameterInformation, 0, len(signatureLabelParts))
	for _, part := range signatureLabelParts {
		signatureParameters = append(signatureParameters, protocol.ParameterInformation{
			Label: part,
		})
	}

	var activeParameter uint32
	for _, trailingSeparatorPosition := range invocation.TrailingSeparatorPositions {
		if position.Compare(sema.ASTToSemaPosition(trailingSeparatorPosition)) > 0 {
			activeParameter++
		}
	}

	return &protocol.SignatureHelp{
		Signatures: []protocol.SignatureInformation{
			{
				Label:      signatureLabel,
				Parameters: signatureParameters,
			},
		},
		ActiveParameter: activeParameter,
	}, nil
}

// --- DocumentHighlight ---

func (s *ServerV2) DocumentHighlight(
	_ protocol.Conn,
	params *protocol.TextDocumentPositionParams,
) ([]*protocol.DocumentHighlight, error) {

	uri := DocumentURI(params.TextDocument.URI)
	checker := s.checkerForDocument(uri)
	if checker == nil {
		return nil, nil
	}

	position := conversion.ProtocolToSemaPosition(params.Position)
	occurrences := checker.PositionInfo.Occurrences.FindAll(position)
	// If there are no occurrences, try the preceding position
	if len(occurrences) == 0 && position.Column > 0 {
		previousPosition := position
		previousPosition.Column -= 1
		occurrences = checker.PositionInfo.Occurrences.FindAll(previousPosition)
	}

	documentHighlights := make([]*protocol.DocumentHighlight, 0)

	for _, occurrence := range occurrences {
		origin := occurrence.Origin
		if origin == nil || origin.StartPos == nil || origin.EndPos == nil {
			continue
		}

		for _, occurrenceRange := range origin.Occurrences {
			documentHighlights = append(documentHighlights,
				&protocol.DocumentHighlight{
					Range: conversion.ASTToProtocolRange(
						occurrenceRange.StartPos,
						occurrenceRange.EndPos,
					),
				},
			)
		}
	}

	return documentHighlights, nil
}

// --- Rename ---

func (s *ServerV2) Rename(
	_ protocol.Conn,
	params *protocol.RenameParams,
) (*protocol.WorkspaceEdit, error) {

	uri := DocumentURI(params.TextDocument.URI)
	checker := s.checkerForDocument(uri)
	if checker == nil {
		return nil, nil
	}

	position := conversion.ProtocolToSemaPosition(params.Position)
	occurrences := checker.PositionInfo.Occurrences.FindAll(position)
	// If there are no occurrences, try the preceding position
	if len(occurrences) == 0 && position.Column > 0 {
		previousPosition := position
		previousPosition.Column -= 1
		occurrences = checker.PositionInfo.Occurrences.FindAll(previousPosition)
	}

	textEdits := make([]protocol.TextEdit, 0)

	for _, occurrence := range occurrences {
		origin := occurrence.Origin
		if origin == nil || origin.StartPos == nil || origin.EndPos == nil {
			continue
		}

		for _, occurrenceRange := range origin.Occurrences {
			textEdits = append(textEdits,
				protocol.TextEdit{
					Range: conversion.ASTToProtocolRange(
						occurrenceRange.StartPos,
						occurrenceRange.EndPos,
					),
					NewText: params.NewName,
				},
			)
		}
	}

	return &protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			protocol.DocumentURI(uri): textEdits,
		},
	}, nil
}

// --- CodeAction ---

func (s *ServerV2) CodeAction(
	_ protocol.Conn,
	params *protocol.CodeActionParams,
) ([]*protocol.CodeAction, error) {
	// NOTE: Always initialize to an empty slice, i.e DON'T use nil:
	// nil will be ignored instead of being treated as no items
	codeActions := []*protocol.CodeAction{}

	uri := DocumentURI(params.TextDocument.URI)
	checker := s.checkerForDocument(uri)
	if checker == nil {
		return codeActions, nil
	}

	document, ok := s.getDocument(uri)
	if !ok {
		return codeActions, nil
	}

	// Resolve diagnostic code actions
	s.codeActionsResolversMu.RLock()
	codeActionsResolvers := s.codeActionsResolvers[uri]
	s.codeActionsResolversMu.RUnlock()

	if codeActionsResolvers != nil {
		for _, diagnostic := range params.Context.Diagnostics {
			if data, ok := diagnostic.Data.(string); ok {
				codeActionsResolver, ok := codeActionsResolvers[data]
				if !ok {
					continue
				}
				codeActions = append(codeActions, codeActionsResolver()...)
			}
		}
	}

	// Split-lines refactoring (invoked code actions)
	triggerKind := params.Context.TriggerKind
	if triggerKind != nil && *triggerKind == protocol.CodeActionInvoked {
		ast.Inspect(checker.Program, func(element ast.Element) bool {
			switch element := element.(type) {
			case *ast.InvocationExpression:
				if codeAction := maybeSplitLinesContainerElementsCodeAction(
					protocol.DocumentURI(uri),
					document,
					params.Range,
					element,
					element.Arguments,
					"arguments",
				); codeAction != nil {
					codeActions = append(codeActions, codeAction)
				}

			case *ast.ArrayExpression:
				if codeAction := maybeSplitLinesContainerElementsCodeAction(
					protocol.DocumentURI(uri),
					document,
					params.Range,
					element,
					element.Values,
					"elements",
				); codeAction != nil {
					codeActions = append(codeActions, codeAction)
				}

			case *ast.DictionaryExpression:
				var entryPositions []ast.HasPosition
				for _, entry := range element.Entries {
					entryPositions = append(
						entryPositions,
						ast.NewUnmeteredRange(
							entry.Key.StartPosition(),
							entry.Value.EndPosition(nil),
						),
					)
				}
				if codeAction := maybeSplitLinesContainerElementsCodeAction(
					protocol.DocumentURI(uri),
					document,
					params.Range,
					element,
					entryPositions,
					"entries",
				); codeAction != nil {
					codeActions = append(codeActions, codeAction)
				}

			case *ast.FunctionDeclaration:
				parameterList := element.ParameterList
				if codeAction := maybeSplitLinesContainerElementsCodeAction(
					protocol.DocumentURI(uri),
					document,
					params.Range,
					parameterList,
					parameterList.Parameters,
					"parameters",
				); codeAction != nil {
					codeActions = append(codeActions, codeAction)
				}

			case *ast.SpecialFunctionDeclaration:
				parameterList := element.FunctionDeclaration.ParameterList
				if codeAction := maybeSplitLinesContainerElementsCodeAction(
					protocol.DocumentURI(uri),
					document,
					params.Range,
					parameterList,
					parameterList.Parameters,
					"parameters",
				); codeAction != nil {
					codeActions = append(codeActions, codeAction)
				}

			case *ast.FunctionExpression:
				parameterList := element.ParameterList
				if codeAction := maybeSplitLinesContainerElementsCodeAction(
					protocol.DocumentURI(uri),
					document,
					params.Range,
					parameterList,
					parameterList.Parameters,
					"parameters",
				); codeAction != nil {
					codeActions = append(codeActions, codeAction)
				}
			}
			return true
		})
	}

	return codeActions, nil
}

const indentationCount = 4

func extractIndentation(text string, pos ast.Position) string {
	lineStartOffset := pos.Offset - pos.Column
	indentationEndOffset := lineStartOffset
	for ; indentationEndOffset < pos.Offset; indentationEndOffset++ {
		switch text[indentationEndOffset] {
		case ' ', '\t':
			continue
		}
		break
	}
	return text[lineStartOffset:indentationEndOffset]
}

func maybeSplitLinesContainerElementsCodeAction[E ast.HasPosition](
	uri protocol.DocumentURI,
	document Document,
	requestedRange protocol.Range,
	container ast.HasPosition,
	elements []E,
	pluralElementDescription string,
) *protocol.CodeAction {
	elementCount := len(elements)
	if elementCount < 2 {
		return nil
	}

	firstElement := elements[0]
	lastElement := elements[elementCount-1]

	firstElementStartPos := firstElement.StartPosition()
	lastElementEndPos := lastElement.EndPosition(nil)

	lastElementEndProtocolPos := conversion.ASTToProtocolPosition(
		lastElementEndPos.Shifted(nil, 1),
	)

	if conversion.ASTToProtocolPosition(firstElementStartPos).Compare(requestedRange.Start) > 0 ||
		lastElementEndProtocolPos.Compare(requestedRange.End) < 0 {
		return nil
	}

	containerStartPos := container.StartPosition()
	indentation := extractIndentation(document.Text, containerStartPos)
	elementIndentation := indentation + strings.Repeat(" ", indentationCount)

	var textEdits []protocol.TextEdit

	// Insert a newline before the first element if not already on a new line
	if firstElementStartPos.Line == containerStartPos.Line {
		firstElementStartElementPos := conversion.ASTToProtocolPosition(firstElementStartPos)
		textEdits = append(textEdits, protocol.TextEdit{
			Range: protocol.Range{
				Start: firstElementStartElementPos,
				End:   firstElementStartElementPos,
			},
			NewText: "\n" + elementIndentation,
		})
	}

	// Insert a newline before each element on the same line as the previous
	for i := 1; i < len(elements); i++ {
		currentElement := elements[i]
		previousElement := elements[i-1]

		currentStartPosition := currentElement.StartPosition()
		previousEndPosition := previousElement.EndPosition(nil)

		if currentStartPosition.Line != previousEndPosition.Line {
			continue
		}

		newlinePosition := conversion.ASTToProtocolPosition(currentStartPosition)
		textEdits = append(textEdits, protocol.TextEdit{
			Range: protocol.Range{
				Start: newlinePosition,
				End:   newlinePosition,
			},
			NewText: "\n" + elementIndentation,
		})
	}

	// Insert a newline after the last element if not already on a new line
	if lastElementEndPos.Line == container.EndPosition(nil).Line {
		textEdits = append(textEdits, protocol.TextEdit{
			Range: protocol.Range{
				Start: lastElementEndProtocolPos,
				End:   lastElementEndProtocolPos,
			},
			NewText: "\n" + indentation,
		})
	}

	if len(textEdits) == 0 {
		return nil
	}

	return &protocol.CodeAction{
		Title: fmt.Sprintf("Split %s onto separate lines", pluralElementDescription),
		Kind:  protocol.RefactorRewrite,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[protocol.DocumentURI][]protocol.TextEdit{
				uri: textEdits,
			},
		},
	}
}

// --- CodeLens ---

func (s *ServerV2) CodeLens(
	_ protocol.Conn,
	params *protocol.CodeLensParams,
) ([]*protocol.CodeLens, error) {
	// Return empty slice (stub for now, no code lens providers registered yet)
	return []*protocol.CodeLens{}, nil
}

// --- Completion ---

func (s *ServerV2) Completion(
	_ protocol.Conn,
	params *protocol.CompletionParams,
) ([]*protocol.CompletionItem, error) {
	// NOTE: Always initialize to an empty slice
	items := []*protocol.CompletionItem{}

	uri := DocumentURI(params.TextDocument.URI)
	checker := s.checkerForDocument(uri)
	if checker == nil {
		return items, nil
	}

	document, ok := s.getDocument(uri)
	if !ok {
		return items, nil
	}

	position := conversion.ProtocolToSemaPosition(params.Position)

	memberCompletions := s.memberCompletions(position, checker, uri)
	if len(memberCompletions) > 0 {
		return memberCompletions, nil
	}

	// Prioritize range completion items over other items
	rangeCompletions := s.rangeCompletions(position, checker, uri)
	for _, item := range rangeCompletions {
		item.SortText = "1" + item.Label
	}
	items = append(items, rangeCompletions...)

	items = append(items, statementCompletionItems...)
	items = append(items, expressionCompletionItems...)

	requiresAccessModifierPlaceholder :=
		!documentHasAnyPrecedingStringsAtPosition(document, allAccessOptions, position.Line, position.Column)

	for _, item := range declarationCompletionItems {
		if requiresAccessModifierPlaceholder {
			item = withCompletionItemInsertText(
				item,
				fmt.Sprintf("${1|%s|} %s", allAccessOptionsCommaSeparated, item.InsertText),
			)
		}
		items = append(items, item)
	}

	for _, item := range containerCompletionItems {
		if requiresAccessModifierPlaceholder {
			item = withCompletionItemInsertText(
				item,
				fmt.Sprintf("${1|%s|} %s", allAccessOptionsCommaSeparated, item.InsertText),
			)
		}
		items = append(items, item)
	}

	return items, nil
}

func withCompletionItemInsertText(item *protocol.CompletionItem, insertText string) *protocol.CompletionItem {
	itemCopy := *item
	itemCopy.InsertText = insertText
	return &itemCopy
}

func (s *ServerV2) memberCompletions(
	position sema.Position,
	checker *sema.Checker,
	uri DocumentURI,
) (items []*protocol.CompletionItem) {

	// The client asks for the column after the identifier,
	// query the member accesses for the preceding position
	if position.Column > 0 {
		position.Column -= 1
	}
	memberAccess := checker.PositionInfo.MemberAccesses.Find(position)

	s.memberResolversMu.Lock()
	delete(s.memberResolvers, uri)
	s.memberResolversMu.Unlock()

	if memberAccess == nil {
		return
	}

	memberResolvers := memberAccess.AccessedType.GetMembers()

	s.memberResolversMu.Lock()
	s.memberResolvers[uri] = memberResolvers
	s.memberResolversMu.Unlock()

	for name, resolver := range memberResolvers {
		kind := conversion.DeclarationKindToCompletionItemType(resolver.Kind)
		commitCharacters := declarationKindCommitCharacters(resolver.Kind)

		item := &protocol.CompletionItem{
			Label:            name,
			Kind:             kind,
			CommitCharacters: commitCharacters,
			Data: CompletionItemData{
				URI: protocol.DocumentURI(uri),
				ID:  name,
			},
		}

		// If the member is a function, also prepare the argument list
		if resolver.Kind == common.DeclarationKindFunction {
			s.prepareFunctionMemberCompletionItem(item, resolver, name)

			item.Command = &protocol.Command{
				Command: "editor.action.triggerParameterHints",
			}
		}

		items = append(items, item)
	}

	return items
}

func (s *ServerV2) rangeCompletions(
	position sema.Position,
	checker *sema.Checker,
	uri DocumentURI,
) (items []*protocol.CompletionItem) {

	ranges := checker.PositionInfo.Ranges.FindAll(position)

	s.rangesMu.Lock()
	delete(s.ranges, uri)
	s.rangesMu.Unlock()

	if ranges == nil {
		return
	}

	resolvers := make(map[string]sema.Range, len(ranges))

	s.rangesMu.Lock()
	s.ranges[uri] = resolvers
	s.rangesMu.Unlock()

	for index, r := range ranges {
		id := strconv.Itoa(index)
		kind := conversion.DeclarationKindToCompletionItemType(r.DeclarationKind)
		item := &protocol.CompletionItem{
			Label: r.Identifier,
			Kind:  kind,
			Data: CompletionItemData{
				URI: protocol.DocumentURI(uri),
				ID:  id,
			},
		}

		resolvers[id] = r

		var isFunctionCompletion bool

		switch r.DeclarationKind {
		case common.DeclarationKindFunction:
			functionType := r.Type.(*sema.FunctionType)
			s.prepareParametersCompletionItem(item, r.Identifier, functionType.Parameters)
			isFunctionCompletion = true

		case common.DeclarationKindStructure,
			common.DeclarationKindResource,
			common.DeclarationKindEvent:

			if functionType, ok := r.Type.(*sema.FunctionType); ok && functionType.IsConstructor {
				item.Kind = protocol.ConstructorCompletion
				s.prepareParametersCompletionItem(item, r.Identifier, functionType.Parameters)
				isFunctionCompletion = true
			}
		}

		if isFunctionCompletion {
			item.Command = &protocol.Command{
				Command: "editor.action.triggerParameterHints",
			}
		}

		items = append(items, item)
	}

	return items
}

func (s *ServerV2) prepareFunctionMemberCompletionItem(
	item *protocol.CompletionItem,
	resolver sema.MemberResolver,
	name string,
) {
	member := resolver.Resolve(nil, item.Label, ast.Range{}, func(err error) { /* NO-OP */ })
	functionType, ok := member.TypeAnnotation.Type.(*sema.FunctionType)
	if !ok {
		return
	}

	if linter.MemberIsDeprecated(member.DocString) {
		item.Tags = append(item.Tags, protocol.ComplDeprecated)
	}

	s.prepareParametersCompletionItem(item, name, functionType.Parameters)
}

func (s *ServerV2) prepareParametersCompletionItem(
	item *protocol.CompletionItem,
	name string,
	parameters []sema.Parameter,
) {
	item.InsertTextFormat = &insertTextFormat

	var builder strings.Builder
	builder.WriteString(name)
	builder.WriteRune('(')

	for i, parameter := range parameters {
		if i > 0 {
			builder.WriteString(", ")
		}
		label := parameter.EffectiveArgumentLabel()
		if label != sema.ArgumentLabelNotRequired {
			builder.WriteString(label)
			builder.WriteString(": ")
		}
		builder.WriteString("${")
		builder.WriteString(strconv.Itoa(i + 1))
		builder.WriteRune(':')
		builder.WriteString(parameter.Identifier)
		builder.WriteRune('}')
	}

	builder.WriteRune(')')
	item.InsertText = builder.String()
}

func declarationKindCommitCharacters(kind common.DeclarationKind) []string {
	switch kind {
	case common.DeclarationKindField:
		return []string{"."}
	default:
		return nil
	}
}

// --- ResolveCompletionItem ---

func (s *ServerV2) ResolveCompletionItem(
	_ protocol.Conn,
	item *protocol.CompletionItem,
) (*protocol.CompletionItem, error) {
	result := item

	// Extract data (CompletionItemData) from the item
	data, ok := item.Data.(CompletionItemData)
	if !ok {
		// Try to extract from map (JSON deserialization may produce a map)
		if m, ok := item.Data.(map[string]interface{}); ok {
			if uriVal, ok := m["uri"].(string); ok {
				data.URI = protocol.DocumentURI(uriVal)
			}
			if idVal, ok := m["id"].(string); ok {
				data.ID = idVal
			}
		} else {
			return result, nil
		}
	}

	if s.maybeResolveMember(DocumentURI(data.URI), data.ID, result) {
		return result, nil
	}

	if s.maybeResolveRange(DocumentURI(data.URI), data.ID, result) {
		return result, nil
	}

	return result, nil
}

func (s *ServerV2) maybeResolveMember(uri DocumentURI, id string, result *protocol.CompletionItem) bool {
	s.memberResolversMu.RLock()
	memberResolvers, ok := s.memberResolvers[uri]
	s.memberResolversMu.RUnlock()

	if !ok {
		return false
	}

	resolver, ok := memberResolvers[id]
	if !ok {
		return false
	}

	member := resolver.Resolve(nil, result.Label, ast.Range{}, func(err error) { /* NO-OP */ })

	result.Documentation = &protocol.Or_CompletionItem_documentation{
		Value: protocol.MarkupContent{
			Kind:  "markdown",
			Value: member.DocString,
		},
	}

	switch member.DeclarationKind {
	case common.DeclarationKindField:
		typeString := member.TypeAnnotation.Type.QualifiedString()
		result.Detail = fmt.Sprintf(
			"%s.%s: %s",
			member.ContainerType.String(),
			member.Identifier,
			typeString,
		)
		if member.VariableKind != ast.VariableKindNotSpecified {
			result.Detail = fmt.Sprintf("(%s) %s",
				member.VariableKind.Name(),
				result.Detail,
			)
		}

	case common.DeclarationKindFunction:
		typeString := member.TypeAnnotation.Type.QualifiedString()
		result.Detail = fmt.Sprintf(
			"(function) %s.%s: %s",
			member.ContainerType.String(),
			member.Identifier,
			typeString,
		)

	case common.DeclarationKindStructure,
		common.DeclarationKindResource,
		common.DeclarationKindEvent,
		common.DeclarationKindContract,
		common.DeclarationKindStructureInterface,
		common.DeclarationKindResourceInterface,
		common.DeclarationKindContractInterface:

		result.Detail = fmt.Sprintf(
			"(%s) %s.%s",
			member.DeclarationKind.Name(),
			member.ContainerType.String(),
			member.Identifier,
		)
	}

	return true
}

func (s *ServerV2) maybeResolveRange(uri DocumentURI, id string, result *protocol.CompletionItem) bool {
	s.rangesMu.RLock()
	ranges, ok := s.ranges[uri]
	s.rangesMu.RUnlock()

	if !ok {
		return false
	}

	r, ok := ranges[id]
	if !ok {
		return false
	}

	if functionType, ok := r.Type.(*sema.FunctionType); ok && functionType.IsConstructor {
		typeString := functionType.QualifiedString()
		result.Detail = fmt.Sprintf("(constructor) %s", typeString)
	} else {
		result.Detail = fmt.Sprintf("(%s) %s", r.DeclarationKind.Name(), r.Type.String())
	}

	result.Documentation = &protocol.Or_CompletionItem_documentation{
		Value: r.DocString,
	}

	return true
}

// --- DocumentSymbol ---

func (s *ServerV2) DocumentSymbol(
	_ protocol.Conn,
	params *protocol.DocumentSymbolParams,
) ([]*protocol.DocumentSymbol, error) {
	// NOTE: Always initialize to an empty slice
	symbols := []*protocol.DocumentSymbol{}

	uri := DocumentURI(params.TextDocument.URI)
	checker := s.checkerForDocument(uri)
	if checker == nil {
		return symbols, nil
	}

	for _, declaration := range checker.Program.Declarations() {
		symbol := conversion.DeclarationToDocumentSymbol(declaration)
		if strings.TrimSpace(symbol.Name) != "" {
			symbols = append(symbols, &symbol)
		}
	}

	return symbols, nil
}

// --- DocumentLink ---

func (s *ServerV2) DocumentLink(
	_ protocol.Conn,
	_ *protocol.DocumentLinkParams,
) ([]*protocol.DocumentLink, error) {
	return nil, nil
}

// --- InlayHint ---

func (s *ServerV2) InlayHint(
	_ protocol.Conn,
	params *protocol.InlayHintParams,
) ([]*protocol.InlayHint, error) {
	// NOTE: Always initialize to an empty slice
	inlayHints := []*protocol.InlayHint{}

	uri := DocumentURI(params.TextDocument.URI)
	checker := s.checkerForDocument(uri)
	if checker == nil {
		return inlayHints, nil
	}

	var variableDeclarations []*ast.VariableDeclaration

	// Find all variable declarations without type annotations within the requested range
	ast.Inspect(checker.Program, func(element ast.Element) bool {
		variableDeclaration, ok := element.(*ast.VariableDeclaration)
		if !ok || variableDeclaration.TypeAnnotation != nil {
			return true
		}

		declRange := conversion.ASTToProtocolRange(
			variableDeclaration.StartPos,
			variableDeclaration.EndPosition(nil),
		)

		if params.Range.Overlaps(declRange) {
			variableDeclarations = append(variableDeclarations, variableDeclaration)
		}

		return true
	})

	// For each variable declaration, get its inferred type and create an inlay hint
	for _, variableDeclaration := range variableDeclarations {
		targetType := checker.Elaboration.VariableDeclarationTypes(variableDeclaration).TargetType
		if targetType == nil || targetType.IsInvalidType() {
			continue
		}

		typeAnnotation := sema.NewTypeAnnotation(targetType)
		typeAnnotationString := fmt.Sprintf(": %s", typeAnnotation.QualifiedString())

		identifierEndPosition := variableDeclaration.Identifier.EndPosition(nil)
		inlayHintPosition := conversion.ASTToProtocolPosition(identifierEndPosition.Shifted(nil, 1))
		inlayHint := protocol.InlayHint{
			Position: inlayHintPosition,
			Label: []protocol.InlayHintLabelPart{
				{
					Value: typeAnnotationString,
				},
			},
			Kind: protocol.Type,
			TextEdits: []protocol.TextEdit{
				{
					Range: protocol.Range{
						Start: inlayHintPosition,
						End:   inlayHintPosition,
					},
					NewText: typeAnnotationString,
				},
			},
		}

		inlayHints = append(inlayHints, &inlayHint)
	}

	return inlayHints, nil
}

// --- ExecuteCommand ---

func (s *ServerV2) ExecuteCommand(
	_ protocol.Conn,
	_ *protocol.ExecuteCommandParams,
) (any, error) {
	// Stub for now - no commands registered yet
	return nil, nil
}

// --- Document helpers (used by CodeAction split-lines) ---

// documentOffset computes the byte offset for a given 1-based line and 0-based column.
func documentOffset(text string, line, column int) int {
	reader := bufio.NewReader(strings.NewReader(text))
	offset := 0
	for i := 1; i < line; i++ {
		l, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return -1
		}
		offset += len(l)
	}
	return offset + column
}

// documentHasAnyPrecedingStringsAtPosition checks if any of the given options
// appear before the given position (after skipping preceding non-whitespace and then whitespace).
func documentHasAnyPrecedingStringsAtPosition(doc Document, options []string, line, column int) bool {
	endOffset := documentOffset(doc.Text, line, column)
	if endOffset >= len(doc.Text) {
		endOffset = len(doc.Text) - 1
	}
	if endOffset < 0 {
		return false
	}

	isWhitespace := func(c byte) bool {
		return c == ' ' || c == '\t' || c == '\n'
	}

	skip := func(predicate func(byte) bool) (done bool) {
		for {
			c := doc.Text[endOffset]
			if !predicate(c) {
				break
			}
			endOffset--
			if endOffset < 0 {
				return true
			}
		}
		return false
	}

	// Skip preceding non-whitespace
	done := skip(func(c byte) bool {
		return !isWhitespace(c)
	})
	if done {
		return false
	}

	// Skip preceding whitespace
	done = skip(isWhitespace)
	if done {
		return false
	}

	// Check if any of the options matches
	for _, option := range options {
		optLen := len(option)
		startOffset := endOffset - optLen + 1
		if startOffset < 0 {
			continue
		}
		subStr := doc.Text[startOffset : endOffset+1]
		if subStr == option {
			return true
		}
	}

	return false
}

// CodeActionResolver is a function that resolves code actions lazily.
type CodeActionResolver = func() []*protocol.CodeAction

// completionMaps holds the mutex-protected maps used for completion resolution.
// These are stored as fields on ServerV2.

var _ = sync.RWMutex{} // ensure sync is used
