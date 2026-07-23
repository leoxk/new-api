package model

import (
	"sort"
	"strconv"
)

type AIUsageKey struct {
	KeyID             int
	KeyLabel          string
	RemainQuota       int64
	UnlimitedQuota    bool
	QuotaUsedToday    int64
	QuotaUsedThisWeek int64
	ModelDistribution []AIUsageModel
}

type AIUsageModel struct {
	ModelName    string
	Tokens       int64
	ChargedQuota int64
	RequestCount int64
}

type AIUsageData struct {
	UserID                      int
	Username                    string
	CompanyWalletRemainingQuota int64
	Keys                        []AIUsageKey
}

// GetAIUsageByUsername returns every current key owned by a company user and
// its Hong Kong calendar-week consumption. Main and log databases are read
// separately because New API supports deploying them on different backends.
func GetAIUsageByUsername(username string, todayStartedAt int64, weekStartedAt int64) (*AIUsageData, error) {
	var user User
	if err := DB.Select("id", "username", "quota").Where("username = ?", username).First(&user).Error; err != nil {
		return nil, err
	}

	var tokens []Token
	if err := DB.Select("id", "name", "remain_quota", "unlimited_quota").Where("user_id = ?", user.Id).Order("id asc").Find(&tokens).Error; err != nil {
		return nil, err
	}

	data := &AIUsageData{
		UserID:                      user.Id,
		Username:                    user.Username,
		CompanyWalletRemainingQuota: int64(user.Quota),
		Keys:                        make([]AIUsageKey, 0, len(tokens)),
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
		data.Keys = append(data.Keys, AIUsageKey{
			KeyID:          token.Id,
			KeyLabel:       label,
			RemainQuota:    int64(token.RemainQuota),
			UnlimitedQuota: token.UnlimitedQuota,
		})
	}

	if len(tokenIDs) == 0 {
		return data, nil
	}

	var rows []struct {
		TokenID        int    `gorm:"column:token_id"`
		ModelName      string `gorm:"column:model_name"`
		TokensToday    int64  `gorm:"column:tokens_today"`
		TokensThisWeek int64  `gorm:"column:tokens_this_week"`
		QuotaToday     int64  `gorm:"column:quota_today"`
		QuotaThisWeek  int64  `gorm:"column:quota_this_week"`
		RequestCount   int64  `gorm:"column:request_count"`
	}
	selectUsage := `token_id, model_name,
		COALESCE(SUM(CASE WHEN created_at >= ? THEN CAST(prompt_tokens AS BIGINT) + CAST(completion_tokens AS BIGINT) ELSE 0 END), 0) AS tokens_today,
		COALESCE(SUM(CAST(prompt_tokens AS BIGINT) + CAST(completion_tokens AS BIGINT)), 0) AS tokens_this_week,
		COALESCE(SUM(CASE WHEN created_at >= ? THEN quota ELSE 0 END), 0) AS quota_today,
		COALESCE(SUM(quota), 0) AS quota_this_week,
		COUNT(*) AS request_count`
	if err := LOG_DB.Model(&Log{}).
		Select(selectUsage, todayStartedAt, todayStartedAt).
		Where("token_id IN ? AND type = ? AND created_at >= ?", tokenIDs, LogTypeConsume, weekStartedAt).
		Group("token_id, model_name").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	modelTotals := make(map[int]map[string]*AIUsageModel, len(tokens))
	for _, row := range rows {
		idx, ok := keyIndexes[row.TokenID]
		if !ok {
			continue
		}
		key := &data.Keys[idx]
		key.QuotaUsedToday += row.QuotaToday
		key.QuotaUsedThisWeek += row.QuotaThisWeek
		modelName := row.ModelName
		if modelName == "" {
			modelName = "unknown"
		}
		if modelTotals[row.TokenID] == nil {
			modelTotals[row.TokenID] = make(map[string]*AIUsageModel)
		}
		modelUsage := modelTotals[row.TokenID][modelName]
		if modelUsage == nil {
			modelUsage = &AIUsageModel{ModelName: modelName}
			modelTotals[row.TokenID][modelName] = modelUsage
		}
		modelUsage.Tokens += row.TokensThisWeek
		modelUsage.ChargedQuota += row.QuotaThisWeek
		modelUsage.RequestCount += row.RequestCount
	}
	for tokenID, totals := range modelTotals {
		idx := keyIndexes[tokenID]
		models := make([]AIUsageModel, 0, len(totals))
		for _, usage := range totals {
			models = append(models, *usage)
		}
		sort.Slice(models, func(i, j int) bool {
			if models[i].ChargedQuota == models[j].ChargedQuota {
				return models[i].ModelName < models[j].ModelName
			}
			return models[i].ChargedQuota > models[j].ChargedQuota
		})
		data.Keys[idx].ModelDistribution = models
	}
	return data, nil
}
