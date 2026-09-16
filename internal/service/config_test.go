package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFromEnvUsesDefaultsAndOverrides(t *testing.T) {
	values := map[string]string{
		"MARKET_DATA_ENDPOINT":               "localhost:9090",
		"MARKET_ANALYZER_GRPC_ADDRESS":       "127.0.0.1:9001",
		"MARKET_ANALYZER_REQUEST_TIMEOUT":    "12s",
		"MARKET_ANALYZER_MAX_RESPONSE_BYTES": "2048",
		"MARKET_DATA_MAX_RESPONSE_BYTES":     "1024",
	}

	config, err := ConfigFromEnv(func(name string) string { return values[name] })

	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:9001", config.GRPCAddress)
	assert.Equal(t, ":8081", config.HTTPAddress)
	assert.Equal(t, 12*time.Second, config.RequestTimeout)
	assert.Equal(t, 2048, config.MaxResponseBytes)
	assert.Equal(t, 1024, config.MarketDataMaxResponseBytes)
}

func TestConfigFromEnvRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name   string
		values map[string]string
	}{
		{"missing endpoint", nil},
		{"invalid duration", map[string]string{"MARKET_DATA_ENDPOINT": "localhost:9090", "MARKET_ANALYZER_REQUEST_TIMEOUT": "later"}},
		{"zero duration", map[string]string{"MARKET_DATA_ENDPOINT": "localhost:9090", "MARKET_ANALYZER_SHUTDOWN_TIMEOUT": "0s"}},
		{"invalid size", map[string]string{"MARKET_DATA_ENDPOINT": "localhost:9090", "MARKET_ANALYZER_MAX_REQUEST_BYTES": "-1"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ConfigFromEnv(func(name string) string { return testCase.values[name] })
			assert.Error(t, err)
		})
	}
}
