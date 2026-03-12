package server2

import (
	"sort"

	"github.com/onflow/cadence/ast"
	"github.com/onflow/cadence/common"

	"github.com/onflow/cadence-tools/languageserver/conversion"
	"github.com/onflow/cadence/sema"
)

// Token type indices (must match semanticTokenTypes order).
const (
	tokenTypeNamespace = iota
	tokenTypeType
	tokenTypeClass
	tokenTypeEnum
	tokenTypeInterface
	tokenTypeStruct
	tokenTypeParameter
	tokenTypeVariable
	tokenTypeProperty
	tokenTypeFunction
	tokenTypeKeyword
	tokenTypeComment
	tokenTypeString
	tokenTypeNumber
	tokenTypeOperator
	tokenTypeDecorator
	tokenTypeEvent
)

// semanticTokenTypes defines the legend for token types.
var semanticTokenTypes = []string{
	"namespace",  // 0
	"type",       // 1
	"class",      // 2 (resource)
	"enum",       // 3
	"interface",  // 4
	"struct",     // 5
	"parameter",  // 6
	"variable",   // 7
	"property",   // 8
	"function",   // 9
	"keyword",    // 10
	"comment",    // 11
	"string",     // 12
	"number",     // 13
	"operator",   // 14
	"decorator",  // 15 (access modifiers)
	"event",      // 16
}

// Token modifier bit flags.
const (
	tokenModDeclaration = 1 << iota
	tokenModDefinition
	tokenModReadonly
	tokenModDeprecated
)

var semanticTokenModifiers = []string{
	"declaration",
	"definition",
	"readonly",
	"deprecated",
}

// rawToken is an intermediate representation before delta encoding.
type rawToken struct {
	line      uint32 // 0-based
	startChar uint32
	length    uint32
	tokenType uint32
	modifiers uint32
}

// encodeSemanticTokens takes raw tokens and produces the LSP delta-encoded uint32 array.
// Tokens must be sorted by (line, startChar).
func encodeSemanticTokens(tokens []rawToken) []uint32 {
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].line != tokens[j].line {
			return tokens[i].line < tokens[j].line
		}
		return tokens[i].startChar < tokens[j].startChar
	})

	data := make([]uint32, 0, len(tokens)*5)
	var prevLine, prevChar uint32

	for _, tok := range tokens {
		deltaLine := tok.line - prevLine
		var deltaChar uint32
		if deltaLine == 0 {
			deltaChar = tok.startChar - prevChar
		} else {
			deltaChar = tok.startChar
		}

		data = append(data, deltaLine, deltaChar, tok.length, tok.tokenType, tok.modifiers)

		prevLine = tok.line
		prevChar = tok.startChar
	}

	return data
}

// collectSemanticTokens walks the checker and produces raw tokens.
func collectSemanticTokens(checker *sema.Checker) []rawToken {
	var tokens []rawToken
	program := checker.Program

	ast.Inspect(program, func(element ast.Element) bool {
		switch e := element.(type) {
		case *ast.FunctionDeclaration:
			id := e.Identifier
			tokens = append(tokens, makeToken(id.StartPosition(), id.EndPosition(nil), tokenTypeFunction, tokenModDeclaration))

		case *ast.SpecialFunctionDeclaration:
			fd := e.FunctionDeclaration
			id := fd.Identifier
			if id.Identifier != "" {
				tokens = append(tokens, makeToken(id.StartPosition(), id.EndPosition(nil), tokenTypeFunction, tokenModDeclaration))
			}

		case *ast.CompositeDeclaration:
			id := e.Identifier
			tokenType := compositeTokenType(e.Kind())
			tokens = append(tokens, makeToken(id.StartPosition(), id.EndPosition(nil), tokenType, tokenModDeclaration))

		case *ast.InterfaceDeclaration:
			id := e.Identifier
			tokens = append(tokens, makeToken(id.StartPosition(), id.EndPosition(nil), tokenTypeInterface, tokenModDeclaration))

		case *ast.EnumCaseDeclaration:
			id := e.Identifier
			tokens = append(tokens, makeToken(id.StartPosition(), id.EndPosition(nil), tokenTypeEnum, 0))

		case *ast.VariableDeclaration:
			id := e.Identifier
			mod := uint32(tokenModDeclaration)
			if e.IsConstant {
				mod |= tokenModReadonly
			}
			tokens = append(tokens, makeToken(id.StartPosition(), id.EndPosition(nil), tokenTypeVariable, mod))

		case *ast.FieldDeclaration:
			id := e.Identifier
			tokens = append(tokens, makeToken(id.StartPosition(), id.EndPosition(nil), tokenTypeProperty, tokenModDeclaration))

		case *ast.IntegerExpression:
			tokens = append(tokens, makeToken(e.StartPosition(), e.EndPosition(nil), tokenTypeNumber, 0))

		case *ast.FixedPointExpression:
			tokens = append(tokens, makeToken(e.StartPosition(), e.EndPosition(nil), tokenTypeNumber, 0))

		case *ast.StringExpression:
			tokens = append(tokens, makeToken(e.StartPosition(), e.EndPosition(nil), tokenTypeString, 0))

		case *ast.BoolExpression:
			tokens = append(tokens, makeToken(e.StartPosition(), e.EndPosition(nil), tokenTypeKeyword, 0))

		case *ast.NilExpression:
			tokens = append(tokens, makeToken(e.StartPosition(), e.EndPosition(nil), tokenTypeKeyword, 0))
		}

		return true
	})

	return tokens
}

func makeToken(startPos, endPos ast.Position, tokenType uint32, modifiers uint32) rawToken {
	protoStart := conversion.ASTToProtocolPosition(startPos)
	protoEnd := conversion.ASTToProtocolPosition(endPos.Shifted(nil, 1))
	length := uint32(0)
	if protoStart.Line == protoEnd.Line {
		length = protoEnd.Character - protoStart.Character
	} else {
		// Multi-line token: approximate with end column
		length = protoEnd.Character
	}
	if length == 0 {
		length = 1
	}
	return rawToken{
		line:      protoStart.Line,
		startChar: protoStart.Character,
		length:    length,
		tokenType: tokenType,
		modifiers: modifiers,
	}
}

func compositeTokenType(kind common.CompositeKind) uint32 {
	switch kind {
	case common.CompositeKindResource:
		return tokenTypeClass
	case common.CompositeKindStructure:
		return tokenTypeStruct
	case common.CompositeKindContract:
		return tokenTypeNamespace
	case common.CompositeKindEnum:
		return tokenTypeEnum
	case common.CompositeKindEvent:
		return tokenTypeEvent
	default:
		return tokenTypeType
	}
}
