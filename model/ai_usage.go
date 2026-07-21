package model

import (
	"database/sql"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

const (
	aiUsageDaySeconds        = int64(86_400)
	aiUsageSevenDaysSeconds  = 7 * aiUsageDaySeconds
	aiUsageThirtyDaysSeconds = 30 * aiUsageDaySeconds
)

type AIUsageKey struct {
	KeyID             int
	KeyLabel          string
	Tokens1D          int64
	Tokens7D          int64
	Tokens30D         int64
	PromptTokens7D    int64
	CacheTokens7D     int64
	ModelDistribution []AIUsageModel
}

type AIUsageModel struct {
	ModelName string
	Tokens7D  int64
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

		var detailRows []struct {
			TokenID          int    `gorm:"column:token_id"`
			ModelName        string `gorm:"column:model_name"`
			PromptTokens     int64  `gorm:"column:prompt_tokens"`
			CompletionTokens int64  `gorm:"column:completion_tokens"`
			Other            string `gorm:"column:other"`
		}
		if err := LOG_DB.Model(&Log{}).
			Select("token_id", "model_name", "prompt_tokens", "completion_tokens", "other").
			Where("token_id IN ? AND type = ? AND created_at >= ?", tokenIDs, LogTypeConsume, now-aiUsageSevenDaysSeconds).
			Scan(&detailRows).Error; err != nil {
			return nil, err
		}
		modelTotals := make(map[int]map[string]int64, len(tokens))
		for _, row := range detailRows {
			idx, ok := keyIndexes[row.TokenID]
			if !ok {
				continue
			}
			data.Keys[idx].PromptTokens7D += row.PromptTokens
			var details struct {
				CacheTokens int64 `json:"cache_tokens"`
			}
			if row.Other != "" && common.UnmarshalJsonStr(row.Other, &details) == nil && details.CacheTokens > 0 {
				data.Keys[idx].CacheTokens7D += details.CacheTokens
			}
			modelName := row.ModelName
			if modelName == "" {
				modelName = "unknown"
			}
			if modelTotals[row.TokenID] == nil {
				modelTotals[row.TokenID] = make(map[string]int64)
			}
			modelTotals[row.TokenID][modelName] += row.PromptTokens + row.CompletionTokens
		}
		for tokenID, totals := range modelTotals {
			idx := keyIndexes[tokenID]
			models := make([]AIUsageModel, 0, len(totals))
			for modelName, tokens7D := range totals {
				models = append(models, AIUsageModel{ModelName: modelName, Tokens7D: tokens7D})
			}
			sort.Slice(models, func(i, j int) bool {
				if models[i].Tokens7D == models[j].Tokens7D {
					return models[i].ModelName < models[j].ModelName
				}
				return models[i].Tokens7D > models[j].Tokens7D
			})
			data.Keys[idx].ModelDistribution = models
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
