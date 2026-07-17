package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAIUsageRouteAllowsAnyOriginAndRequiresUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	preflight := httptest.NewRequest(http.MethodOptions, "/internal-api/ai-usage?username=leo", nil)
	preflight.Header.Set("Origin", "https://game-engine.com")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflightRecorder := httptest.NewRecorder()
	engine.ServeHTTP(preflightRecorder, preflight)
	require.Equal(t, http.StatusNoContent, preflightRecorder.Code)
	assert.Equal(t, "*", preflightRecorder.Header().Get("Access-Control-Allow-Origin"))

	request := httptest.NewRequest(http.MethodGet, "/internal-api/ai-usage", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.JSONEq(t, `{"error":"username must be provided exactly once"}`, recorder.Body.String())
}
