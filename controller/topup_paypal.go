package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const paypalCurrency = "USD"

type paypalOrderRequest struct {
	Intent             string                   `json:"intent"`
	PurchaseUnits      []paypalPurchaseUnit     `json:"purchase_units"`
	ApplicationContext paypalApplicationContext `json:"application_context"`
}

type paypalPurchaseUnit struct {
	CustomID string       `json:"custom_id,omitempty"`
	Amount   paypalAmount `json:"amount"`
	Payments *struct {
		Captures []paypalCapture `json:"captures"`
	} `json:"payments,omitempty"`
}

type paypalAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type paypalCapture struct {
	Status string       `json:"status"`
	Amount paypalAmount `json:"amount"`
}

type paypalApplicationContext struct {
	ReturnURL  string `json:"return_url"`
	CancelURL  string `json:"cancel_url"`
	UserAction string `json:"user_action"`
}

type paypalOrder struct {
	ID            string               `json:"id"`
	Status        string               `json:"status"`
	PurchaseUnits []paypalPurchaseUnit `json:"purchase_units"`
	Links         []struct {
		Href string `json:"href"`
		Rel  string `json:"rel"`
	} `json:"links"`
}

type paypalTokenResponse struct {
	AccessToken string `json:"access_token"`
}

func paypalConfig() (baseURL, clientID, clientSecret, webhookID string, ok bool) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("PAYPAL_MODE")))
	baseURL = "https://api-m.sandbox.paypal.com"
	if mode == "live" {
		baseURL = "https://api-m.paypal.com"
	} else if mode != "sandbox" {
		return "", "", "", "", false
	}
	clientID = strings.TrimSpace(os.Getenv("PAYPAL_CLIENT_ID"))
	clientSecret = strings.TrimSpace(os.Getenv("PAYPAL_CLIENT_SECRET"))
	webhookID = strings.TrimSpace(os.Getenv("PAYPAL_WEBHOOK_ID"))
	ok = clientID != "" && clientSecret != "" && webhookID != ""
	return
}

func isPayPalTopUpEnabled() bool {
	_, _, _, _, configured := paypalConfig()
	return isPaymentComplianceConfirmed() && configured
}

func paypalTopUpMoney(amount int64, group string) decimal.Decimal {
	money := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		money = money.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	topupRatio := common.GetTopupGroupRatio(group)
	if topupRatio == 0 {
		topupRatio = 1
	}
	return money.Mul(decimal.NewFromFloat(topupRatio)).Round(2)
}

func paypalAccessToken(ctx context.Context) (string, error) {
	baseURL, clientID, clientSecret, _, ok := paypalConfig()
	if !ok {
		return "", errors.New("PayPal is not configured")
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var result paypalTokenResponse
	if err := paypalDo(req, &result); err != nil {
		return "", err
	}
	if result.AccessToken == "" {
		return "", errors.New("PayPal returned an empty access token")
	}
	return result.AccessToken, nil
}

func paypalDo(req *http.Request, out interface{}) error {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("PayPal API returned HTTP %d", resp.StatusCode)
	}
	if out != nil && len(body) > 0 {
		return common.Unmarshal(body, out)
	}
	return nil
}

func paypalJSONRequest(ctx context.Context, method, path, requestID string, payload, out interface{}) error {
	baseURL, _, _, _, ok := paypalConfig()
	if !ok {
		return errors.New("PayPal is not configured")
	}
	token, err := paypalAccessToken(ctx)
	if err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		encoded, err := common.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if requestID != "" {
		req.Header.Set("PayPal-Request-Id", requestID)
	}
	return paypalDo(req, out)
}

func RequestPayPalPay(c *gin.Context) {
	if !isPayPalTopUpEnabled() {
		common.ApiErrorMsg(c, "PayPal is not available")
		return
	}
	var req AmountRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount < getMinTopup() || req.Amount > 10000 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	userID := c.GetInt("id")
	group, err := model.GetUserGroup(userID, true)
	if err != nil || group != "b2b" {
		common.ApiErrorMsg(c, "PayPal is available only to approved B2B customers")
		return
	}
	money := paypalTopUpMoney(req.Amount, group)
	localReference := fmt.Sprintf("paypal-%d-%d-%s", userID, time.Now().UnixMilli(), common.GetRandomString(6))
	callbackBase := service.GetCallbackAddress()
	orderRequest := paypalOrderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []paypalPurchaseUnit{{
			CustomID: localReference,
			Amount:   paypalAmount{CurrencyCode: paypalCurrency, Value: money.StringFixed(2)},
		}},
		ApplicationContext: paypalApplicationContext{
			ReturnURL:  callbackBase + "/api/paypal/return",
			CancelURL:  paymentReturnPath("/console/topup?pay=cancelled"),
			UserAction: "PAY_NOW",
		},
	}
	var order paypalOrder
	if err := paypalJSONRequest(c.Request.Context(), http.MethodPost, "/v2/checkout/orders", localReference, orderRequest, &order); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("PayPal create order failed user_id=%d error=%q", userID, err.Error()))
		common.ApiErrorMsg(c, "创建 PayPal 订单失败")
		return
	}
	approvalURL := ""
	for _, link := range order.Links {
		if link.Rel == "approve" {
			approvalURL = link.Href
			break
		}
	}
	if order.ID == "" || approvalURL == "" {
		common.ApiErrorMsg(c, "PayPal 未返回有效的批准链接")
		return
	}
	topUp := &model.TopUp{
		UserId: userID, Amount: req.Amount, Money: money.InexactFloat64(), TradeNo: order.ID,
		PaymentMethod: model.PaymentMethodPayPal, PaymentProvider: model.PaymentProviderPayPal,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("PayPal local order insert failed user_id=%d order_id=%s error=%q", userID, order.ID, err.Error()))
		common.ApiErrorMsg(c, "创建本地充值订单失败")
		return
	}
	common.ApiSuccess(c, gin.H{"pay_link": approvalURL})
}

func paypalPaidMinorUnits(order paypalOrder) (int64, string, error) {
	for _, unit := range order.PurchaseUnits {
		if unit.Payments == nil {
			continue
		}
		for _, capture := range unit.Payments.Captures {
			if capture.Status != "COMPLETED" {
				continue
			}
			amount, err := decimal.NewFromString(capture.Amount.Value)
			if err != nil {
				return 0, "", err
			}
			minor := amount.Mul(decimal.NewFromInt(100))
			if !minor.Equal(minor.Truncate(0)) {
				return 0, "", errors.New("PayPal amount has unsupported precision")
			}
			return minor.IntPart(), strings.ToUpper(capture.Amount.CurrencyCode), nil
		}
	}
	return 0, "", errors.New("PayPal order has no completed capture")
}

func captureAndSettlePayPal(ctx context.Context, orderID, callerIP string) error {
	LockOrder(orderID)
	defer UnlockOrder(orderID)
	var order paypalOrder
	err := paypalJSONRequest(ctx, http.MethodPost, "/v2/checkout/orders/"+url.PathEscape(orderID)+"/capture", "capture-"+orderID, map[string]interface{}{}, &order)
	if err != nil {
		// A webhook may arrive after the return handler captured the order. Fetching
		// the canonical order keeps duplicate delivery idempotent.
		err = paypalJSONRequest(ctx, http.MethodGet, "/v2/checkout/orders/"+url.PathEscape(orderID), "", nil, &order)
	}
	if err != nil || order.Status != "COMPLETED" {
		return errors.New("PayPal order is not completed")
	}
	paidMinor, currency, err := paypalPaidMinorUnits(order)
	if err != nil {
		return err
	}
	return model.CompleteCashTopUp(orderID, model.PaymentProviderPayPal, "", callerIP, paidMinor, currency)
}

func PayPalReturn(c *gin.Context) {
	orderID := strings.TrimSpace(c.Query("token"))
	if orderID == "" {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}
	if err := captureAndSettlePayPal(c.Request.Context(), orderID, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("PayPal capture failed order_id=%s error=%q", orderID, err.Error()))
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}
	c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=success&show_history=true"))
}

func PayPalWebhook(c *gin.Context) {
	_, _, _, webhookID, ok := paypalConfig()
	if !ok || !isPayPalTopUpEnabled() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, 2<<20))
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	var event map[string]interface{}
	if err := common.Unmarshal(payload, &event); err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	verification := map[string]interface{}{
		"auth_algo":         c.GetHeader("PAYPAL-AUTH-ALGO"),
		"cert_url":          c.GetHeader("PAYPAL-CERT-URL"),
		"transmission_id":   c.GetHeader("PAYPAL-TRANSMISSION-ID"),
		"transmission_sig":  c.GetHeader("PAYPAL-TRANSMISSION-SIG"),
		"transmission_time": c.GetHeader("PAYPAL-TRANSMISSION-TIME"),
		"webhook_id":        webhookID,
		"webhook_event":     event,
	}
	var verified struct {
		Status string `json:"verification_status"`
	}
	if err := paypalJSONRequest(c.Request.Context(), http.MethodPost, "/v1/notifications/verify-webhook-signature", "", verification, &verified); err != nil || verified.Status != "SUCCESS" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("PayPal webhook signature verification failed client_ip=%s payload_bytes=%d", c.ClientIP(), len(payload)))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	eventType, _ := event["event_type"].(string)
	if eventType == "CHECKOUT.ORDER.APPROVED" {
		resource, _ := event["resource"].(map[string]interface{})
		orderID, _ := resource["id"].(string)
		if orderID != "" {
			if err := captureAndSettlePayPal(c.Request.Context(), orderID, c.ClientIP()); err != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("PayPal webhook settlement failed order_id=%s error=%q", orderID, err.Error()))
				c.AbortWithStatus(http.StatusServiceUnavailable)
				return
			}
		}
	}
	c.Status(http.StatusOK)
}

func RequestPayPalAmount(c *gin.Context) {
	var req AmountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil || group != "b2b" {
		common.ApiErrorMsg(c, "PayPal is available only to approved B2B customers")
		return
	}
	amount := paypalTopUpMoney(req.Amount, group)
	common.ApiSuccess(c, strconv.FormatFloat(amount.InexactFloat64(), 'f', 2, 64))
}
