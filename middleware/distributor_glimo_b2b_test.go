package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistributeRejectsBlockedB2BGPTImageBeforeUpstreamSelection(t *testing.T) {
	t.Setenv("GLIMO_B2B_GPT_IMAGE_ENABLED", "false")
	gin.SetMode(gin.TestMode)

	called := false
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserGroup, "b2b")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/images/generations", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/images/generations",
		strings.NewReader(`{"model":"gpt-image-2","prompt":"test"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	assert.False(t, called)
	assert.Contains(t, recorder.Body.String(), "model_not_found")
	assert.Contains(t, recorder.Body.String(), "not available for this customer group")
}
