package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

func TestGetAIUsageBuildsPublicResponse(t *testing.T) {
	resetAIUsageCache(t)
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	mainDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	mainSQL, err := mainDB.DB()
	require.NoError(t, err)
	mainSQL.SetMaxOpenConns(1)
	logSQL, err := logDB.DB()
	require.NoError(t, err)
	logSQL.SetMaxOpenConns(1)
	model.DB = mainDB
	model.LOG_DB = logDB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}))
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	t.Cleanup(func() {
		_ = mainSQL.Close()
		_ = logSQL.Close()
		model.DB = previousDB
		model.LOG_DB = previousLogDB
	})

	user := model.User{Username: "leo", Password: "test-password", Status: 1, Role: 1}
	require.NoError(t, model.DB.Create(&user).Error)
	capacityPath := filepath.Join(t.TempDir(), "last-good.json")
	require.NoError(t, os.WriteFile(capacityPath, []byte(`{
		"fiveHour":{"usedPercent":20,"resetAt":2000000100},
		"sevenDay":{"usedPercent":45,"resetAt":2000600000}
	}`), 0o600))
	t.Setenv("AI_USAGE_CAPACITY_PATH", capacityPath)
	now := time.Unix(2_000_000_000, 0).UTC()

	response, err := GetAIUsage("leo", now)
	require.NoError(t, err)
	assert.Equal(t, 3, response.SchemaVersion)
	assert.Equal(t, AIUsageUser{UserID: user.Id, Username: "leo"}, response.User)
	assert.Equal(t, now.Format(time.RFC3339Nano), response.GeneratedAt)
	assert.Equal(t, now.Format(time.RFC3339Nano), response.HistoryStartedAt)
	assert.False(t, response.Stale)
	assert.NotNil(t, response.Keys)
	assert.Empty(t, response.Keys)
	require.NotNil(t, response.CodexCapacity.FiveHour)
	assert.Equal(t, float64(80), response.CodexCapacity.FiveHour.RemainingPercent)
	assert.Equal(t, time.Unix(2_000_000_100, 0).UTC().Format(time.RFC3339Nano), response.CodexCapacity.FiveHour.ResetAt)
	require.NotNil(t, response.CodexCapacity.SevenDay)
	assert.Equal(t, float64(55), response.CodexCapacity.SevenDay.RemainingPercent)
}

func TestGetAIUsageCachesSuccessfulResponseForFiveMinutes(t *testing.T) {
	resetAIUsageCache(t)
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	mainDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	mainSQL, err := mainDB.DB()
	require.NoError(t, err)
	mainSQL.SetMaxOpenConns(1)
	logSQL, err := logDB.DB()
	require.NoError(t, err)
	logSQL.SetMaxOpenConns(1)
	model.DB = mainDB
	model.LOG_DB = logDB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}))
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	t.Cleanup(func() {
		_ = mainSQL.Close()
		_ = logSQL.Close()
		model.DB = previousDB
		model.LOG_DB = previousLogDB
	})

	user := model.User{Username: "leo", Password: "test-password", Status: 1, Role: 1}
	require.NoError(t, model.DB.Create(&user).Error)
	firstKey := model.Token{UserId: user.Id, Key: "first-key", Name: "first"}
	require.NoError(t, model.DB.Create(&firstKey).Error)
	now := time.Unix(2_000_000_000, 0).UTC()
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId: user.Id, CreatedAt: now.Unix(), Type: model.LogTypeConsume,
		TokenId: firstKey.Id, PromptTokens: 10, CompletionTokens: 5,
	}).Error)

	first, err := GetAIUsage("leo", now)
	require.NoError(t, err)
	require.Len(t, first.Keys, 1)
	assert.Equal(t, int64(15), first.Keys[0].Tokens1D)

	secondKey := model.Token{UserId: user.Id, Key: "second-key", Name: "second"}
	require.NoError(t, model.DB.Create(&secondKey).Error)
	require.NoError(t, model.LOG_DB.Create(&model.Log{
		UserId: user.Id, CreatedAt: now.Add(time.Minute).Unix(), Type: model.LogTypeConsume,
		TokenId: secondKey.Id, PromptTokens: 20, CompletionTokens: 10,
	}).Error)

	cached, err := GetAIUsage("leo", now.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, first.GeneratedAt, cached.GeneratedAt)
	assert.Equal(t, first.Keys, cached.Keys)

	refreshedAt := now.Add(aiUsageCacheTTL + time.Second)
	refreshed, err := GetAIUsage("leo", refreshedAt)
	require.NoError(t, err)
	assert.Equal(t, refreshedAt.Format(time.RFC3339Nano), refreshed.GeneratedAt)
	require.Len(t, refreshed.Keys, 2)
	assert.Equal(t, int64(30), refreshed.Keys[1].Tokens1D)
}

func resetAIUsageCache(t *testing.T) {
	t.Helper()
	aiUsageCache.Range(func(key, _ any) bool {
		aiUsageCache.Delete(key)
		return true
	})
	aiUsageRefreshGroup = singleflight.Group{}
	t.Cleanup(func() {
		aiUsageCache.Range(func(key, _ any) bool {
			aiUsageCache.Delete(key)
			return true
		})
		aiUsageRefreshGroup = singleflight.Group{}
	})
}
