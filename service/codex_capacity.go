package service

import (
	"fmt"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	defaultCodexCapacityStatePath = "/quota-state/last-good.json"
	codexCapacityStaleAfter       = 10 * time.Minute
)

type CodexCapacityResponse struct {
	Allowed  bool                 `json:"allowed"`
	Stale    bool                 `json:"stale"`
	CheckedAt string               `json:"checked_at"`
	FiveHour *CodexCapacityWindow `json:"five_hour"`
	SevenDay *CodexCapacityWindow `json:"seven_day"`
}

type CodexCapacityWindow struct {
	RemainingPercent float64 `json:"remaining_percent"`
	ResetAt          string  `json:"reset_at"`
}

type codexCapacityState struct {
	Allowed   bool                      `json:"allowed"`
	FiveHour  *codexCapacityStateWindow `json:"fiveHour"`
	SevenDay  *codexCapacityStateWindow `json:"sevenDay"`
	CheckedAt int64                     `json:"checkedAt"`
}

type codexCapacityStateWindow struct {
	UsedPercent float64 `json:"usedPercent"`
	ResetAt     int64   `json:"resetAt"`
}

func GetCodexCapacity(now time.Time) (*CodexCapacityResponse, error) {
	statePath := os.Getenv("CODEX_CAPACITY_STATE_PATH")
	if statePath == "" {
		statePath = defaultCodexCapacityStatePath
	}

	raw, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("read Codex capacity state: %w", err)
	}

	var state codexCapacityState
	if err := common.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("decode Codex capacity state: %w", err)
	}
	if state.CheckedAt <= 0 {
		return nil, fmt.Errorf("Codex capacity state has no check time")
	}

	checkedAt := time.Unix(state.CheckedAt, 0).UTC()
	response := &CodexCapacityResponse{
		Allowed:   state.Allowed,
		Stale:     now.UTC().Sub(checkedAt) > codexCapacityStaleAfter,
		CheckedAt: checkedAt.Format(time.RFC3339),
	}
	if state.FiveHour != nil {
		if state.FiveHour.ResetAt <= 0 {
			return nil, fmt.Errorf("Codex capacity state has an invalid five-hour reset time")
		}
		response.FiveHour = &CodexCapacityWindow{
			RemainingPercent: 100 - state.FiveHour.UsedPercent,
			ResetAt:          time.Unix(state.FiveHour.ResetAt, 0).UTC().Format(time.RFC3339),
		}
	}
	if state.SevenDay != nil {
		if state.SevenDay.ResetAt <= 0 {
			return nil, fmt.Errorf("Codex capacity state has an invalid seven-day reset time")
		}
		response.SevenDay = &CodexCapacityWindow{
			RemainingPercent: 100 - state.SevenDay.UsedPercent,
			ResetAt:          time.Unix(state.SevenDay.ResetAt, 0).UTC().Format(time.RFC3339),
		}
	}
	if response.FiveHour == nil && response.SevenDay == nil {
		return nil, fmt.Errorf("Codex capacity state has no capacity windows")
	}
	return response, nil
}
