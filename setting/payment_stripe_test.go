package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripeRuntimeEnvironmentOverridesStoredSettings(t *testing.T) {
	originalAPISecret := StripeApiSecret
	originalWebhookSecret := StripeWebhookSecret
	originalPriceID := StripePriceId
	t.Cleanup(func() {
		StripeApiSecret = originalAPISecret
		StripeWebhookSecret = originalWebhookSecret
		StripePriceId = originalPriceID
	})
	StripeApiSecret = "stored-api"
	StripeWebhookSecret = "stored-webhook"
	StripePriceId = "stored-price"
	t.Setenv("STRIPE_API_SECRET", "runtime-api")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "runtime-webhook")
	t.Setenv("STRIPE_PRICE_ID", "runtime-price")

	assert.Equal(t, "runtime-api", GetStripeAPISecret())
	assert.Equal(t, "runtime-webhook", GetStripeWebhookSecret())
	assert.Equal(t, "runtime-price", GetStripePriceID())
}
