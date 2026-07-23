package service

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/sync/singleflight"
)

const (
	aiUsageCacheTTL = 5 * time.Minute
	aiUsageTimezone = "Asia/Hong_Kong"
)

type aiUsageCacheEntry struct {
	response  *AIUsageResponse
	expiresAt time.Time
}

var (
	aiUsageCache        sync.Map
	aiUsageRefreshGroup singleflight.Group
)

type AIUsageResponse struct {
	SchemaVersion int            `json:"schema_version"`
	Company       AIUsageCompany `json:"company"`
	GeneratedAt   string         `json:"generated_at"`
	Period        AIUsagePeriod  `json:"period"`
	Summary       AIUsageSummary `json:"summary"`
	TopCostModels []AIUsageModel `json:"top_cost_models_this_week"`
	Keys          []AIUsageKey   `json:"keys"`
}

type AIUsageCompany struct {
	Username string `json:"username"`
	Timezone string `json:"timezone"`
}

type AIUsagePeriod struct {
	TodayStartedAt string `json:"today_started_at"`
	WeekStartedAt  string `json:"week_started_at"`
}

type AIUsageSummary struct {
	CostUSDToday              string `json:"cost_usd_today"`
	CostUSDThisWeek           string `json:"cost_usd_this_week"`
	CompanyWalletRemainingUSD string `json:"company_wallet_remaining_usd"`
	KeyCount                  int    `json:"key_count"`
}

type AIUsageKey struct {
	KeyID                  int            `json:"key_id"`
	KeyLabel               string         `json:"key_label"`
	WeeklyQuotaUSD         string         `json:"weekly_quota_usd"`
	WeeklyRemainingUSD     string         `json:"weekly_remaining_usd"`
	WeeklyRemainingPercent float64        `json:"weekly_remaining_percent"`
	WeeklyQuotaUnlimited   bool           `json:"weekly_quota_unlimited"`
	CostUSDToday           string         `json:"cost_usd_today"`
	CostUSDThisWeek        string         `json:"cost_usd_this_week"`
	ModelDistribution      []AIUsageModel `json:"model_distribution_this_week"`
}

type AIUsageModel struct {
	ModelName       string  `json:"model_name"`
	Tokens          int64   `json:"tokens"`
	TokenPercentage float64 `json:"token_percentage"`
	CostUSD         string  `json:"cost_usd"`
	CostPercentage  float64 `json:"cost_percentage"`
	RequestCount    int64   `json:"request_count"`
	ChargedQuota    int64   `json:"-"`
}

func GetAIUsage(username string, now time.Time) (*AIUsageResponse, error) {
	now = now.UTC()
	if cached, ok := aiUsageCache.Load(username); ok {
		entry := cached.(aiUsageCacheEntry)
		if now.Before(entry.expiresAt) {
			return entry.response, nil
		}
		aiUsageCache.Delete(username)
	}

	value, err, _ := aiUsageRefreshGroup.Do(username, func() (any, error) {
		if cached, ok := aiUsageCache.Load(username); ok {
			entry := cached.(aiUsageCacheEntry)
			if now.Before(entry.expiresAt) {
				return entry.response, nil
			}
			aiUsageCache.Delete(username)
		}

		response, err := buildAIUsageResponse(username, now)
		if err != nil {
			return nil, err
		}
		aiUsageCache.Store(username, aiUsageCacheEntry{
			response:  response,
			expiresAt: now.Add(aiUsageCacheTTL),
		})
		return response, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*AIUsageResponse), nil
}

func buildAIUsageResponse(username string, now time.Time) (*AIUsageResponse, error) {
	location, err := time.LoadLocation(aiUsageTimezone)
	if err != nil {
		return nil, err
	}
	generatedAt := now.In(location)
	todayStartedAt := time.Date(generatedAt.Year(), generatedAt.Month(), generatedAt.Day(), 0, 0, 0, 0, location)
	weekStartedAt := todayStartedAt.AddDate(0, 0, -((int(todayStartedAt.Weekday()) + 6) % 7))
	data, err := model.GetAIUsageByUsername(username, todayStartedAt.Unix(), weekStartedAt.Unix())
	if err != nil {
		return nil, err
	}

	response := &AIUsageResponse{
		SchemaVersion: 5,
		Company: AIUsageCompany{
			Username: data.Username,
			Timezone: aiUsageTimezone,
		},
		GeneratedAt: generatedAt.Format(time.RFC3339Nano),
		Period: AIUsagePeriod{
			TodayStartedAt: todayStartedAt.Format(time.RFC3339Nano),
			WeekStartedAt:  weekStartedAt.Format(time.RFC3339Nano),
		},
		Summary: AIUsageSummary{
			CompanyWalletRemainingUSD: quotaToUSD(data.CompanyWalletRemainingQuota),
			KeyCount:                  len(data.Keys),
		},
		TopCostModels: make([]AIUsageModel, 0),
		Keys:          make([]AIUsageKey, 0, len(data.Keys)),
	}
	companyModels := make(map[string]*AIUsageModel)
	var companyQuotaUsedToday int64
	var companyQuotaUsedThisWeek int64
	for _, key := range data.Keys {
		weeklyQuota := key.RemainQuota + key.QuotaUsedThisWeek
		if key.UnlimitedQuota {
			weeklyQuota = 0
		}
		usageKey := AIUsageKey{
			KeyID:                key.KeyID,
			KeyLabel:             key.KeyLabel,
			WeeklyQuotaUSD:       quotaToUSD(weeklyQuota),
			WeeklyRemainingUSD:   quotaToUSD(key.RemainQuota),
			WeeklyQuotaUnlimited: key.UnlimitedQuota,
			CostUSDToday:         quotaToUSD(key.QuotaUsedToday),
			CostUSDThisWeek:      quotaToUSD(key.QuotaUsedThisWeek),
			ModelDistribution:    make([]AIUsageModel, 0, len(key.ModelDistribution)),
		}
		if weeklyQuota > 0 {
			usageKey.WeeklyRemainingPercent = roundedPercentage(key.RemainQuota, weeklyQuota)
		}
		companyQuotaUsedToday += key.QuotaUsedToday
		companyQuotaUsedThisWeek += key.QuotaUsedThisWeek

		var keyTokens int64
		for _, usage := range key.ModelDistribution {
			keyTokens += usage.Tokens
		}
		for _, usage := range key.ModelDistribution {
			modelUsage := AIUsageModel{
				ModelName:       usage.ModelName,
				Tokens:          usage.Tokens,
				TokenPercentage: roundedPercentage(usage.Tokens, keyTokens),
				CostUSD:         quotaToUSD(usage.ChargedQuota),
				CostPercentage:  roundedPercentage(usage.ChargedQuota, key.QuotaUsedThisWeek),
				RequestCount:    usage.RequestCount,
				ChargedQuota:    usage.ChargedQuota,
			}
			usageKey.ModelDistribution = append(usageKey.ModelDistribution, modelUsage)
			companyUsage := companyModels[usage.ModelName]
			if companyUsage == nil {
				companyUsage = &AIUsageModel{ModelName: usage.ModelName}
				companyModels[usage.ModelName] = companyUsage
			}
			companyUsage.Tokens += usage.Tokens
			companyUsage.ChargedQuota += usage.ChargedQuota
			companyUsage.RequestCount += usage.RequestCount
		}
		response.Keys = append(response.Keys, usageKey)
	}

	var companyTokens int64
	for _, usage := range companyModels {
		companyTokens += usage.Tokens
	}
	for _, usage := range companyModels {
		response.TopCostModels = append(response.TopCostModels, AIUsageModel{
			ModelName:       usage.ModelName,
			Tokens:          usage.Tokens,
			TokenPercentage: roundedPercentage(usage.Tokens, companyTokens),
			CostUSD:         quotaToUSD(usage.ChargedQuota),
			CostPercentage:  roundedPercentage(usage.ChargedQuota, companyQuotaUsedThisWeek),
			RequestCount:    usage.RequestCount,
			ChargedQuota:    usage.ChargedQuota,
		})
	}
	response.Summary.CostUSDToday = quotaToUSD(companyQuotaUsedToday)
	response.Summary.CostUSDThisWeek = quotaToUSD(companyQuotaUsedThisWeek)
	sort.Slice(response.TopCostModels, func(i, j int) bool {
		if response.TopCostModels[i].ChargedQuota == response.TopCostModels[j].ChargedQuota {
			return response.TopCostModels[i].ModelName < response.TopCostModels[j].ModelName
		}
		return response.TopCostModels[i].ChargedQuota > response.TopCostModels[j].ChargedQuota
	})
	return response, nil
}

func quotaToUSD(quota int64) string {
	if common.QuotaPerUnit <= 0 {
		return "0.00"
	}
	cents := int64(math.Round(float64(quota) * 100 / common.QuotaPerUnit))
	if cents < 0 {
		return fmt.Sprintf("-%d.%02d", -cents/100, -cents%100)
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func roundedPercentage(value int64, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(float64(value)*10_000/float64(total)) / 100
}
