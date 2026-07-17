package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestFilterPricingByEnabledModelsExcludesUnapprovedCatalogEntries(t *testing.T) {
	pricing := []model.Pricing{
		{ModelName: "gpt-5.6-sol"},
		{ModelName: "deepseek-v4-pro"},
		{ModelName: "codex-auto-review"},
		{ModelName: "dall-e-3"},
	}

	filtered := filterPricingByEnabledModels(pricing, []string{"gpt-5.6-sol", "deepseek-v4-pro"})
	assert.Equal(t, []model.Pricing{
		{ModelName: "gpt-5.6-sol"},
		{ModelName: "deepseek-v4-pro"},
	}, filtered)
}
