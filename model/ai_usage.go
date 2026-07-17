package model

import (
	"database/sql"
	"strconv"
)

const (
	aiUsageDaySeconds        = int64(86_400)
	aiUsageSevenDaysSeconds  = 7 * aiUsageDaySeconds
	aiUsageThirtyDaysSeconds = 30 * aiUsageDaySeconds
)

type AIUsageKey struct {
	KeyID     int
	KeyLabel  string
	Tokens1D  int64
	Tokens7D  int64
	Tokens30D int64
}

type AIUsageData struct {
	UserID           int
	Username         string
	HistoryStartedAt int64
	Keys             []AIUsageKey
}

// GetAIUsageByUsername returns every current key owned by a user together with
// rolling token totals from consumption logs. Main and log databases are read
// separately because New API supports deploying them on different backends.
func GetAIUsageByUsername(username string, now int64) (*AIUsageData, error) {
	var user User
	if err := DB.Select("id", "username").Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}

	var tokens []Token
	if err := DB.Select("id", "name").Where("user_id = ?", user.Id).Order("id asc").Find(&tokens).Error; err != nil {
		return nil, err
	}

	data := &AIUsageData{
		UserID:   user.Id,
		Username: user.Username,
		Keys:     make([]AIUsageKey, 0, len(tokens)),
	}
	keyIndexes := make(map[int]int, len(tokens))
	tokenIDs := make([]int, 0, len(tokens))
	for _, token := range tokens {
		label := token.Name
		if label == "" {
			label = "Key " + strconv.Itoa(token.Id)
		}
		keyIndexes[token.Id] = len(data.Keys)
		tokenIDs = append(tokenIDs, token.Id)
		data.Keys = append(data.Keys, AIUsageKey{KeyID: token.Id, KeyLabel: label})
	}

	if len(tokenIDs) > 0 {
		var rows []struct {
			TokenID   int   `gorm:"column:token_id"`
			Tokens1D  int64 `gorm:"column:tokens_1d"`
			Tokens7D  int64 `gorm:"column:tokens_7d"`
			Tokens30D int64 `gorm:"column:tokens_30d"`
		}
		selectUsage := `token_id,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN CAST(prompt_tokens AS BIGINT) + CAST(completion_tokens AS BIGINT) ELSE 0 END), 0) AS tokens_1d,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN CAST(prompt_tokens AS BIGINT) + CAST(completion_tokens AS BIGINT) ELSE 0 END), 0) AS tokens_7d,
			COALESCE(SUM(CASE WHEN created_at >= ? THEN CAST(prompt_tokens AS BIGINT) + CAST(completion_tokens AS BIGINT) ELSE 0 END), 0) AS tokens_30d`
		if err := LOG_DB.Model(&Log{}).
			Select(selectUsage, now-aiUsageDaySeconds, now-aiUsageSevenDaysSeconds, now-aiUsageThirtyDaysSeconds).
			Where("token_id IN ? AND type = ? AND created_at >= ?", tokenIDs, LogTypeConsume, now-aiUsageThirtyDaysSeconds).
			Group("token_id").
			Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			idx, ok := keyIndexes[row.TokenID]
			if !ok {
				continue
			}
			data.Keys[idx].Tokens1D = row.Tokens1D
			data.Keys[idx].Tokens7D = row.Tokens7D
			data.Keys[idx].Tokens30D = row.Tokens30D
		}
	}

	var earliest sql.NullInt64
	if err := LOG_DB.Model(&Log{}).
		Select("MIN(created_at)").
		Where("type = ?", LogTypeConsume).
		Scan(&earliest).Error; err != nil {
		return nil, err
	}
	if earliest.Valid {
		data.HistoryStartedAt = earliest.Int64
	}
	return data, nil
}
