package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	CodexResetOperationPending   = "pending"
	CodexResetOperationSucceeded = "succeeded"
	CodexResetOperationFailed    = "failed"
	CodexResetOperationUncertain = "uncertain"
)

// CodexCapacitySnapshot contains only display-safe, normalized upstream state.
// OAuth tokens, account IDs, and the original upstream response are never stored.
type CodexCapacitySnapshot struct {
	InstanceID             string  `json:"instance_id" gorm:"type:varchar(64);primaryKey"`
	DisplayName            string  `json:"display_name" gorm:"type:varchar(128)"`
	Allowed                bool    `json:"allowed"`
	FiveHourUsedPercent    float64 `json:"five_hour_used_percent"`
	FiveHourResetAt        int64   `json:"five_hour_reset_at"`
	SevenDayUsedPercent    float64 `json:"seven_day_used_percent"`
	SevenDayResetAt        int64   `json:"seven_day_reset_at"`
	CheckedAt              int64   `json:"checked_at" gorm:"bigint;index"`
	LastError              string  `json:"-" gorm:"type:text"`
	CreatedAt              int64   `json:"created_at" gorm:"bigint;index"`
	UpdatedAt              int64   `json:"updated_at" gorm:"bigint;index"`
}

type CodexResetCredit struct {
	ID         int64  `json:"-" gorm:"primaryKey"`
	InstanceID string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_codex_reset_credit"`
	CreditID   string `json:"-" gorm:"type:varchar(255);uniqueIndex:idx_codex_reset_credit"`
	ResetType  string `json:"reset_type" gorm:"type:varchar(128)"`
	Status     string `json:"status" gorm:"type:varchar(64);index"`
	Title      string `json:"title" gorm:"type:varchar(255)"`
	Description string `json:"description" gorm:"type:text"`
	GrantedAt  string `json:"granted_at" gorm:"type:varchar(64)"`
	ExpiresAt  string `json:"expires_at" gorm:"type:varchar(64)"`
	RedeemedAt string `json:"redeemed_at" gorm:"type:varchar(64)"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint;index"`
}

type CodexResetOperation struct {
	ID             int64  `json:"id" gorm:"primaryKey"`
	InstanceID     string `json:"instance_id" gorm:"type:varchar(64);uniqueIndex:idx_codex_reset_operation"`
	IdempotencyKey string `json:"idempotency_key" gorm:"type:varchar(128);uniqueIndex:idx_codex_reset_operation"`
	ActorID        int    `json:"actor_id" gorm:"index"`
	Status         string `json:"status" gorm:"type:varchar(32);index"`
	// ActiveLock is set only while an operation may still consume a credit. Its
	// unique index serializes reset attempts for one OAuth instance across API
	// nodes; completed operations clear it to NULL.
	ActiveLock     *string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_codex_reset_operation_active"`
	UpstreamStatus int    `json:"upstream_status"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;index"`
	CompletedAt    int64  `json:"completed_at" gorm:"bigint;index"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;index"`
}

func (m *CodexCapacitySnapshot) BeforeCreate(_ *gorm.DB) error { now := common.GetTimestamp(); m.CreatedAt = now; m.UpdatedAt = now; return nil }
func (m *CodexResetCredit) BeforeCreate(_ *gorm.DB) error { now := common.GetTimestamp(); m.CreatedAt = now; m.UpdatedAt = now; return nil }
func (m *CodexResetOperation) BeforeCreate(_ *gorm.DB) error { now := common.GetTimestamp(); m.CreatedAt = now; m.UpdatedAt = now; return nil }

func UpsertCodexCapacitySnapshot(snapshot *CodexCapacitySnapshot, credits []*CodexResetCredit) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		snapshot.UpdatedAt = common.GetTimestamp()
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "instance_id"}}, DoUpdates: clause.AssignmentColumns([]string{"display_name", "allowed", "five_hour_used_percent", "five_hour_reset_at", "seven_day_used_percent", "seven_day_reset_at", "checked_at", "last_error", "updated_at"})}).Create(snapshot).Error; err != nil { return err }
		if err := tx.Where("instance_id = ?", snapshot.InstanceID).Delete(&CodexResetCredit{}).Error; err != nil { return err }
		if len(credits) == 0 { return nil }
		return tx.Create(&credits).Error
	})
}

func SetCodexCapacityError(instanceID string, message string) error {
	// CheckedAt is deliberately the last successful check time. On an initial
	// failure it remains zero; on later failures it preserves the prior value.
	snapshot := &CodexCapacitySnapshot{InstanceID: instanceID, LastError: message}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "instance_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_error", "updated_at"}),
	}).Create(snapshot).Error
}

func ListCodexCapacitySnapshots() ([]*CodexCapacitySnapshot, error) { var rows []*CodexCapacitySnapshot; return rows, DB.Order("instance_id asc").Find(&rows).Error }
func ListCodexResetCredits(instanceID string) ([]*CodexResetCredit, error) { var rows []*CodexResetCredit; return rows, DB.Where("instance_id = ?", instanceID).Order("expires_at asc").Find(&rows).Error }

func GetCodexResetOperation(instanceID, key string) (*CodexResetOperation, error) {
	var row CodexResetOperation
	err := DB.Where("instance_id = ? AND idempotency_key = ?", instanceID, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return nil, nil }
	return &row, err
}

func GetActiveCodexResetOperation(instanceID string) (*CodexResetOperation, error) {
	var row CodexResetOperation
	err := DB.Where("instance_id = ? AND active_lock IS NOT NULL", instanceID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func CreateCodexResetOperation(row *CodexResetOperation) error { return DB.Create(row).Error }
func CompleteCodexResetOperation(row *CodexResetOperation, status string, upstreamStatus int) error {
	now := common.GetTimestamp()
	return DB.Model(&CodexResetOperation{}).Where("id = ?", row.ID).Updates(map[string]any{"status": status, "active_lock": nil, "upstream_status": upstreamStatus, "completed_at": now, "updated_at": now}).Error
}
func ListRecentCodexResetOperations(limit int) ([]*CodexResetOperation, error) { var rows []*CodexResetOperation; return rows, DB.Order("id desc").Limit(limit).Find(&rows).Error }
