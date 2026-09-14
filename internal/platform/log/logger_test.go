package log

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReplaceTimeFormat verifica que replaceTimeFormat reformata o atributo time e deixa os demais intactos.
func TestReplaceTimeFormat(t *testing.T) {
	t.Parallel()

	attr := ReplaceTimeFormat(nil, slog.String("time", "old"))
	require.Equal(t, "time", attr.Key)
	assert.NotEqual(t, "old", attr.Value.String())

	other := ReplaceTimeFormat(nil, slog.String("msg", "hello"))
	assert.Equal(t, "hello", other.Value.String())
}

// TestNewLogger verifica que newLogger escreve mensagens JSON com campo time.
func TestNewLogger(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelInfo)
	logger.Info("hello-logger")

	assert.Contains(t, buf.String(), "hello-logger")
	assert.Contains(t, buf.String(), `"time"`)
}
