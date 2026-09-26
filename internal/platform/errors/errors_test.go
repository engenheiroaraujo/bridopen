package errors

import (
	stderrors "errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRepositoryErrorUnwrap verifica que RepositoryError envolve e permite unwrap do erro raiz.
func TestRepositoryErrorUnwrap(t *testing.T) {
	t.Parallel()

	root := stderrors.New("db down")
	err := NewRepositoryError(root)

	var repoErr *RepositoryError
	require.ErrorAs(t, err, &repoErr)
	assert.ErrorIs(t, err, root)
	assert.Equal(t, root, repoErr.Unwrap())
}

// TestNewRepositoryErrorNil verifica que erro nil não vira um RepositoryError
// envolvendo nil — o chamador pode repassar o resultado sem checagem extra.
func TestNewRepositoryErrorNil(t *testing.T) {
	t.Parallel()

	assert.NoError(t, NewRepositoryError(nil))
	assert.Nil(t, NewRepositoryError(nil))
}

// TestSentinelErrorsDistinct verifica que os erros sentinela do pacote são distintos entre si.
func TestSentinelErrorsDistinct(t *testing.T) {
	t.Parallel()

	assert.NotErrorIs(t, ErrNotFound, ErrDuplicate)
	assert.NotErrorIs(t, ErrUserInactive, ErrInvalidTokenOrUserAlreadyConfirmed)
}

// TestRepositoryErrorNilUnwrap verifica que Unwrap em RepositoryError nil retorna nil com segurança.
func TestRepositoryErrorNilUnwrap(t *testing.T) {
	t.Parallel()

	var repoErr *RepositoryError
	assert.Nil(t, repoErr.Unwrap())
}

// TestWithStatusPreservesMessageAndCode verifica que WithStatus preserva a mensagem original e o status HTTP informado.
func TestWithStatusPreservesMessageAndCode(t *testing.T) {
	t.Parallel()

	err := WithStatus(stderrors.New("falhou"), http.StatusBadRequest)
	statusErr, ok := stderrors.AsType[StatusError](err)
	require.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, statusErr.StatusCode())
	assert.Equal(t, "falhou", statusErr.Error())
}

// TestHTTPStatusSentinels verifica os status HTTP dos erros sentinela de interface.
func TestHTTPStatusSentinels(t *testing.T) {
	t.Parallel()

	notFound, ok := stderrors.AsType[StatusError](ErrPageNotFound)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, notFound.StatusCode())

	internal, ok := stderrors.AsType[StatusError](ErrInternal)
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, internal.StatusCode())
}
