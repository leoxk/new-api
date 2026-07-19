package model_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const glimoB2BGPTImageEnabledEnv = "GLIMO_B2B_GPT_IMAGE_ENABLED"

// IsModelBlockedForUserGroup applies the Glimo B2B commercial catalog gate.
// It is intentionally based on the customer's user group, not the internal
// route group, so auto-routed DeepSeek requests continue to work while GPT
// Image can be withheld without changing or disabling its production channel.
func IsModelBlockedForUserGroup(userGroup string, modelName string) bool {
	return userGroup == "b2b" &&
		strings.HasPrefix(modelName, "gpt-image-") &&
		!common.GetEnvOrDefaultBool(glimoB2BGPTImageEnabledEnv, true)
}

func FilterModelsForUserGroup(userGroup string, models []string) []string {
	filtered := make([]string, 0, len(models))
	for _, modelName := range models {
		if !IsModelBlockedForUserGroup(userGroup, modelName) {
			filtered = append(filtered, modelName)
		}
	}
	return filtered
}
