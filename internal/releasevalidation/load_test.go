package releasevalidation

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLoadExecutesConfiguredWorkload(t *testing.T) {
	client := &fakeAnalyzerClient{}
	config := testConfig()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

	report, err := RunLoad(t.Context(), client, config, LoadOptions{Concurrency: 4, Requests: 40}, func() time.Time { return now })

	require.NoError(t, err)
	assert.Equal(t, int64(40), client.calls.Load())
	assert.Equal(t, 40, report.Summary.Total)
	assert.Equal(t, 40, report.Summary.Passed)
	assert.Zero(t, report.Summary.Failed)
	assert.Equal(t, map[string]int{"OK": 40}, report.Summary.Statuses)
	assert.Positive(t, report.Summary.ResponseBytes)
	assert.Positive(t, report.Summary.RequestsPerSecond)
	assert.NotEmpty(t, report.Summary.LatencyP50)
	assert.Len(t, report.Results, 40)
	for index, result := range report.Results {
		assert.Equal(t, index, result.Index)
	}
}

func TestRunLoadRejectsInvalidOptions(t *testing.T) {
	testCases := []struct {
		name    string
		options LoadOptions
		error   string
	}{
		{name: "zero concurrency", options: LoadOptions{Requests: 1}, error: "concurrency"},
		{name: "zero requests", options: LoadOptions{Concurrency: 1}, error: "requests"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := RunLoad(t.Context(), &fakeAnalyzerClient{}, testConfig(), testCase.options, time.Now)

			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.error)
		})
	}
}

func TestRunLoadRecordsFailuresAndContinues(t *testing.T) {
	client := &fakeAnalyzerClient{atrErr: errors.New("dependency failed")}

	report, err := RunLoad(t.Context(), client, testConfig(), LoadOptions{Concurrency: 2, Requests: 20}, time.Now)

	require.Error(t, err)
	assert.Equal(t, int64(20), client.calls.Load())
	assert.Equal(t, 19, report.Summary.Passed)
	assert.Equal(t, 1, report.Summary.Failed)
	assert.Equal(t, 1, report.Summary.Statuses["Unknown"])
	assert.Contains(t, report.Results[0].Error, "dependency failed")
}

func TestSummarizeLoadUsesNearestRankPercentiles(t *testing.T) {
	results := make([]LoadResult, 100)
	for index := range results {
		duration := time.Duration(index+1) * time.Millisecond
		results[index] = LoadResult{RPC: "GetATR", Status: "OK", ResponseBytes: 10, duration: duration}
	}

	summary := summarizeLoad(results, 2*time.Second)

	assert.Equal(t, "50ms", summary.LatencyP50)
	assert.Equal(t, "95ms", summary.LatencyP95)
	assert.Equal(t, "99ms", summary.LatencyP99)
	assert.Equal(t, "100ms", summary.LatencyMaximum)
	assert.Equal(t, 50.0, summary.RequestsPerSecond)
	assert.Equal(t, 1000, summary.ResponseBytes)
}
