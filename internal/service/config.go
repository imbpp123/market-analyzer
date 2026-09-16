package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/imbpp123/market-analyzer/internal/application"
	marketdataadapter "github.com/imbpp123/market-analyzer/internal/infrastructure/marketdata"
	grpctransport "github.com/imbpp123/market-analyzer/internal/transport/grpc"
)

const DefaultMaxRequestBytes = 64 << 10

type Config struct {
	GRPCAddress                string
	HTTPAddress                string
	MarketDataEndpoint         string
	RequestTimeout             time.Duration
	ShutdownTimeout            time.Duration
	MaxRequestBytes            int
	MarketDataMaxResponseBytes int
	MaxResponseBytes           int
}

func DefaultConfig() Config {
	return Config{
		GRPCAddress:                ":9091",
		HTTPAddress:                ":8081",
		RequestTimeout:             application.DefaultRequestTimeout,
		ShutdownTimeout:            30 * time.Second,
		MaxRequestBytes:            DefaultMaxRequestBytes,
		MarketDataMaxResponseBytes: marketdataadapter.DefaultMaxResponseBytes,
		MaxResponseBytes:           grpctransport.DefaultMaxResponseBytes,
	}
}

func ConfigFromEnv(getenv func(string) string) (Config, error) {
	config := DefaultConfig()
	config.GRPCAddress = stringValue(getenv, "MARKET_ANALYZER_GRPC_ADDRESS", config.GRPCAddress)
	config.HTTPAddress = stringValue(getenv, "MARKET_ANALYZER_HTTP_ADDRESS", config.HTTPAddress)
	config.MarketDataEndpoint = strings.TrimSpace(getenv("MARKET_DATA_ENDPOINT"))

	var err error
	if config.RequestTimeout, err = durationValue(getenv, "MARKET_ANALYZER_REQUEST_TIMEOUT", config.RequestTimeout); err != nil {
		return Config{}, err
	}
	if config.ShutdownTimeout, err = durationValue(getenv, "MARKET_ANALYZER_SHUTDOWN_TIMEOUT", config.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if config.MaxRequestBytes, err = intValue(getenv, "MARKET_ANALYZER_MAX_REQUEST_BYTES", config.MaxRequestBytes); err != nil {
		return Config{}, err
	}
	if config.MarketDataMaxResponseBytes, err = intValue(getenv, "MARKET_DATA_MAX_RESPONSE_BYTES", config.MarketDataMaxResponseBytes); err != nil {
		return Config{}, err
	}
	if config.MaxResponseBytes, err = intValue(getenv, "MARKET_ANALYZER_MAX_RESPONSE_BYTES", config.MaxResponseBytes); err != nil {
		return Config{}, err
	}

	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	switch {
	case strings.TrimSpace(c.GRPCAddress) == "":
		return errors.New("gRPC address is required")
	case strings.TrimSpace(c.HTTPAddress) == "":
		return errors.New("HTTP address is required")
	case strings.TrimSpace(c.MarketDataEndpoint) == "":
		return errors.New("market data endpoint is required")
	case c.RequestTimeout <= 0:
		return errors.New("request timeout must be positive")
	case c.ShutdownTimeout <= 0:
		return errors.New("shutdown timeout must be positive")
	case c.MaxRequestBytes <= 0:
		return errors.New("max request bytes must be positive")
	case c.MarketDataMaxResponseBytes <= 0:
		return errors.New("market data max response bytes must be positive")
	case c.MaxResponseBytes <= 0:
		return errors.New("max response bytes must be positive")
	}
	return nil
}

func stringValue(getenv func(string) string, name, fallback string) string {
	if value := strings.TrimSpace(getenv(name)); value != "" {
		return value
	}
	return fallback
}

func durationValue(getenv func(string) string, name string, fallback time.Duration) (time.Duration, error) {
	text := strings.TrimSpace(getenv(name))
	if text == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(text)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return value, nil
}

func intValue(getenv func(string) string, name string, fallback int) (int, error) {
	text := strings.TrimSpace(getenv(name))
	if text == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}
