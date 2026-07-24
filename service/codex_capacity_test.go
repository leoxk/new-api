package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCodexCapacityTestDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(
		&model.CodexCapacitySnapshot{},
		&model.CodexResetCredit{},
		&model.CodexResetOperation{},
	))
	t.Cleanup(func() { model.DB = previousDB })
}

func TestLoadCodexOAuthInstancesRejectsDuplicateAndInvalidIDs(t *testing.T) {
	t.Setenv("CODEX_OAUTH_INSTANCES", `[
  {"id":"primary","base_url":"http://oauth-one:10531","auth_file":"/auth/one.json"},
  {"id":"primary","base_url":"http://oauth-two:10531","auth_file":"/auth/two.json"}
]`)
	_, err := LoadCodexOAuthInstances()
	require.Error(t, err)

	t.Setenv("CODEX_OAUTH_INSTANCES", `[{"id":"not valid","base_url":"http://oauth:10531","auth_file":"/auth/state.json"}]`)
	_, err = LoadCodexOAuthInstances()
	require.Error(t, err)
}

func TestGetCodexCapacityReturnsConfiguredInstancesAndDatabaseSnapshots(t *testing.T) {
	setupCodexCapacityTestDB(t)
	t.Setenv("CODEX_OAUTH_INSTANCES", `[
  {"id":"primary","display_name":"Primary","base_url":"http://oauth-one:10531","auth_file":"/auth/one.json"},
  {"id":"secondary","display_name":"Secondary","base_url":"http://oauth-two:10531","auth_file":"/auth/two.json"}
]`)
	checkedAt := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC).Unix()
	require.NoError(t, model.UpsertCodexCapacitySnapshot(&model.CodexCapacitySnapshot{
		InstanceID:          "primary",
		DisplayName:         "Primary",
		Allowed:             true,
		FiveHourUsedPercent: 20.5,
		FiveHourResetAt:     checkedAt + 3600,
		SevenDayUsedPercent: 75,
		SevenDayResetAt:     checkedAt + 86400,
		CheckedAt:           checkedAt,
	}, []*model.CodexResetCredit{{
		InstanceID: "primary", CreditID: "credit-1", Status: "available", ResetType: "rate_limit",
	}}))

	capacity, err := GetCodexCapacity(time.Unix(checkedAt+30, 0))
	require.NoError(t, err)
	require.Len(t, capacity.Instances, 2)
	assert.Equal(t, "primary", capacity.Instances[0].ID)
	assert.False(t, capacity.Instances[0].Stale)
	assert.Equal(t, 79.5, capacity.Instances[0].FiveHour.RemainingPercent)
	assert.Equal(t, 1, capacity.Instances[0].AvailableResetCount)
	assert.Equal(t, "secondary", capacity.Instances[1].ID)
	assert.True(t, capacity.Instances[1].Stale)
}

func TestGetCodexCapacityMarksOldSnapshotStale(t *testing.T) {
	setupCodexCapacityTestDB(t)
	t.Setenv("CODEX_OAUTH_INSTANCES", `[{"id":"primary","base_url":"http://oauth:10531","auth_file":"/auth/state.json"}]`)
	checkedAt := time.Now().UTC().Add(-codexCapacityStaleAfter - time.Second).Unix()
	require.NoError(t, model.UpsertCodexCapacitySnapshot(&model.CodexCapacitySnapshot{InstanceID: "primary", CheckedAt: checkedAt}, nil))
	capacity, err := GetCodexCapacity(time.Now().UTC())
	require.NoError(t, err)
	assert.True(t, capacity.Instances[0].Stale)
}

func TestGetCodexCapacityMarksRefreshFailureStaleWithoutReplacingLastSuccess(t *testing.T) {
	setupCodexCapacityTestDB(t)
	t.Setenv("CODEX_OAUTH_INSTANCES", `[{"id":"primary","base_url":"http://oauth:10531","auth_file":"/auth/state.json"}]`)
	checkedAt := time.Now().UTC().Unix()
	require.NoError(t, model.UpsertCodexCapacitySnapshot(&model.CodexCapacitySnapshot{
		InstanceID: "primary", CheckedAt: checkedAt,
	}, nil))
	require.NoError(t, model.SetCodexCapacityError("primary", "refresh failed"))

	capacity, err := GetCodexCapacity(time.Unix(checkedAt+1, 0))
	require.NoError(t, err)
	assert.True(t, capacity.Instances[0].Stale)
	assert.Equal(t, time.Unix(checkedAt, 0).UTC().Format(time.RFC3339), capacity.Instances[0].CheckedAt)
}
