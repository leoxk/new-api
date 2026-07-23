package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAIUsageTestDatabases(t *testing.T) {
	t.Helper()
	previousDB := DB
	previousLogDB := LOG_DB

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

	DB = mainDB
	LOG_DB = logDB
	require.NoError(t, DB.AutoMigrate(&User{}, &Token{}))
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
	t.Cleanup(func() {
		_ = mainSQL.Close()
		_ = logSQL.Close()
		DB = previousDB
		LOG_DB = previousLogDB
	})
}

func TestGetAIUsageByUsernameReturnsCurrentKeysAndWeeklyCostTotals(t *testing.T) {
	setupAIUsageTestDatabases(t)
	const todayStartedAt = int64(2_000_000_000)
	const weekStartedAt = todayStartedAt - 6*86_400

	user := User{Username: "leo", Password: "test-password", Status: 1, Role: 1, AffCode: "leo-aff", Quota: 9_000}
	require.NoError(t, DB.Create(&user).Error)
	otherUser := User{Username: "other", Password: "test-password", Status: 1, Role: 1, AffCode: "other-aff"}
	require.NoError(t, DB.Create(&otherUser).Error)

	first := Token{UserId: user.Id, Key: "first-key", Name: "first", RemainQuota: 150}
	second := Token{UserId: user.Id, Key: "second-key", RemainQuota: 200}
	deleted := Token{UserId: user.Id, Key: "deleted-key", Name: "deleted"}
	other := Token{UserId: otherUser.Id, Key: "other-key", Name: "other"}
	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)
	require.NoError(t, DB.Create(&deleted).Error)
	require.NoError(t, DB.Create(&other).Error)
	require.NoError(t, DB.Delete(&deleted).Error)

	logs := []Log{
		{TokenId: first.Id, UserId: user.Id, Type: LogTypeConsume, CreatedAt: todayStartedAt + 100, ModelName: "gpt-large", PromptTokens: 10, CompletionTokens: 5, Quota: 200},
		{TokenId: first.Id, UserId: user.Id, Type: LogTypeConsume, CreatedAt: todayStartedAt - 2*86_400, ModelName: "gpt-small", PromptTokens: 20, CompletionTokens: 10, Quota: 150},
		{TokenId: first.Id, UserId: user.Id, Type: LogTypeConsume, CreatedAt: weekStartedAt - 1, ModelName: "gpt-old", PromptTokens: 30, CompletionTokens: 15, Quota: 300},
		{TokenId: first.Id, UserId: user.Id, Type: LogTypeTopup, CreatedAt: todayStartedAt + 50, PromptTokens: 999, CompletionTokens: 999},
		{TokenId: deleted.Id, UserId: user.Id, Type: LogTypeConsume, CreatedAt: todayStartedAt + 50, PromptTokens: 999, CompletionTokens: 999},
		{TokenId: other.Id, UserId: otherUser.Id, Type: LogTypeConsume, CreatedAt: todayStartedAt + 50, PromptTokens: 1, CompletionTokens: 1},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	usage, err := GetAIUsageByUsername("leo", todayStartedAt, weekStartedAt)
	require.NoError(t, err)
	assert.Equal(t, user.Id, usage.UserID)
	assert.Equal(t, "leo", usage.Username)
	assert.Equal(t, int64(9_000), usage.CompanyWalletRemainingQuota)
	require.Len(t, usage.Keys, 2)
	assert.Equal(t, AIUsageKey{
		KeyID: first.Id, KeyLabel: "first", RemainQuota: 150, QuotaUsedToday: 200, QuotaUsedThisWeek: 350,
		ModelDistribution: []AIUsageModel{
			{ModelName: "gpt-large", Tokens: 15, ChargedQuota: 200, RequestCount: 1},
			{ModelName: "gpt-small", Tokens: 30, ChargedQuota: 150, RequestCount: 1},
		},
	}, usage.Keys[0])
	assert.Equal(t, AIUsageKey{
		KeyID: second.Id, KeyLabel: "Key 2", RemainQuota: 200,
	}, usage.Keys[1])
}

func TestGetAIUsageByUsernameReturnsNotFound(t *testing.T) {
	setupAIUsageTestDatabases(t)

	_, err := GetAIUsageByUsername("missing", 2_000_000_000, 1_999_481_600)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
