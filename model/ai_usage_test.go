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

func TestGetAIUsageByUsernameReturnsCurrentKeysAndRollingTotals(t *testing.T) {
	setupAIUsageTestDatabases(t)
	const now = int64(2_000_000_000)

	user := User{Username: "leo", Password: "test-password", Status: 1, Role: 1, AffCode: "leo-aff"}
	require.NoError(t, DB.Create(&user).Error)
	otherUser := User{Username: "other", Password: "test-password", Status: 1, Role: 1, AffCode: "other-aff"}
	require.NoError(t, DB.Create(&otherUser).Error)

	first := Token{UserId: user.Id, Key: "first-key", Name: "first"}
	second := Token{UserId: user.Id, Key: "second-key"}
	deleted := Token{UserId: user.Id, Key: "deleted-key", Name: "deleted"}
	other := Token{UserId: otherUser.Id, Key: "other-key", Name: "other"}
	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)
	require.NoError(t, DB.Create(&deleted).Error)
	require.NoError(t, DB.Create(&other).Error)
	require.NoError(t, DB.Delete(&deleted).Error)

	logs := []Log{
		{TokenId: first.Id, UserId: user.Id, Type: LogTypeConsume, CreatedAt: now - 100, ModelName: "gpt-large", PromptTokens: 10, CompletionTokens: 5, Other: `{"cache_tokens":6}`},
		{TokenId: first.Id, UserId: user.Id, Type: LogTypeConsume, CreatedAt: now - 2*aiUsageDaySeconds, ModelName: "gpt-small", PromptTokens: 20, CompletionTokens: 10, Other: `{"cache_tokens":4}`},
		{TokenId: first.Id, UserId: user.Id, Type: LogTypeConsume, CreatedAt: now - 10*aiUsageDaySeconds, ModelName: "gpt-old", PromptTokens: 30, CompletionTokens: 15, Other: `{"cache_tokens":30}`},
		{TokenId: first.Id, UserId: user.Id, Type: LogTypeTopup, CreatedAt: now - 50, PromptTokens: 999, CompletionTokens: 999},
		{TokenId: deleted.Id, UserId: user.Id, Type: LogTypeConsume, CreatedAt: now - 50, PromptTokens: 999, CompletionTokens: 999},
		{TokenId: other.Id, UserId: otherUser.Id, Type: LogTypeConsume, CreatedAt: now - 40*aiUsageDaySeconds, PromptTokens: 1, CompletionTokens: 1},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)

	usage, err := GetAIUsageByUsername("leo", now)
	require.NoError(t, err)
	assert.Equal(t, user.Id, usage.UserID)
	assert.Equal(t, "leo", usage.Username)
	assert.Equal(t, now-40*aiUsageDaySeconds, usage.HistoryStartedAt)
	require.Len(t, usage.Keys, 2)
	assert.Equal(t, AIUsageKey{
		KeyID: first.Id, KeyLabel: "first", Tokens1D: 15, Tokens7D: 45, Tokens30D: 90,
		PromptTokens7D: 30, CacheTokens7D: 10,
		ModelDistribution: []AIUsageModel{
			{ModelName: "gpt-small", Tokens7D: 30},
			{ModelName: "gpt-large", Tokens7D: 15},
		},
	}, usage.Keys[0])
	assert.Equal(t, AIUsageKey{
		KeyID: second.Id, KeyLabel: "Key 2", Tokens1D: 0, Tokens7D: 0, Tokens30D: 0,
	}, usage.Keys[1])
}

func TestGetAIUsageByUsernameReturnsNotFound(t *testing.T) {
	setupAIUsageTestDatabases(t)

	_, err := GetAIUsageByUsername("missing", 2_000_000_000)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
