package server2

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeSemanticTokensDeltaEncoding(t *testing.T) {
	tokens := []rawToken{
		{line: 0, startChar: 5, length: 3, tokenType: 1, modifiers: 0},
		{line: 0, startChar: 10, length: 4, tokenType: 2, modifiers: 1},
		{line: 2, startChar: 3, length: 5, tokenType: 0, modifiers: 0},
	}
	data := encodeSemanticTokens(tokens)
	require.Len(t, data, 15) // 3 tokens * 5 values each
	// First token: deltaLine=0, deltaChar=5, length=3, type=1, mod=0
	assert.Equal(t, []uint32{0, 5, 3, 1, 0}, data[0:5])
	// Second token: deltaLine=0, deltaChar=5 (10-5), length=4, type=2, mod=1
	assert.Equal(t, []uint32{0, 5, 4, 2, 1}, data[5:10])
	// Third token: deltaLine=2, deltaChar=3 (new line), length=5, type=0, mod=0
	assert.Equal(t, []uint32{2, 3, 5, 0, 0}, data[10:15])
}
