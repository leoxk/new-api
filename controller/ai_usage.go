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
