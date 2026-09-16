package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imbpp123/market-analyzer/internal/releasevalidation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchMetrics(t *testing.T) {
	previousClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(
			"market_analyzer_active_requests 2\n")), Request: request}, nil
	})}
	t.Cleanup(func() { http.DefaultClient = previousClient })

	metrics, err := fetchMetrics(t.Context(), "http://metrics.test")

	require.NoError(t, err)
	assert.Equal(t, 2.0, metrics.ActiveRequests)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestWriteReportCreatesPrivateFileWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "load.json")
	report := releasevalidation.LoadReport{SchemaVersion: "test"}

	err := writeReport(path, report)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	err = writeReport(path, report)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create load report")
}
