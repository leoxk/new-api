package controller

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func GetAIUsage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	values, ok := c.GetQueryArray("username")
	if !ok || len(values) != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username must be provided exactly once"})
		return
	}
	username := strings.TrimSpace(values[0])
	if username == "" || utf8.RuneCountInString(username) > model.UserNameMaxLength {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is invalid"})
		return
	}

	usage, err := service.GetAIUsage(username, time.Now())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		common.SysError("get AI usage failed: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "usage data unavailable"})
		return
	}
	c.JSON(http.StatusOK, usage)
}

func AIUsageOptions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusNoContent)
}

func GetCodexCapacity(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	capacity, err := service.GetCodexCapacity(time.Now())
	if err != nil {
		common.SysError("get Codex capacity failed: " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Codex capacity unavailable"})
		return
	}
	c.JSON(http.StatusOK, capacity)
}

func GetAdminCodexCapacity(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	capacity, err := service.GetAdminCodexCapacity(time.Now())
	if err != nil {
		common.SysError("get admin Codex capacity failed: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Codex capacity unavailable"})
		return
	}
	c.JSON(http.StatusOK, capacity)
}

func ResetAdminCodexCapacity(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var body struct { Confirm bool `json:"confirm"` }
	if err := c.ShouldBindJSON(&body); err != nil || !body.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirm must be true"})
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || utf8.RuneCountInString(key) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a valid Idempotency-Key is required"})
		return
	}
	op, err := service.UseCodexResetCredit(c.Request.Context(), c.Param("instance_id"), c.GetInt("id"), key)
	if err != nil {
		if errors.Is(err, service.ErrNoCodexResetCredit) {
			c.JSON(http.StatusConflict, gin.H{"error": "no reset credit is available"})
			return
		}
		if errors.Is(err, service.ErrCodexResetInProgress) {
			c.JSON(http.StatusConflict, gin.H{"error": "a reset operation is already pending for this OAuth instance"})
			return
		}
		common.SysError("use Codex reset credit failed: " + err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": "Codex reset could not be completed", "operation": op})
		return
	}
	if op.Status == model.CodexResetOperationUncertain || op.Status == model.CodexResetOperationPending {
		c.JSON(http.StatusAccepted, op)
		return
	}
	c.JSON(http.StatusOK, op)
}
