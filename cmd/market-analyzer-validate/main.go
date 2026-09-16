package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	outputPath := flag.String("output", "", "path for the JSON report; stdout when empty")
	flag.Parse()
	if *configPath == "" {
		return errors.New("-config is required")
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

	connection, err := grpcgo.NewClient(config.AnalyzerEndpoint,
		grpcgo.WithTransportCredentials(insecure.NewCredentials()),
		grpcgo.WithDefaultCallOptions(grpcgo.MaxCallRecvMsgSize(config.MaxResponseBytes)))
	if err != nil {
		return fmt.Errorf("create Analyzer client: %w", err)
	}
	defer func() { _ = connection.Close() }()

	report, validationErr := releasevalidation.Run(context.Background(), marketanalyzerv1.NewMarketAnalyzerServiceClient(connection), config, time.Now)
	if err := writeReport(*outputPath, report); err != nil {
		return err
	}
	return validationErr
}

func writeReport(path string, report releasevalidation.Report) error {
	var writer io.Writer = os.Stdout
	var file *os.File
	if path != "" {
		var err error
		file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("create validation report: %w", err)
		}
		writer = file
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		if file != nil {
			_ = file.Close()
		}
		return fmt.Errorf("write validation report: %w", err)
	}
	if file != nil {
		if err := file.Close(); err != nil {
			return fmt.Errorf("close validation report: %w", err)
		}
	}

	return nil
}

func fillBuildVersions(config *releasevalidation.Config) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	fillBuildVersionsFromInfo(config, info)
}

func fillBuildVersionsFromInfo(config *releasevalidation.Config, info *debug.BuildInfo) {
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
