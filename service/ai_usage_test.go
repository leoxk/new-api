package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

func TestGetAIUsageBuildsV5CostDistributionInHongKongTime(t *testing.T) {
	resetAIUsageCache(t)
	mainDB, logDB := setupAIUsageDatabases(t)
	user := model.User{Username: "babypro", Password: "test-password", Status: 1, Role: 1, Quota: 9_000}
	require.NoError(t, mainDB.Create(&user).Error)
	firstKey := model.Token{UserId: user.Id, Key: "first-key", Name: "BP - Alice", RemainQuota: 190}
	secondKey := model.Token{UserId: user.Id, Key: "second-key", Name: "BP - Bob", RemainQuota: 400}
	require.NoError(t, mainDB.Create(&firstKey).Error)
	require.NoError(t, mainDB.Create(&secondKey).Error)

	location, err := time.LoadLocation(aiUsageTimezone)
	require.NoError(t, err)
	now := time.Date(2026, time.July, 23, 10, 30, 0, 0, location)
	weekStartedAt := time.Date(2026, time.July, 20, 0, 0, 0, 0, location)
	todayStartedAt := time.Date(2026, time.July, 23, 0, 0, 0, 0, location)
	for _, log := range []model.Log{
		{UserId: user.Id, CreatedAt: weekStartedAt.Add(time.Hour).Unix(), Type: model.LogTypeConsume, TokenId: firstKey.Id, ModelName: "gpt-5.6-sol", PromptTokens: 80, CompletionTokens: 20, Quota: 680},
		{UserId: user.Id, CreatedAt: todayStartedAt.Add(time.Hour).Unix(), Type: model.LogTypeConsume, TokenId: firstKey.Id, ModelName: "gpt-5.6-terra", PromptTokens: 800, CompletionTokens: 100, Quota: 100},
		{UserId: user.Id, CreatedAt: todayStartedAt.Add(2 * time.Hour).Unix(), Type: model.LogTypeConsume, TokenId: secondKey.Id, ModelName: "gpt-5.6-terra", PromptTokens: 100, CompletionTokens: 0, Quota: 20},
	} {
		require.NoError(t, logDB.Create(&log).Error)
	}

	response, err := GetAIUsage("babypro", now)
	require.NoError(t, err)
	assert.Equal(t, 5, response.SchemaVersion)
	assert.Equal(t, AIUsageCompany{Username: "babypro", Timezone: aiUsageTimezone}, response.Company)
	assert.Equal(t, now.Format(time.RFC3339Nano), response.GeneratedAt)
	assert.Equal(t, todayStartedAt.Format(time.RFC3339Nano), response.Period.TodayStartedAt)
	assert.Equal(t, weekStartedAt.Format(time.RFC3339Nano), response.Period.WeekStartedAt)
	assert.Equal(t, AIUsageSummary{
		CostUSDToday:              "0.00",
		CostUSDThisWeek:           "0.00",
		CompanyWalletRemainingUSD: "0.02",
		KeyCount:                  2,
	}, response.Summary)
	require.Len(t, response.TopCostModels, 2)
	assert.Equal(t, AIUsageModel{
		ModelName: "gpt-5.6-sol", Tokens: 100, TokenPercentage: 9.09, CostUSD: "0.00", ChargedQuota: 680, CostPercentage: 85, RequestCount: 1,
	}, response.TopCostModels[0])
	assert.Equal(t, AIUsageModel{
		ModelName: "gpt-5.6-terra", Tokens: 1_000, TokenPercentage: 90.91, CostUSD: "0.00", ChargedQuota: 120, CostPercentage: 15, RequestCount: 2,
	}, response.TopCostModels[1])
	require.Len(t, response.Keys, 2)
	assert.Equal(t, AIUsageKey{
		KeyID: firstKey.Id, KeyLabel: "BP - Alice", WeeklyQuotaUSD: "0.00", WeeklyRemainingUSD: "0.00",
		WeeklyRemainingPercent: 19.59, CostUSDToday: "0.00", CostUSDThisWeek: "0.00",
		ModelDistribution: []AIUsageModel{
			{ModelName: "gpt-5.6-sol", Tokens: 100, TokenPercentage: 10, CostUSD: "0.00", ChargedQuota: 680, CostPercentage: 87.18, RequestCount: 1},
			{ModelName: "gpt-5.6-terra", Tokens: 900, TokenPercentage: 90, CostUSD: "0.00", ChargedQuota: 100, CostPercentage: 12.82, RequestCount: 1},
		},
	}, response.Keys[0])
	assert.Equal(t, "0.00", response.Keys[1].WeeklyQuotaUSD)
	assert.Equal(t, "0.00", response.Keys[1].CostUSDThisWeek)
}

func TestGetAIUsageCachesSuccessfulResponseForFiveMinutes(t *testing.T) {
	resetAIUsageCache(t)
	mainDB, logDB := setupAIUsageDatabases(t)
	user := model.User{Username: "babypro", Password: "test-password", Status: 1, Role: 1}
	require.NoError(t, mainDB.Create(&user).Error)
	firstKey := model.Token{UserId: user.Id, Key: "first-key", Name: "first", RemainQuota: 100}
	require.NoError(t, mainDB.Create(&firstKey).Error)
	now := time.Date(2026, time.July, 23, 10, 30, 0, 0, time.UTC)
	require.NoError(t, logDB.Create(&model.Log{
		UserId: user.Id, CreatedAt: now.Unix(), Type: model.LogTypeConsume,
		TokenId: firstKey.Id, ModelName: "gpt-test", PromptTokens: 10, CompletionTokens: 5, Quota: 50,
	}).Error)

	first, err := GetAIUsage("babypro", now)
	require.NoError(t, err)
	assert.Equal(t, "0.00", first.Summary.CostUSDToday)

	secondKey := model.Token{UserId: user.Id, Key: "second-key", Name: "second", RemainQuota: 100}
	require.NoError(t, mainDB.Create(&secondKey).Error)
	require.NoError(t, logDB.Create(&model.Log{
		UserId: user.Id, CreatedAt: now.Add(time.Minute).Unix(), Type: model.LogTypeConsume,
		TokenId: secondKey.Id, ModelName: "gpt-test", PromptTokens: 20, CompletionTokens: 10, Quota: 30,
	}).Error)

	cached, err := GetAIUsage("babypro", now.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, first.GeneratedAt, cached.GeneratedAt)
	assert.Len(t, cached.Keys, 1)

	refreshed, err := GetAIUsage("babypro", now.Add(aiUsageCacheTTL+time.Second))
	require.NoError(t, err)
	assert.Len(t, refreshed.Keys, 2)
	assert.Equal(t, "0.00", refreshed.Summary.CostUSDToday)
}

func TestQuotaToUSDRoundsAndKeepsTwoDecimalPlaces(t *testing.T) {
	assert.Equal(t, "1.00", quotaToUSD(500_000))
	assert.Equal(t, "1.36", quotaToUSD(680_400))
	assert.Equal(t, "1.20", quotaToUSD(600_000))
	assert.Equal(t, "0.00", quotaToUSD(1))
}

func TestAIUsageModelSerializesUSDWithoutInternalQuota(t *testing.T) {
	payload, err := json.Marshal(AIUsageModel{CostUSD: "1.20", ChargedQuota: 600_000})
	require.NoError(t, err)
	assert.JSONEq(t, `{"model_name":"","tokens":0,"token_percentage":0,"cost_usd":"1.20","cost_percentage":0,"request_count":0}`, string(payload))
}

func setupAIUsageDatabases(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
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
	return mainDB, logDB
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
