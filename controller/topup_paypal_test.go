package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPayPalPaidMinorUnitsUsesOnlyCompletedCapture(t *testing.T) {
	order := paypalOrder{PurchaseUnits: []paypalPurchaseUnit{{
		Payments: &struct {
			Captures []paypalCapture `json:"captures"`
		}{Captures: []paypalCapture{
			{Status: "PENDING", Amount: paypalAmount{CurrencyCode: "USD", Value: "99.00"}},
			{Status: "COMPLETED", Amount: paypalAmount{CurrencyCode: "usd", Value: "10.25"}},
		}},
	}}}

	minor, currency, err := paypalPaidMinorUnits(order)
	require.NoError(t, err)
	assert.Equal(t, int64(1025), minor)
	assert.Equal(t, "USD", currency)
}

func TestPayPalPaidMinorUnitsRejectsUnsupportedPrecision(t *testing.T) {
	order := paypalOrder{PurchaseUnits: []paypalPurchaseUnit{{
		Payments: &struct {
			Captures []paypalCapture `json:"captures"`
		}{Captures: []paypalCapture{
			{Status: "COMPLETED", Amount: paypalAmount{CurrencyCode: "USD", Value: "10.001"}},
		}},
	}}}

	_, _, err := paypalPaidMinorUnits(order)
	require.Error(t, err)
}
