package model_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlimoB2BGPTImageCatalogGate(t *testing.T) {
	t.Setenv(glimoB2BGPTImageEnabledEnv, "false")

	assert.True(t, IsModelBlockedForUserGroup("b2b", "gpt-image-1"))
	assert.True(t, IsModelBlockedForUserGroup("b2b", "gpt-image-2"))
	assert.False(t, IsModelBlockedForUserGroup("b2b", "gpt-5.6-sol"))
	assert.False(t, IsModelBlockedForUserGroup("default", "gpt-image-2"))

	require.Equal(t, []string{"gpt-5.6-sol", "deepseek-v4-pro"}, FilterModelsForUserGroup(
		"b2b",
		[]string{"gpt-image-1", "gpt-5.6-sol", "gpt-image-2", "deepseek-v4-pro"},
	))
}

func TestGlimoB2BGPTImageCatalogGateDefaultsToEnabled(t *testing.T) {
	t.Setenv(glimoB2BGPTImageEnabledEnv, "")
	assert.False(t, IsModelBlockedForUserGroup("b2b", "gpt-image-1"))
}
