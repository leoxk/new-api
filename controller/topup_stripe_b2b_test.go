package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stripeWebhookTestSecret = "whsec_glimo_b2b_test"

func configureStripeWebhookTest(t *testing.T) {
	t.Helper()
	confirmPaymentComplianceForTest(t)

	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	originalPriceID := setting.StripePriceId
	originalMinTopUp := setting.StripeMinTopUp
	originalUnitPrice := setting.StripeUnitPrice
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
		setting.StripePriceId = originalPriceID
		setting.StripeMinTopUp = originalMinTopUp
		setting.StripeUnitPrice = originalUnitPrice
	})

	t.Setenv("STRIPE_API_SECRET", "sk_test_glimo_b2b")
	t.Setenv("STRIPE_WEBHOOK_SECRET", stripeWebhookTestSecret)
	t.Setenv("STRIPE_PRICE_ID", "price_glimo_b2b_test")
	setting.StripeApiSecret = "sk_test_glimo_b2b"
	setting.StripeWebhookSecret = stripeWebhookTestSecret
	setting.StripePriceId = "price_glimo_b2b_test"
	setting.StripeMinTopUp = 10
	setting.StripeUnitPrice = 1
}

func seedStripeWebhookOrder(t *testing.T, tradeNo string, amount float64) *model.User {
	t.Helper()
	user := &model.User{
		Username: "stripe-webhook-" + tradeNo,
		Group:    "b2b",
		Status:   common.UserStatusEnabled,
		AffCode:  "aff-" + tradeNo,
	}
	require.NoError(t, model.DB.Create(user).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:          user.Id,
		Amount:          int64(amount),
		Money:           amount,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		Status:          common.TopUpStatusPending,
	}).Error)
	return user
}

func stripeCheckoutEventPayload(t *testing.T, eventID string, eventType stripe.EventType, tradeNo string, fields map[string]interface{}) []byte {
	t.Helper()
	object := map[string]interface{}{
		"id":                  "cs_test_" + eventID,
		"object":              "checkout.session",
		"client_reference_id": tradeNo,
		"customer":            "cus_glimo_b2b_test",
		"status":              "complete",
		"payment_status":      "paid",
		"amount_total":        2000,
		"currency":            "usd",
	}
	for key, value := range fields {
		object[key] = value
	}
	payload, err := common.Marshal(map[string]interface{}{
		"id":          eventID,
		"object":      "event",
		"api_version": stripe.APIVersion,
		"type":        eventType,
		"data": map[string]interface{}{
			"object": object,
		},
	})
	require.NoError(t, err)
	return payload
}

func invokeSignedStripeWebhook(t *testing.T, payload []byte, secret string) *httptest.ResponseRecorder {
	t.Helper()
	signedPayload := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  secret,
	})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", bytes.NewReader(signedPayload.Payload))
	ctx.Request.Header.Set("Stripe-Signature", signedPayload.Header)
	StripeWebhook(ctx)
	return recorder
}

func invokeStripeAmountRequest(t *testing.T, userID int, amount int64) string {
	t.Helper()
	payload, err := common.Marshal(StripePayRequest{Amount: amount, PaymentMethod: model.PaymentMethodStripe})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/amount", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userID)
	RequestStripeAmount(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	return recorder.Body.String()
}

func TestStripeWebhookRejectsInvalidSignature(t *testing.T) {
	setupB2BControllerTestDB(t)
	configureStripeWebhookTest(t)
	user := seedStripeWebhookOrder(t, "ref_invalid_signature", 20)
	payload := stripeCheckoutEventPayload(t, "evt_invalid_signature", stripe.EventTypeCheckoutSessionCompleted, "ref_invalid_signature", nil)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", bytes.NewReader(payload))
	ctx.Request.Header.Set("Stripe-Signature", "t=1,v1=invalid")
	StripeWebhook(ctx)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	require.NoError(t, model.DB.First(user, user.Id).Error)
	assert.Zero(t, user.Quota)
}

func TestStripeWebhookCompletedIsIdempotent(t *testing.T) {
	setupB2BControllerTestDB(t)
	configureStripeWebhookTest(t)
	user := seedStripeWebhookOrder(t, "ref_completed", 20)
	payload := stripeCheckoutEventPayload(t, "evt_completed", stripe.EventTypeCheckoutSessionCompleted, "ref_completed", nil)

	assert.Equal(t, http.StatusOK, invokeSignedStripeWebhook(t, payload, stripeWebhookTestSecret).Code)
	assert.Equal(t, http.StatusOK, invokeSignedStripeWebhook(t, payload, stripeWebhookTestSecret).Code)

	require.NoError(t, model.DB.First(user, user.Id).Error)
	assert.Equal(t, int(20*common.QuotaPerUnit), user.Quota)
	assert.Equal(t, "cus_glimo_b2b_test", user.StripeCustomer)
	var topUp model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", "ref_completed").First(&topUp).Error)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
}

func TestStripeWebhookDelayedPaymentLifecycle(t *testing.T) {
	setupB2BControllerTestDB(t)
	configureStripeWebhookTest(t)
	user := seedStripeWebhookOrder(t, "ref_delayed", 20)

	completedPayload := stripeCheckoutEventPayload(t, "evt_delayed_pending", stripe.EventTypeCheckoutSessionCompleted, "ref_delayed", map[string]interface{}{
		"payment_status": "unpaid",
	})
	assert.Equal(t, http.StatusOK, invokeSignedStripeWebhook(t, completedPayload, stripeWebhookTestSecret).Code)
	require.NoError(t, model.DB.First(user, user.Id).Error)
	assert.Zero(t, user.Quota)
	var pending model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", "ref_delayed").First(&pending).Error)
	assert.Equal(t, common.TopUpStatusPending, pending.Status)

	succeededPayload := stripeCheckoutEventPayload(t, "evt_delayed_succeeded", stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded, "ref_delayed", nil)
	assert.Equal(t, http.StatusOK, invokeSignedStripeWebhook(t, succeededPayload, stripeWebhookTestSecret).Code)
	require.NoError(t, model.DB.First(user, user.Id).Error)
	assert.Equal(t, int(20*common.QuotaPerUnit), user.Quota)
	var completed model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", "ref_delayed").First(&completed).Error)
	assert.Equal(t, common.TopUpStatusSuccess, completed.Status)
}

func TestStripeWebhookAsyncFailureAndExpirationDoNotCredit(t *testing.T) {
	tests := []struct {
		name       string
		tradeNo    string
		eventType  stripe.EventType
		fields     map[string]interface{}
		wantStatus string
	}{
		{
			name:       "async failure",
			tradeNo:    "ref_async_failed",
			eventType:  stripe.EventTypeCheckoutSessionAsyncPaymentFailed,
			wantStatus: common.TopUpStatusFailed,
		},
		{
			name:       "expired checkout",
			tradeNo:    "ref_expired",
			eventType:  stripe.EventTypeCheckoutSessionExpired,
			fields:     map[string]interface{}{"status": "expired", "payment_status": "unpaid"},
			wantStatus: common.TopUpStatusExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupB2BControllerTestDB(t)
			configureStripeWebhookTest(t)
			user := seedStripeWebhookOrder(t, tt.tradeNo, 20)
			payload := stripeCheckoutEventPayload(t, "evt_"+tt.tradeNo, tt.eventType, tt.tradeNo, tt.fields)

			assert.Equal(t, http.StatusOK, invokeSignedStripeWebhook(t, payload, stripeWebhookTestSecret).Code)
			require.NoError(t, model.DB.First(user, user.Id).Error)
			assert.Zero(t, user.Quota)
			var topUp model.TopUp
			require.NoError(t, model.DB.Where("trade_no = ?", tt.tradeNo).First(&topUp).Error)
			assert.Equal(t, tt.wantStatus, topUp.Status)
		})
	}
}

func TestStripeWebhookRejectsAmountAndCurrencyMismatchWithoutCrediting(t *testing.T) {
	tests := []struct {
		name    string
		tradeNo string
		fields  map[string]interface{}
	}{
		{name: "amount mismatch", tradeNo: "ref_amount_mismatch", fields: map[string]interface{}{"amount_total": 1900}},
		{name: "currency mismatch", tradeNo: "ref_currency_mismatch", fields: map[string]interface{}{"currency": "eur"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupB2BControllerTestDB(t)
			configureStripeWebhookTest(t)
			user := seedStripeWebhookOrder(t, tt.tradeNo, 20)
			payload := stripeCheckoutEventPayload(t, "evt_"+tt.tradeNo, stripe.EventTypeCheckoutSessionCompleted, tt.tradeNo, tt.fields)

			assert.Equal(t, http.StatusOK, invokeSignedStripeWebhook(t, payload, stripeWebhookTestSecret).Code)
			require.NoError(t, model.DB.First(user, user.Id).Error)
			assert.Zero(t, user.Quota)
			var topUp model.TopUp
			require.NoError(t, model.DB.Where("trade_no = ?", tt.tradeNo).First(&topUp).Error)
			assert.Equal(t, common.TopUpStatusPending, topUp.Status)
		})
	}
}

func TestStripeMinimumTopUpRejectsNineAndAcceptsTen(t *testing.T) {
	setupB2BControllerTestDB(t)
	configureStripeWebhookTest(t)
	user := &model.User{Username: "stripe-minimum", Group: "b2b", Status: common.UserStatusEnabled, AffCode: "stripe-minimum"}
	require.NoError(t, model.DB.Create(user).Error)

	rejected := invokeStripeAmountRequest(t, user.Id, 9)
	assert.Contains(t, rejected, `"message":"error"`)
	assert.Contains(t, rejected, "10")

	accepted := invokeStripeAmountRequest(t, user.Id, 10)
	assert.Contains(t, accepted, `"message":"success"`)
	assert.Contains(t, accepted, `"10.00"`)
}
