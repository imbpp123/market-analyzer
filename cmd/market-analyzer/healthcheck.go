package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

const defaultHTTPAddress = ":8081"

type healthClient interface {
	Do(*http.Request) (*http.Response, error)
}

func checkHealth(ctx context.Context, getenv func(string) string, client healthClient) error {
	address := strings.TrimSpace(getenv("MARKET_ANALYZER_HTTP_ADDRESS"))
	if address == "" {
		address = defaultHTTPAddress
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("parse HTTP address: %w", err)
	}
	if host == "" || isUnspecifiedIP(host) {
		host = "127.0.0.1"
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/health", nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("call health endpoint: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned status %d", response.StatusCode)
	}

	return nil
}

func isUnspecifiedIP(host string) bool {
	address := net.ParseIP(host)
	return address != nil && address.IsUnspecified()
}

func healthcheckRequested(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "-healthcheck" {
		return true, nil
	}
	return false, errors.New("usage: market-analyzer [-healthcheck]")
}
