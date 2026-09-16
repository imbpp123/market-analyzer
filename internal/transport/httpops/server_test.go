package httpops

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOperationalEndpoints(t *testing.T) {
	ready := false
	handler := NewHandler(func() bool { return ready }, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusOK)
	}))

	assert.Equal(t, http.StatusOK, request(t, handler, "/health").Code)
	assert.Equal(t, http.StatusServiceUnavailable, request(t, handler, "/ready").Code)
	ready = true
	assert.Equal(t, http.StatusOK, request(t, handler, "/ready").Code)
	assert.Equal(t, http.StatusOK, request(t, handler, "/metrics").Code)
	assert.Equal(t, http.StatusNotFound, request(t, handler, "/analysis").Code)
}

func request(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil).WithContext(t.Context()))
	return response
}
