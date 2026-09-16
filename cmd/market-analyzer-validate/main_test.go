package main

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"

	"github.com/imbpp123/market-analyzer/internal/releasevalidation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFillBuildVersionsFromInfo(t *testing.T) {
	config := releasevalidation.Config{}
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}, {Key: "vcs.modified", Value: "true"}},
		Deps:     []*debug.Module{{Path: "github.com/imbpp123/market-data/api/go", Version: "v1.2.3"}},
	}

	fillBuildVersionsFromInfo(&config, info)

	assert.Equal(t, "abc123+dirty", config.BuildRevision)
	assert.Equal(t, "v1.2.3", config.UpstreamContractVersion)
}

func TestFillBuildVersionsPreservesExplicitValues(t *testing.T) {
	config := releasevalidation.Config{BuildRevision: "release", UpstreamContractVersion: "contract"}
	info := &debug.BuildInfo{
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}},
		Deps:     []*debug.Module{{Path: "github.com/imbpp123/market-data/api/go", Version: "v1.2.3"}},
	}

	fillBuildVersionsFromInfo(&config, info)

	assert.Equal(t, "release", config.BuildRevision)
	assert.Equal(t, "contract", config.UpstreamContractVersion)
}

func TestWriteReportCreatesPrivateFileWithoutOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	report := releasevalidation.Report{SchemaVersion: "test"}

	err := writeReport(path, report)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"schema_version":"test","started_at":"0001-01-01T00:00:00Z","completed_at":"0001-01-01T00:00:00Z","configuration":{"analyzer_endpoint":"","timeout":"","max_response_bytes":0,"build_revision":"","upstream_contract_version":"","selection":{"exchange":"","market":"","symbol":"","to":"0001-01-01T00:00:00Z","candle_count":0,"interval":""},"settings":{"atr_period":0,"pivot_span":0,"reversal_pct":"","reversal_atr_period":0,"reversal_atr_multiplier":"","equality_tolerance_pct":"","level_atr_period":0,"zone_width_atr":"","min_touches":0,"min_touch_separation_bars":0},"runtime_settings":{"request_timeout":"","max_request_bytes":0,"market_data_max_response_bytes":0,"max_response_bytes":0}},"checks":null,"summary":{"total":0,"passed":0,"failed":0,"response_bytes":0}}`, string(data))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	err = writeReport(path, report)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create validation report")
}
