package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type b2bAPIResponse struct {
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func setupB2BControllerTestDB(t *testing.T) {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.Log{}, &model.Redemption{}))
}

func invokeController(t *testing.T, method string, body interface{}, handler gin.HandlerFunc, values map[string]interface{}) b2bAPIResponse {
	t.Helper()
	var requestBody []byte
	if body != nil {
		var err error
		requestBody, err = common.Marshal(body)
		require.NoError(t, err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, "/", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	for key, value := range values {
		ctx.Set(key, value)
	}
	handler(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response b2bAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func TestGetSelfReturnsDerivedBalancesOnlyForB2B(t *testing.T) {
	setupB2BControllerTestDB(t)

	b2bUser := &model.User{Username: "b2b-wallet", Group: "b2b", Status: common.UserStatusEnabled, Quota: int(100 * common.QuotaPerUnit), AffCode: "b2b1"}
	require.NoError(t, model.DB.Create(b2bUser).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: b2bUser.Id, Money: 60, TradeNo: "b2b-cash", Status: common.TopUpStatusSuccess,
	}).Error)
	response := invokeController(t, http.MethodGet, nil, GetSelf, map[string]interface{}{
		"id": b2bUser.Id, "role": common.RoleCommonUser,
	})
	require.True(t, response.Success)
	assert.Contains(t, response.Data, "recharge_quota")
	assert.Contains(t, response.Data, "promotional_quota")
	assert.Equal(t, response.Data["quota"], response.Data["recharge_quota"].(float64)+response.Data["promotional_quota"].(float64))

	defaultUser := &model.User{Username: "default-wallet", Group: "default", Status: common.UserStatusEnabled, Quota: 1000, AffCode: "def1"}
	require.NoError(t, model.DB.Create(defaultUser).Error)
	response = invokeController(t, http.MethodGet, nil, GetSelf, map[string]interface{}{
		"id": defaultUser.Id, "role": common.RoleCommonUser,
	})
	require.True(t, response.Success)
	assert.NotContains(t, response.Data, "recharge_quota")
	assert.NotContains(t, response.Data, "promotional_quota")
}

func TestAdminRecordCompletedTopUpRefundRecordsAudit(t *testing.T) {
	setupB2BControllerTestDB(t)
	user := &model.User{Username: "refund-customer", Group: "b2b", Status: common.UserStatusEnabled, Quota: int(100 * common.QuotaPerUnit), AffCode: "ref1"}
	require.NoError(t, model.DB.Create(user).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: user.Id, Money: 100, TradeNo: "refund-controller", Status: common.TopUpStatusSuccess,
	}).Error)

	response := invokeController(t, http.MethodPost, AdminRecordCompletedTopUpRefundRequest{
		TradeNo:          "refund-controller",
		RefundedMoney:    "10.00",
		ProviderRefundId: "re_controller",
		Reason:           "unused cash",
	}, AdminRecordCompletedTopUpRefund, map[string]interface{}{
		"id": 999, "role": common.RoleRootUser, "username": "operator",
	})
	require.True(t, response.Success, response.Message)

	var topUp model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", "refund-controller").First(&topUp).Error)
	assert.Equal(t, 10.0, topUp.RefundedMoney)
	assert.Equal(t, "re_controller", topUp.ProviderRefundId)

	var audit model.Log
	require.NoError(t, model.DB.Where("type = ?", model.LogTypeManage).First(&audit).Error)
	assert.Contains(t, audit.Content, "re_controller")
	assert.Contains(t, audit.Other, "topup.refund_record")
}

func TestAdminRecordCompletedTopUpRefundRejectsInvalidAmount(t *testing.T) {
	setupB2BControllerTestDB(t)
	response := invokeController(t, http.MethodPost, AdminRecordCompletedTopUpRefundRequest{
		TradeNo:          "invalid-refund",
		RefundedMoney:    "1.001",
		ProviderRefundId: "re_invalid",
		Reason:           "invalid precision",
	}, AdminRecordCompletedTopUpRefund, map[string]interface{}{"id": 999})
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "两位小数")
}
