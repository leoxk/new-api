package setting

import (
	"os"
	"strings"
)

var StripeApiSecret = ""
var StripeWebhookSecret = ""
var StripePriceId = ""
var StripeUnitPrice = 8.0
var StripeMinTopUp = 1
var StripePromotionCodesEnabled = false

func GetStripeAPISecret() string {
	if value := strings.TrimSpace(os.Getenv("STRIPE_API_SECRET")); value != "" {
		return value
	}
	return StripeApiSecret
}

func GetStripeWebhookSecret() string {
	if value := strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")); value != "" {
		return value
	}
	return StripeWebhookSecret
}

func GetStripePriceID() string {
	if value := strings.TrimSpace(os.Getenv("STRIPE_PRICE_ID")); value != "" {
		return value
	}
	return StripePriceId
}
