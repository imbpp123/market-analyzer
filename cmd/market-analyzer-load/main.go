package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	marketanalyzerv1 "github.com/imbpp123/market-analyzer/api/go/marketanalyzer/v1"
	"github.com/imbpp123/market-analyzer/internal/releasevalidation"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to the validation JSON configuration")
	outputPath := flag.String("output", "", "path for the JSON report")
	metricsURL := flag.String("metrics-url", "", "Analyzer metrics URL")
	concurrency := flag.Int("concurrency", 8, "number of concurrent callers")
	requests := flag.Int("requests", 200, "total request count")
	flag.Parse()
	if *configPath == "" {
		return errors.New("-config is required")
	}
	if *outputPath == "" {
		return errors.New("-output is required")
	}
	if *metricsURL == "" {
		return errors.New("-metrics-url is required")
	}

	configFile, err := os.Open(*configPath)
	if err != nil {
		return fmt.Errorf("open validation config: %w", err)
	}
	config, parseErr := releasevalidation.ParseConfig(configFile)
	closeErr := configFile.Close()
	if parseErr != nil {
		return parseErr
	}
	if closeErr != nil {
		return fmt.Errorf("close validation config: %w", closeErr)
	}
	fillBuildVersions(&config)
	if config.BuildRevision == "" {
		return errors.New("build revision is unavailable; set build_revision in the validation config")
	}
	if config.UpstreamContractVersion == "" {
		return errors.New("upstream contract version is unavailable; set upstream_contract_version in the validation config")
	}

	ctx := context.Background()
	before, err := fetchMetrics(ctx, *metricsURL)
	if err != nil {
		return err
	}
	connection, err := grpcgo.NewClient(config.AnalyzerEndpoint,
		grpcgo.WithTransportCredentials(insecure.NewCredentials()),
		grpcgo.WithDefaultCallOptions(grpcgo.MaxCallRecvMsgSize(config.MaxResponseBytes)))
	if err != nil {
		return fmt.Errorf("create Analyzer client: %w", err)
	}
	defer func() { _ = connection.Close() }()

	report, loadErr := releasevalidation.RunLoad(ctx, marketanalyzerv1.NewMarketAnalyzerServiceClient(connection), config,
		releasevalidation.LoadOptions{Concurrency: *concurrency, Requests: *requests}, time.Now)
	after, metricsErr := fetchMetrics(ctx, *metricsURL)
	if metricsErr == nil {
		delta := releasevalidation.DifferenceMetrics(before, after)
		report.ServiceMetrics = &delta
	}
	if err := writeReport(*outputPath, report); err != nil {
		return err
	}
	if loadErr != nil {
		return loadErr
	}
	return metricsErr
}

func fetchMetrics(ctx context.Context, url string) (releasevalidation.MetricsSnapshot, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return releasevalidation.MetricsSnapshot{}, fmt.Errorf("create metrics request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return releasevalidation.MetricsSnapshot{}, fmt.Errorf("get metrics: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return releasevalidation.MetricsSnapshot{}, fmt.Errorf("get metrics: unexpected HTTP status %s", response.Status)
	}
	metrics, err := releasevalidation.ParseMetrics(response.Body)
	if err != nil {
		return releasevalidation.MetricsSnapshot{}, err
	}
	return metrics, nil
}

func writeReport(path string, report releasevalidation.LoadReport) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create load report: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		_ = file.Close()
		return fmt.Errorf("write load report: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close load report: %w", err)
	}
	return nil
}

func fillBuildVersions(config *releasevalidation.Config) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if config.BuildRevision == "" {
		var revision string
		modified := false
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
		config.BuildRevision = revision
		if modified && revision != "" && !strings.HasSuffix(revision, "+dirty") {
			config.BuildRevision += "+dirty"
		}
	}
	if config.UpstreamContractVersion == "" {
		for _, dependency := range info.Deps {
			if dependency.Path == "github.com/imbpp123/market-data/api/go" {
				config.UpstreamContractVersion = dependency.Version
				break
			}
		}
	}
}
