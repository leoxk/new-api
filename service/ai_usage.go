package service

import (
	"os"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/sync/singleflight"
)

const (
	defaultAIUsageCapacityPath = "/quota-state/last-good.json"
	aiUsageCacheTTL            = 5 * time.Minute
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
	SchemaVersion    int                  `json:"schema_version"`
	User             AIUsageUser          `json:"user"`
	GeneratedAt      string               `json:"generated_at"`
	Stale            bool                 `json:"stale"`
	HistoryStartedAt string               `json:"history_started_at"`
	CodexCapacity    AIUsageCodexCapacity `json:"codex_capacity"`
	Keys             []AIUsageKey         `json:"keys"`
}

type AIUsageUser struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
}

type AIUsageKey struct {
	KeyID     int    `json:"key_id"`
	KeyLabel  string `json:"key_label"`
	Tokens1D  int64  `json:"tokens_1d"`
	Tokens7D  int64  `json:"tokens_7d"`
	Tokens30D int64  `json:"tokens_30d"`
}

type AIUsageCodexCapacity struct {
	FiveHour *AIUsageCapacityWindow `json:"five_hour"`
	SevenDay *AIUsageCapacityWindow `json:"seven_day"`
}

type AIUsageCapacityWindow struct {
	RemainingPercent float64 `json:"remaining_percent"`
	ResetAt          string  `json:"reset_at"`
}

type aiUsageCapacityCache struct {
	FiveHour             *aiUsageCapacityCacheWindow `json:"fiveHour"`
	SevenDay             *aiUsageCapacityCacheWindow `json:"sevenDay"`
	PrimaryUsedPercent   *float64                    `json:"primaryUsedPercent"`
	PrimaryResetAt       *int64                      `json:"primaryResetAt"`
	SecondaryUsedPercent *float64                    `json:"secondaryUsedPercent"`
	SecondaryResetAt     *int64                      `json:"secondaryResetAt"`
}

type aiUsageCapacityCacheWindow struct {
	UsedPercent float64 `json:"usedPercent"`
	ResetAt     int64   `json:"resetAt"`
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
	data, err := model.GetAIUsageByUsername(username, now.Unix())
	if err != nil {
		return nil, err
	}

	historyStartedAt := now
	if data.HistoryStartedAt > 0 {
		historyStartedAt = time.Unix(data.HistoryStartedAt, 0).UTC()
	}
	response := &AIUsageResponse{
		SchemaVersion: 3,
		User: AIUsageUser{
			UserID:   data.UserID,
			Username: data.Username,
		},
		GeneratedAt:      now.Format(time.RFC3339Nano),
		Stale:            false,
		HistoryStartedAt: historyStartedAt.Format(time.RFC3339Nano),
		CodexCapacity:    readAIUsageCodexCapacity(),
		Keys:             make([]AIUsageKey, 0, len(data.Keys)),
	}
	for _, key := range data.Keys {
		response.Keys = append(response.Keys, AIUsageKey{
			KeyID:     key.KeyID,
			KeyLabel:  key.KeyLabel,
			Tokens1D:  key.Tokens1D,
			Tokens7D:  key.Tokens7D,
			Tokens30D: key.Tokens30D,
		})
	}
	return response, nil
}

func readAIUsageCodexCapacity() AIUsageCodexCapacity {
	path := os.Getenv("AI_USAGE_CAPACITY_PATH")
	if path == "" {
		path = defaultAIUsageCapacityPath
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return AIUsageCodexCapacity{}
	}
	var cache aiUsageCapacityCache
	if err := common.Unmarshal(content, &cache); err != nil {
		return AIUsageCodexCapacity{}
	}

	fiveHour := cache.FiveHour
	if fiveHour == nil && cache.PrimaryUsedPercent != nil && cache.PrimaryResetAt != nil {
		fiveHour = &aiUsageCapacityCacheWindow{UsedPercent: *cache.PrimaryUsedPercent, ResetAt: *cache.PrimaryResetAt}
	}
	sevenDay := cache.SevenDay
	if sevenDay == nil && cache.SecondaryUsedPercent != nil && cache.SecondaryResetAt != nil {
		sevenDay = &aiUsageCapacityCacheWindow{UsedPercent: *cache.SecondaryUsedPercent, ResetAt: *cache.SecondaryResetAt}
	}
	return AIUsageCodexCapacity{
		FiveHour: formatAIUsageCapacityWindow(fiveHour),
		SevenDay: formatAIUsageCapacityWindow(sevenDay),
	}
}

func formatAIUsageCapacityWindow(window *aiUsageCapacityCacheWindow) *AIUsageCapacityWindow {
	if window == nil {
		return nil
	}
	return &AIUsageCapacityWindow{
		RemainingPercent: 100 - window.UsedPercent,
		ResetAt:          time.Unix(window.ResetAt, 0).UTC().Format(time.RFC3339Nano),
	}
}
