package buildinfo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerReturnsBuildMetadata(t *testing.T) {
	originalVersion, originalCommit, originalBuildTime := Version, Commit, BuildTime
	t.Cleanup(func() {
		Version, Commit, BuildTime = originalVersion, originalCommit, originalBuildTime
	})
	Version, Commit, BuildTime = "v1.2.3", "abc1234", "2026-08-07T20:00:00Z"

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/version", nil)
	Handler(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))

	var response Info
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	assert.Equal(t, Current(), response)
}
