package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCodexCapacityReadsTheMonitorStateWithoutExposingOAuthData(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "last-good.json")
	require.NoError(t, os.WriteFile(statePath, []byte(`{
  "allowed": true,
  "fiveHour": {"usedPercent": 20.5, "resetAt": 1784808000},
  "sevenDay": {"usedPercent": 75, "resetAt": 1785400000},
  "checkedAt": 1784799000
}`), 0o600))
	t.Setenv("CODEX_CAPACITY_STATE_PATH", statePath)

	capacity, err := GetCodexCapacity(time.Unix(1784799300, 0))
	require.NoError(t, err)
	assert.True(t, capacity.Allowed)
	assert.False(t, capacity.Stale)
	assert.Equal(t, "2026-07-23T09:30:00Z", capacity.CheckedAt)
	require.NotNil(t, capacity.FiveHour)
	assert.Equal(t, 79.5, capacity.FiveHour.RemainingPercent)
	assert.Equal(t, "2026-07-23T12:00:00Z", capacity.FiveHour.ResetAt)
	require.NotNil(t, capacity.SevenDay)
	assert.Equal(t, 25.0, capacity.SevenDay.RemainingPercent)
}

func TestGetCodexCapacityMarksAnOldMonitorStateAsStale(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "last-good.json")
	require.NoError(t, os.WriteFile(statePath, []byte(`{
  "allowed": true,
  "fiveHour": {"usedPercent": 10, "resetAt": 1784808000},
  "checkedAt": 1784799000
}`), 0o600))
	t.Setenv("CODEX_CAPACITY_STATE_PATH", statePath)

	capacity, err := GetCodexCapacity(time.Unix(1784799000, 0).Add(codexCapacityStaleAfter + time.Second))
	require.NoError(t, err)
	assert.True(t, capacity.Stale)
	assert.Nil(t, capacity.SevenDay)
}
