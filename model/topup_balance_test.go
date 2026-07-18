package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quotaForUSD(t *testing.T, amount int64) int {
	t.Helper()
	quota, clamp := common.QuotaFromDecimalChecked(decimal.NewFromInt(amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	require.Nil(t, clamp)
	return quota
}

func ensureWalletBalanceTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Redemption{}))
	require.NoError(t, DB.Exec("DELETE FROM redemptions").Error)
}

func seedSuccessfulTopUp(t *testing.T, userID int, tradeNo string, money float64, refundedMoney float64) {
	t.Helper()
	require.NoError(t, DB.Create(&TopUp{
		UserId:          userID,
		Amount:          int64(money),
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		Status:          common.TopUpStatusSuccess,
		RefundedMoney:   refundedMoney,
	}).Error)
}

func TestGetUserWalletBalanceDerivesPromotionalCreditFirst(t *testing.T) {
	ensureWalletBalanceTables(t)
	tests := []struct {
		name               string
		currentUSD         int64
		cashUSD            float64
		refundedUSD        float64
		wantRechargeUSD    int64
		wantPromotionalUSD int64
	}{
		{name: "cash only", currentUSD: 100, cashUSD: 100, wantRechargeUSD: 100},
		{name: "promotion only", currentUSD: 20, wantPromotionalUSD: 20},
		{name: "promotion remains above cash", currentUSD: 110, cashUSD: 100, wantRechargeUSD: 100, wantPromotionalUSD: 10},
		{name: "promotion consumed before cash", currentUSD: 70, cashUSD: 100, wantRechargeUSD: 70},
		{name: "completed refund reduces cash funding", currentUSD: 30, cashUSD: 100, refundedUSD: 80, wantRechargeUSD: 20, wantPromotionalUSD: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			user := &User{Username: "wallet-" + tt.name, Status: common.UserStatusEnabled, Quota: quotaForUSD(t, tt.currentUSD)}
			require.NoError(t, DB.Create(user).Error)
			if tt.cashUSD > 0 {
				seedSuccessfulTopUp(t, user.Id, "wallet-"+tt.name, tt.cashUSD, tt.refundedUSD)
			}

			balance, err := GetUserWalletBalance(user.Id, user.Quota)
			require.NoError(t, err)
			assert.Equal(t, quotaForUSD(t, tt.wantRechargeUSD), balance.RechargeQuota)
			assert.Equal(t, quotaForUSD(t, tt.wantPromotionalUSD), balance.PromotionalQuota)
			assert.Equal(t, user.Quota, balance.RechargeQuota+balance.PromotionalQuota)
		})
	}
}

func TestGetUserWalletBalanceTracksRedemptionBelowHistoricalCashFunding(t *testing.T) {
	ensureWalletBalanceTables(t)
	truncateTables(t)
	user := &User{
		Username:  "wallet-redemption-after-cash-usage",
		Status:    common.UserStatusEnabled,
		Quota:     quotaForUSD(t, 60),
		UsedQuota: quotaForUSD(t, 100),
	}
	require.NoError(t, DB.Create(user).Error)
	seedSuccessfulTopUp(t, user.Id, "wallet-redemption-after-cash-usage", 135, 0)
	require.NoError(t, DB.Create(&[]Redemption{
		{
			Key:                   "20000000000000000000000000000001",
			Name:                  "consumed promotion",
			Status:                common.RedemptionCodeStatusUsed,
			Quota:                 quotaForUSD(t, 25),
			RedeemedTime:          1,
			UsedUserId:            user.Id,
			UsedQuotaAtRedemption: quotaForUSD(t, 70),
		},
		{
			Key:                   "20000000000000000000000000000002",
			Name:                  "current promotion",
			Status:                common.RedemptionCodeStatusUsed,
			Quota:                 quotaForUSD(t, 25),
			RedeemedTime:          2,
			UsedUserId:            user.Id,
			UsedQuotaAtRedemption: quotaForUSD(t, 100),
		},
	}).Error)

	balance, err := GetUserWalletBalance(user.Id, user.Quota)
	require.NoError(t, err)
	assert.Equal(t, quotaForUSD(t, 35), balance.RechargeQuota)
	assert.Equal(t, quotaForUSD(t, 25), balance.PromotionalQuota)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":      quotaForUSD(t, 50),
		"used_quota": quotaForUSD(t, 110),
	}).Error)
	balance, err = GetUserWalletBalance(user.Id, quotaForUSD(t, 50))
	require.NoError(t, err)
	assert.Equal(t, quotaForUSD(t, 35), balance.RechargeQuota)
	assert.Equal(t, quotaForUSD(t, 15), balance.PromotionalQuota)
}

func TestBackfillB2BRedemptionUsageSnapshotsKeepsOnlyNewestLegacyCredit(t *testing.T) {
	ensureWalletBalanceTables(t)
	truncateTables(t)
	b2bUser := &User{
		Username:  "legacy-b2b-redemption",
		Group:     "b2b",
		AffCode:   "legacy-b2b",
		Status:    common.UserStatusEnabled,
		UsedQuota: quotaForUSD(t, 100),
	}
	defaultUser := &User{
		Username:  "legacy-default-redemption",
		Group:     "default",
		AffCode:   "legacy-default",
		Status:    common.UserStatusEnabled,
		UsedQuota: quotaForUSD(t, 40),
	}
	require.NoError(t, DB.Create(b2bUser).Error)
	require.NoError(t, DB.Create(defaultUser).Error)
	require.NoError(t, DB.Create(&[]Redemption{
		{Key: "30000000000000000000000000000001", Status: common.RedemptionCodeStatusUsed, UsedUserId: b2bUser.Id, RedeemedTime: 1, UsedQuotaAtRedemption: -1},
		{Key: "30000000000000000000000000000002", Status: common.RedemptionCodeStatusUsed, UsedUserId: b2bUser.Id, RedeemedTime: 2, UsedQuotaAtRedemption: -1},
		{Key: "30000000000000000000000000000003", Status: common.RedemptionCodeStatusUsed, UsedUserId: defaultUser.Id, RedeemedTime: 3, UsedQuotaAtRedemption: -1},
	}).Error)

	require.NoError(t, backfillB2BRedemptionUsageSnapshots())

	var b2bRedemptions []Redemption
	require.NoError(t, DB.Where("used_user_id = ?", b2bUser.Id).Order("redeemed_time asc").Find(&b2bRedemptions).Error)
	require.Len(t, b2bRedemptions, 2)
	assert.Equal(t, ignoredLegacyRedemptionUsageSnapshot, b2bRedemptions[0].UsedQuotaAtRedemption)
	assert.Equal(t, b2bUser.UsedQuota, b2bRedemptions[1].UsedQuotaAtRedemption)

	var unchangedDefault Redemption
	require.NoError(t, DB.Where("used_user_id = ?", defaultUser.Id).First(&unchangedDefault).Error)
	assert.Equal(t, -1, unchangedDefault.UsedQuotaAtRedemption)
}

func TestRecordCompletedTopUpRefundUpdatesOrderAndQuota(t *testing.T) {
	ensureWalletBalanceTables(t)
	truncateTables(t)
	user := &User{Username: "refund-user", Status: common.UserStatusEnabled, Quota: quotaForUSD(t, 110)}
	require.NoError(t, DB.Create(user).Error)
	seedSuccessfulTopUp(t, user.Id, "refund-order", 100, 0)

	result, err := RecordCompletedTopUpRefund("refund-order", decimal.NewFromInt(100), "re_123", "customer requested unused cash refund")
	require.NoError(t, err)
	assert.Equal(t, 100.0, result.RefundedMoney)
	assert.Equal(t, "re_123", result.ProviderRefundId)
	assert.NotZero(t, result.RefundRecordedAt)
	assert.Equal(t, "customer requested unused cash refund", result.RefundReason)

	var updated User
	require.NoError(t, DB.First(&updated, user.Id).Error)
	assert.Equal(t, quotaForUSD(t, 10), updated.Quota)

	balance, err := GetUserWalletBalance(user.Id, updated.Quota)
	require.NoError(t, err)
	assert.Zero(t, balance.RechargeQuota)
	assert.Equal(t, quotaForUSD(t, 10), balance.PromotionalQuota)
}

func TestRecordCompletedTopUpRefundRejectsAmountAboveRechargeBalance(t *testing.T) {
	ensureWalletBalanceTables(t)
	truncateTables(t)
	user := &User{Username: "refund-limit-user", Status: common.UserStatusEnabled, Quota: quotaForUSD(t, 30)}
	require.NoError(t, DB.Create(user).Error)
	seedSuccessfulTopUp(t, user.Id, "refund-limit-order", 100, 0)

	_, err := RecordCompletedTopUpRefund("refund-limit-order", decimal.NewFromInt(40), "re_limit", "too large")
	require.ErrorIs(t, err, ErrRefundExceedsRechargeBalance)

	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", "refund-limit-order").First(&topUp).Error)
	assert.Zero(t, topUp.RefundedMoney)
}

func TestRecordCompletedTopUpRefundRejectsSecondRefundForOrder(t *testing.T) {
	ensureWalletBalanceTables(t)
	truncateTables(t)
	user := &User{Username: "refund-once-user", Status: common.UserStatusEnabled, Quota: quotaForUSD(t, 100)}
	require.NoError(t, DB.Create(user).Error)
	seedSuccessfulTopUp(t, user.Id, "refund-once-order", 100, 0)

	_, err := RecordCompletedTopUpRefund("refund-once-order", decimal.NewFromInt(10), "re_first", "first refund")
	require.NoError(t, err)
	_, err = RecordCompletedTopUpRefund("refund-once-order", decimal.NewFromInt(5), "re_second", "second refund")
	require.ErrorIs(t, err, ErrTopUpAlreadyRefunded)
}

func TestStripeRechargeVerifiesAmountAndCurrencyBeforeCrediting(t *testing.T) {
	tests := []struct {
		name      string
		paidMinor int64
		currency  string
		wantErr   error
	}{
		{name: "amount mismatch", paidMinor: 9900, currency: "USD", wantErr: ErrTopUpPaymentAmountMismatch},
		{name: "currency mismatch", paidMinor: 10000, currency: "EUR", wantErr: ErrTopUpPaymentCurrencyMismatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			user := &User{Username: "stripe-" + tt.name, Status: common.UserStatusEnabled}
			require.NoError(t, DB.Create(user).Error)
			require.NoError(t, DB.Create(&TopUp{
				UserId:          user.Id,
				Amount:          100,
				Money:           100,
				TradeNo:         "stripe-" + tt.name,
				PaymentMethod:   PaymentMethodStripe,
				PaymentProvider: PaymentProviderStripe,
				Status:          common.TopUpStatusPending,
			}).Error)

			err := CompleteCashTopUp("stripe-"+tt.name, PaymentProviderStripe, "cus_123", "127.0.0.1", tt.paidMinor, tt.currency)
			require.ErrorIs(t, err, tt.wantErr)

			var updated User
			require.NoError(t, DB.First(&updated, user.Id).Error)
			assert.Zero(t, updated.Quota)
			var topUp TopUp
			require.NoError(t, DB.Where("trade_no = ?", "stripe-"+tt.name).First(&topUp).Error)
			assert.Equal(t, common.TopUpStatusPending, topUp.Status)
		})
	}
}

func TestCompleteCashTopUpIsIdempotentForDuplicateProviderEvents(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "paypal-idempotent", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(user).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:          user.Id,
		Amount:          10,
		Money:           10,
		TradeNo:         "paypal-order-123",
		PaymentMethod:   PaymentMethodPayPal,
		PaymentProvider: PaymentProviderPayPal,
		Status:          common.TopUpStatusPending,
	}).Error)

	require.NoError(t, CompleteCashTopUp("paypal-order-123", PaymentProviderPayPal, "", "127.0.0.1", 1000, "USD"))
	require.NoError(t, CompleteCashTopUp("paypal-order-123", PaymentProviderPayPal, "", "127.0.0.1", 1000, "USD"))

	var updated User
	require.NoError(t, DB.First(&updated, user.Id).Error)
	assert.Equal(t, quotaForUSD(t, 10), updated.Quota)
}
