package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type healthClientFunc func(*http.Request) (*http.Response, error)

func (f healthClientFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCheckHealthUsesConfiguredLocalAddress(t *testing.T) {
	getenv := func(name string) string {
		if name == "MARKET_ANALYZER_HTTP_ADDRESS" {
			return "0.0.0.0:8181"
		}
		return ""
	}
	client := healthClientFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "http://127.0.0.1:8181/health", request.URL.String())
		assert.Equal(t, http.MethodGet, request.Method)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})

	err := checkHealth(t.Context(), getenv, client)

	require.NoError(t, err)
}

func TestCheckHealthUsesDefaultAddress(t *testing.T) {
	client := healthClientFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "http://127.0.0.1:8081/health", request.URL.String())
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})

	err := checkHealth(t.Context(), func(string) string { return "" }, client)

	require.NoError(t, err)
}

func TestCheckHealthReturnsFailures(t *testing.T) {
	cases := []struct {
		name    string
		address string
		client  healthClient
		want    string
	}{
		{"invalid address", "invalid", healthClientFunc(nil), "parse HTTP address"},
		{"request failure", ":8081", healthClientFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("unavailable")
		}), "call health endpoint"},
		{"unhealthy status", ":8081", healthClientFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("not ready"))}, nil
		}), "status 503"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := checkHealth(t.Context(), func(string) string { return testCase.address }, testCase.client)

			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.want)
		})
	}
}

func TestHealthcheckRequested(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
		err  bool
	}{
		{"service", nil, false, false},
		{"healthcheck", []string{"-healthcheck"}, true, false},
		{"unknown", []string{"-unknown"}, false, true},
		{"extra", []string{"-healthcheck", "extra"}, false, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			requested, err := healthcheckRequested(testCase.args)

			assert.Equal(t, testCase.want, requested)
			if testCase.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCheckHealthHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := healthClientFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})

	err := checkHealth(ctx, func(string) string { return ":8081" }, client)

	require.ErrorIs(t, err, context.Canceled)
}
