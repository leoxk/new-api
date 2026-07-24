package service

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

type CodexCapacityResponse struct {
	Instances []CodexCapacityInstance `json:"instances"`
}

type CodexCapacityInstance struct {
	ID                  string                 `json:"id"`
	DisplayName         string                 `json:"display_name"`
	Allowed             bool                   `json:"allowed"`
	Stale               bool                   `json:"stale"`
	CheckedAt           string                 `json:"checked_at,omitempty"`
	FiveHour            *CodexCapacityWindow  `json:"five_hour"`
	SevenDay            *CodexCapacityWindow  `json:"seven_day"`
	AvailableResetCount int                    `json:"available_reset_count"`
	Credits             []CodexResetCreditView `json:"credits"`
}

type CodexCapacityWindow struct { RemainingPercent float64 `json:"remaining_percent"`; ResetAt string `json:"reset_at"` }
type CodexResetCreditView struct { ResetType string `json:"reset_type"`; Status string `json:"status"`; Title string `json:"title"`; Description string `json:"description,omitempty"`; GrantedAt string `json:"granted_at,omitempty"`; ExpiresAt string `json:"expires_at,omitempty"`; RedeemedAt string `json:"redeemed_at,omitempty"` }

func GetCodexCapacity(now time.Time) (*CodexCapacityResponse, error) {
	instances, err := LoadCodexOAuthInstances(); if err != nil { return nil, err }
	snapshots, err := model.ListCodexCapacitySnapshots(); if err != nil { return nil, err }
	byID := make(map[string]*model.CodexCapacitySnapshot, len(snapshots)); for _, row := range snapshots { byID[row.InstanceID] = row }
	response := &CodexCapacityResponse{Instances: make([]CodexCapacityInstance, 0, len(instances))}
	for _, configured := range instances {
		entry := CodexCapacityInstance{ID: configured.ID, DisplayName: configured.DisplayName, Stale: true, Credits: []CodexResetCreditView{}}
		if snapshot := byID[configured.ID]; snapshot != nil {
			entry.Allowed = snapshot.Allowed; entry.Stale = snapshot.LastError != "" || snapshot.CheckedAt == 0 || now.UTC().Sub(time.Unix(snapshot.CheckedAt, 0).UTC()) > codexCapacityStaleAfter
			if snapshot.CheckedAt > 0 { entry.CheckedAt = time.Unix(snapshot.CheckedAt, 0).UTC().Format(time.RFC3339) }
			if snapshot.FiveHourResetAt > 0 { entry.FiveHour = &CodexCapacityWindow{RemainingPercent: 100 - snapshot.FiveHourUsedPercent, ResetAt: time.Unix(snapshot.FiveHourResetAt, 0).UTC().Format(time.RFC3339)} }
			if snapshot.SevenDayResetAt > 0 { entry.SevenDay = &CodexCapacityWindow{RemainingPercent: 100 - snapshot.SevenDayUsedPercent, ResetAt: time.Unix(snapshot.SevenDayResetAt, 0).UTC().Format(time.RFC3339)} }
		}
		credits, err := model.ListCodexResetCredits(configured.ID); if err != nil { return nil, err }
		for _, credit := range credits { view := CodexResetCreditView{ResetType: credit.ResetType, Status: credit.Status, Title: credit.Title, Description: credit.Description, GrantedAt: credit.GrantedAt, ExpiresAt: credit.ExpiresAt, RedeemedAt: credit.RedeemedAt}; entry.Credits = append(entry.Credits, view); if strings.EqualFold(credit.Status, "available") { entry.AvailableResetCount++ } }
		response.Instances = append(response.Instances, entry)
	}
	return response, nil
}

type AdminCodexCapacityResponse struct { Capacity *CodexCapacityResponse `json:"capacity"`; RecentOperations []*model.CodexResetOperation `json:"recent_operations"` }
func GetAdminCodexCapacity(now time.Time) (*AdminCodexCapacityResponse, error) { capacity, err := GetCodexCapacity(now); if err != nil { return nil, err }; operations, err := model.ListRecentCodexResetOperations(50); if err != nil { return nil, err }; return &AdminCodexCapacityResponse{Capacity: capacity, RecentOperations: operations}, nil }
