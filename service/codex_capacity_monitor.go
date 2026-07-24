package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const codexCapacityRefreshInterval = 5 * time.Minute
const codexCapacityStaleAfter = 10 * time.Minute

var codexCapacityOnce sync.Once
var codexInstanceIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
var ErrNoCodexResetCredit = errors.New("no reset credit is available")
var ErrCodexResetInProgress = errors.New("a reset operation is already pending for this OAuth instance")

type CodexOAuthInstance struct { ID string `json:"id"`; DisplayName string `json:"display_name"`; BaseURL string `json:"base_url"`; AuthFile string `json:"auth_file"` }
type codexOAuthAuthFile struct { Tokens struct { AccessToken string `json:"access_token"`; AccountID string `json:"account_id"` } `json:"tokens"` }
type codexWhamUsage struct { Allowed bool `json:"allowed"`; RateLimit struct { Allowed bool `json:"allowed"`; Windows []struct { LimitWindowSeconds int64 `json:"limit_window_seconds"`; UsedPercent float64 `json:"used_percent"`; ResetAt int64 `json:"reset_at"` } `json:"-"`; Primary *codexWhamWindow `json:"primary_window"`; Secondary *codexWhamWindow `json:"secondary_window"` } `json:"rate_limit"` }
type codexWhamWindow struct { LimitWindowSeconds int64 `json:"limit_window_seconds"`; UsedPercent float64 `json:"used_percent"`; ResetAt int64 `json:"reset_at"` }
type codexWhamCreditPayload struct { Credits []struct { ID string `json:"id"`; ResetType string `json:"reset_type"`; Status string `json:"status"`; GrantedAt *string `json:"granted_at"`; ExpiresAt *string `json:"expires_at"`; RedeemedAt *string `json:"redeemed_at"`; Title string `json:"title"`; Description string `json:"description"` } `json:"credits"` }

func LoadCodexOAuthInstances() ([]CodexOAuthInstance, error) {
	raw := strings.TrimSpace(os.Getenv("CODEX_OAUTH_INSTANCES"))
	if raw == "" { raw = `[{"id":"primary","display_name":"Primary","base_url":"http://openai-oauth:10531","auth_file":"/codex-auth/primary/auth.json"}]` }
	var instances []CodexOAuthInstance
	if err := common.UnmarshalJsonStr(raw, &instances); err != nil { return nil, fmt.Errorf("invalid CODEX_OAUTH_INSTANCES: %w", err) }
	if len(instances) == 0 { return nil, errors.New("CODEX_OAUTH_INSTANCES must contain at least one instance") }
	seen := map[string]bool{}
	for i := range instances {
		item := &instances[i]; item.ID = strings.TrimSpace(item.ID); item.DisplayName = strings.TrimSpace(item.DisplayName); item.BaseURL = strings.TrimRight(strings.TrimSpace(item.BaseURL), "/"); item.AuthFile = strings.TrimSpace(item.AuthFile)
		if !codexInstanceIDPattern.MatchString(item.ID) || seen[item.ID] { return nil, fmt.Errorf("invalid or duplicate Codex OAuth instance id %q", item.ID) }; seen[item.ID] = true
		if item.DisplayName == "" { item.DisplayName = item.ID }
		u, err := url.Parse(item.BaseURL); if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" { return nil, fmt.Errorf("invalid base_url for Codex OAuth instance %q", item.ID) }
		if !strings.HasPrefix(item.AuthFile, "/") { return nil, fmt.Errorf("auth_file for Codex OAuth instance %q must be absolute", item.ID) }
	}
	return instances, nil
}

func StartCodexCapacityMonitor() {
	codexCapacityOnce.Do(func() {
		if !common.IsMasterNode { return }
		go func() { RefreshAllCodexCapacity(context.Background()); ticker := time.NewTicker(codexCapacityRefreshInterval); defer ticker.Stop(); for range ticker.C { RefreshAllCodexCapacity(context.Background()) } }()
	})
}

func RefreshAllCodexCapacity(ctx context.Context) {
	instances, err := LoadCodexOAuthInstances(); if err != nil { common.SysError("codex capacity configuration: " + err.Error()); return }
	for _, instance := range instances { if _, err := RefreshCodexCapacityInstance(ctx, instance); err != nil { common.SysError("codex capacity refresh failed for " + instance.ID + ": " + err.Error()); _ = model.SetCodexCapacityError(instance.ID, "refresh failed") } }
}

func GetCodexOAuthInstance(id string) (CodexOAuthInstance, error) { instances, err := LoadCodexOAuthInstances(); if err != nil { return CodexOAuthInstance{}, err }; for _, instance := range instances { if instance.ID == id { return instance, nil } }; return CodexOAuthInstance{}, fmt.Errorf("Codex OAuth instance %q is not configured", id) }

func RefreshCodexCapacityInstance(parent context.Context, instance CodexOAuthInstance) (*model.CodexCapacitySnapshot, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second); defer cancel()
	client := &http.Client{Timeout: 15 * time.Second}
	refresh, err := http.NewRequestWithContext(ctx, http.MethodGet, instance.BaseURL+"/v1/models", nil); if err != nil { return nil, err }
	if resp, err := client.Do(refresh); err != nil { return nil, fmt.Errorf("OAuth refresh request: %w", err) } else { resp.Body.Close(); if resp.StatusCode < 200 || resp.StatusCode >= 300 { return nil, fmt.Errorf("OAuth refresh status %d", resp.StatusCode) } }
	rawAuth, err := os.ReadFile(instance.AuthFile); if err != nil { return nil, fmt.Errorf("read OAuth state: %w", err) }
	var auth codexOAuthAuthFile; if err := common.Unmarshal(rawAuth, &auth); err != nil { return nil, fmt.Errorf("decode OAuth state: %w", err) }
	if strings.TrimSpace(auth.Tokens.AccessToken) == "" || strings.TrimSpace(auth.Tokens.AccountID) == "" { return nil, errors.New("OAuth state is incomplete") }
	usageStatus, usageBody, err := FetchCodexWhamUsage(ctx, client, "https://chatgpt.com", auth.Tokens.AccessToken, auth.Tokens.AccountID)
	if err != nil { return nil, fmt.Errorf("fetch usage: %w", err) }
	if usageStatus < 200 || usageStatus >= 300 { return nil, fmt.Errorf("usage status %d", usageStatus) }
	creditsStatus, creditsBody, err := FetchCodexWhamRateLimitResetCredits(ctx, client, "https://chatgpt.com", auth.Tokens.AccessToken, auth.Tokens.AccountID)
	if err != nil { return nil, fmt.Errorf("fetch reset credits: %w", err) }
	if creditsStatus < 200 || creditsStatus >= 300 { return nil, fmt.Errorf("reset credits status %d", creditsStatus) }
	var usage codexWhamUsage; if err := common.Unmarshal(usageBody, &usage); err != nil { return nil, fmt.Errorf("decode usage: %w", err) }
	var creditsPayload codexWhamCreditPayload; if err := common.Unmarshal(creditsBody, &creditsPayload); err != nil { return nil, fmt.Errorf("decode reset credits: %w", err) }
	now := common.GetTimestamp(); snapshot := &model.CodexCapacitySnapshot{InstanceID: instance.ID, DisplayName: instance.DisplayName, Allowed: usage.RateLimit.Allowed, CheckedAt: now}
	for _, window := range []*codexWhamWindow{usage.RateLimit.Primary, usage.RateLimit.Secondary} { if window == nil { continue }; if window.LimitWindowSeconds == 18000 { snapshot.FiveHourUsedPercent = window.UsedPercent; snapshot.FiveHourResetAt = window.ResetAt }; if window.LimitWindowSeconds == 604800 { snapshot.SevenDayUsedPercent = window.UsedPercent; snapshot.SevenDayResetAt = window.ResetAt } }
	credits := make([]*model.CodexResetCredit, 0, len(creditsPayload.Credits)); for _, credit := range creditsPayload.Credits { if strings.TrimSpace(credit.ID) == "" { continue }; row := &model.CodexResetCredit{InstanceID: instance.ID, CreditID: credit.ID, ResetType: credit.ResetType, Status: credit.Status, Title: credit.Title, Description: credit.Description}; if credit.GrantedAt != nil { row.GrantedAt = *credit.GrantedAt }; if credit.ExpiresAt != nil { row.ExpiresAt = *credit.ExpiresAt }; if credit.RedeemedAt != nil { row.RedeemedAt = *credit.RedeemedAt }; credits = append(credits, row) }
	if err := model.UpsertCodexCapacitySnapshot(snapshot, credits); err != nil { return nil, err }; return snapshot, nil
}

func UseCodexResetCredit(ctx context.Context, instanceID string, actorID int, idempotencyKey string) (*model.CodexResetOperation, error) {
	if existing, err := model.GetCodexResetOperation(instanceID, idempotencyKey); err != nil { return nil, err } else if existing != nil { return existing, nil }
	instance, err := GetCodexOAuthInstance(instanceID); if err != nil { return nil, err }
	if _, err := RefreshCodexCapacityInstance(ctx, instance); err != nil { return nil, err }
	credits, err := model.ListCodexResetCredits(instanceID); if err != nil { return nil, err }; available := false; for _, credit := range credits { if strings.EqualFold(credit.Status, "available") { available = true; break } }; if !available { return nil, ErrNoCodexResetCredit }
	activeLock := instanceID
	op := &model.CodexResetOperation{InstanceID: instanceID, IdempotencyKey: idempotencyKey, ActorID: actorID, Status: model.CodexResetOperationPending, ActiveLock: &activeLock}
	if err := model.CreateCodexResetOperation(op); err != nil {
		if existing, lookupErr := model.GetCodexResetOperation(instanceID, idempotencyKey); lookupErr == nil && existing != nil { return existing, nil }
		if active, lookupErr := model.GetActiveCodexResetOperation(instanceID); lookupErr == nil && active != nil { return nil, ErrCodexResetInProgress }
		return nil, err
	}
	rawAuth, err := os.ReadFile(instance.AuthFile); if err != nil { _ = model.CompleteCodexResetOperation(op, model.CodexResetOperationFailed, 0); return op, err }; var auth codexOAuthAuthFile; if err := common.Unmarshal(rawAuth, &auth); err != nil { _ = model.CompleteCodexResetOperation(op, model.CodexResetOperationFailed, 0); return op, err }
	status, _, err := ConsumeCodexWhamRateLimitResetCreditWithRequestID(ctx, &http.Client{Timeout: 15 * time.Second}, "https://chatgpt.com", auth.Tokens.AccessToken, auth.Tokens.AccountID, idempotencyKey)
	if err != nil { _ = model.CompleteCodexResetOperation(op, model.CodexResetOperationUncertain, status); op.Status = model.CodexResetOperationUncertain; op.UpstreamStatus = status; return op, err }
	if status < 200 || status >= 300 { _ = model.CompleteCodexResetOperation(op, model.CodexResetOperationFailed, status); op.Status = model.CodexResetOperationFailed; op.UpstreamStatus = status; return op, fmt.Errorf("upstream reset status %d", status) }
	_ = model.CompleteCodexResetOperation(op, model.CodexResetOperationSucceeded, status)
	op.Status = model.CodexResetOperationSucceeded
	op.UpstreamStatus = status
	if _, refreshErr := RefreshCodexCapacityInstance(ctx, instance); refreshErr != nil {
		_ = model.SetCodexCapacityError(instance.ID, "refresh failed")
	}
	return op, nil
}
