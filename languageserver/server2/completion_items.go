package server2

import (
	"github.com/onflow/cadence-tools/languageserver/protocol"
)

var insertTextFormat = protocol.SnippetTextFormat

var statementCompletionItems = []*protocol.CompletionItem{
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "for",
		Detail:           "for-in loop",
		InsertText:       "for $1 in $2 {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "while",
		Detail:           "while loop",
		InsertText:       "while $1 {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "if",
		Detail:           "if statement",
		InsertText:       "if $1 {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "if else",
		Detail:           "if-else statement",
		InsertText:       "if $1 {\n\t$2\n} else {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "else",
		Detail:           "else block",
		InsertText:       "else {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "if let",
		Detail:           "if-let statement",
		InsertText:       "if let $1 = $2 {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "return",
		Detail:           "return statement",
		InsertText:       "return $0",
	},
	{
		Kind:   protocol.KeywordCompletion,
		Label:  "break",
		Detail: "break statement",
	},
	{
		Kind:   protocol.KeywordCompletion,
		Label:  "continue",
		Detail: "continue statement",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "emit",
		Detail:           "emit statement",
		InsertText:       "emit $0",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "destroy",
		Detail:           "destroy expression",
		InsertText:       "destroy $0",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "pre",
		Detail:           "pre conditions",
		InsertText:       "pre {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "post",
		Detail:           "post conditions",
		InsertText:       "post {\n\t$0\n}",
	},
}

var expressionCompletionItems = []*protocol.CompletionItem{
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "create",
		Detail:           "create statement",
		InsertText:       "create $0",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "let",
		Detail:           "constant declaration",
		InsertText:       "let $1 = $0",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "var",
		Detail:           "variable declaration",
		InsertText:       "var $1 = $0",
	},
}

var allAccessOptions = []string{"access(all)", "access(contract)", "access(account)", "access(self)"}
var allAccessOptionsCommaSeparated = "access(all),access(contract),access(account),access(self)"

// NOTE: if the document doesn't specify an access modifier yet,
// the completion item's InsertText will get prefixed with a placeholder
// for the access modifier.
//
// Start placeholders at index 2!
var declarationCompletionItems = []*protocol.CompletionItem{
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "struct",
		Detail:           "struct declaration",
		InsertText:       "struct $2 {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "resource",
		Detail:           "resource declaration",
		InsertText:       "resource $2 {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "contract",
		Detail:           "contract declaration",
		InsertText:       "contract $2 {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "struct interface",
		Detail:           "struct interface declaration",
		InsertText:       "struct interface $2 {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "resource interface",
		Detail:           "resource interface declaration",
		InsertText:       "resource interface $2 {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "contract interface",
		Detail:           "contract interface declaration",
		InsertText:       "contract interface $2 {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "event",
		Detail:           "event declaration",
		InsertText:       "event $2($0)",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "fun",
		Detail:           "function declaration",
		InsertText:       "fun $2($3)${4:: $5} {\n\t$0\n}",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "enum",
		Detail:           "enum declaration",
		InsertText:       "enum $2: $3 {\n\t$0\n}",
	},
}

// NOTE: if the document doesn't specify an access modifier yet,
// the completion item's InsertText will get prefixed with a placeholder
// for the access modifier.
//
// Start placeholders at index 2!
var containerCompletionItems = []*protocol.CompletionItem{
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "var",
		Detail:           "variable field",
		InsertText:       "var $2: $0",
	},
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "let",
		Detail:           "constant field",
		InsertText:       "let $2: $0",
	},
	// alias for the above
	{
		Kind:             protocol.KeywordCompletion,
		InsertTextFormat: &insertTextFormat,
		Label:            "const",
		Detail:           "constant field",
		InsertText:       "let $2: $0",
	},
}
