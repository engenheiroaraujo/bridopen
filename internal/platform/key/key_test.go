package key

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateTokenKey verifica que GenerateTokenKey gera tokens não vazios e distintos a cada chamada.
func TestGenerateTokenKey(t *testing.T) {
	t.Parallel()

	a, err := GenerateTokenKey()
	require.NoError(t, err)
	b, err := GenerateTokenKey()
	require.NoError(t, err)
	assert.NotEmpty(t, a)
	assert.NotEqual(t, a, b)
}

// TestHashTokenDeterministicAndTrims verifica que HashToken é determinístico, ignora espaços e retorna hash de 64 caracteres.
func TestHashTokenDeterministicAndTrims(t *testing.T) {
	t.Parallel()

	assert.Equal(t, HashToken("abc"), HashToken("  abc  "))
	assert.NotEqual(t, HashToken("abc"), HashToken("abcd"))
	assert.Len(t, HashToken("abc"), 64)
}

// TestGenerateSessionIDFormat verifica que GenerateSessionID produz um identificador no formato UUID.
func TestGenerateSessionIDFormat(t *testing.T) {
	t.Parallel()

	id, err := GenerateSessionID()
	require.NoError(t, err)
	parts := strings.Split(id, "-")
	require.Len(t, parts, 5)
	assert.Len(t, parts[0], 8)
	assert.Len(t, parts[1], 4)
	assert.Len(t, parts[2], 4)
	assert.Len(t, parts[3], 4)
	assert.Len(t, parts[4], 12)
}

// TestClientIP verifica a resolução do IP do cliente via X-Forwarded-For, X-Real-IP e RemoteAddr.
func TestClientIP(t *testing.T) {
	t.Parallel()

	t.Run("forwarded_for", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.1")
		req.RemoteAddr = "127.0.0.1:1234"
		assert.Equal(t, "203.0.113.10", ClientIP(req))
	})

	t.Run("invalid_forwarded_uses_real_ip", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-For", "not-an-ip")
		req.Header.Set("X-Real-IP", "198.51.100.20")
		req.RemoteAddr = "127.0.0.1:1234"
		assert.Equal(t, "198.51.100.20", ClientIP(req))
	})

	t.Run("invalid_real_ip_uses_remote_addr", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Real-IP", "bad")
		req.RemoteAddr = "192.0.2.1:8080"
		assert.Equal(t, "192.0.2.1", ClientIP(req))
	})

	t.Run("remote_addr", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.1:8080"
		assert.Equal(t, "192.0.2.1", ClientIP(req))
	})

	t.Run("remote_addr_without_port", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.9"
		assert.Equal(t, "192.0.2.9", ClientIP(req))
	})
}
