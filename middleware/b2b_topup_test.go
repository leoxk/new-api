package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupB2BTopUpMiddlewareDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
}

func performB2BTopUpRequest(t *testing.T, group string) *httptest.ResponseRecorder {
	t.Helper()
	user := &model.User{
		Username: "wallet-" + group,
		Group:    group,
		Status:   common.UserStatusEnabled,
		AffCode:  "aff-" + group,
	}
	require.NoError(t, model.DB.Create(user).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", user.Id)
		c.Next()
	})
	router.GET("/api/user/topup/info", B2BTopUpOnly(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil))
	return recorder
}

func TestB2BTopUpOnlyAllowsApprovedGroup(t *testing.T) {
	setupB2BTopUpMiddlewareDB(t)
	require.Equal(t, http.StatusOK, performB2BTopUpRequest(t, "b2b").Code)
}

func TestB2BTopUpOnlyRejectsDefaultGroup(t *testing.T) {
	setupB2BTopUpMiddlewareDB(t)
	recorder := performB2BTopUpRequest(t, "default")
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "B2B approval is required")
}
