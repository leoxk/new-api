package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRequestHeadersCarryIdempotencyAndCorrelation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("preserves client idempotency key", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Header.Set("Idempotency-Key", "client-operation-1")
		header := http.Header{}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{},
			RelayMode:   relayconstant.RelayModeImagesGenerations,
			RequestId:   "gateway-request-1",
			UserId:      10,
			TokenId:     20,
		}

		err := (&Adaptor{}).SetupRequestHeader(c, &header, info)

		require.NoError(t, err)
		assert.Equal(t, "client-operation-1", header.Get("Idempotency-Key"))
		assert.Equal(t, "gateway-request-1", header.Get("X-Request-Id"))
		assert.Equal(t, "10:20", header.Get("X-Idempotency-Scope"))
	})

	t.Run("falls back to gateway request id", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
		header := http.Header{}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{},
			RelayMode:   relayconstant.RelayModeImagesEdits,
			RequestId:   "gateway-request-2",
		}

		err := (&Adaptor{}).SetupRequestHeader(c, &header, info)

		require.NoError(t, err)
		assert.Equal(t, "gateway-request-2", header.Get("Idempotency-Key"))
		assert.Equal(t, "gateway-request-2", header.Get("X-Request-Id"))
	})

	t.Run("does not add image headers to text requests", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		header := http.Header{}
		info := &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{},
			RequestId:   "gateway-request-3",
		}

		err := (&Adaptor{}).SetupRequestHeader(c, &header, info)

		require.NoError(t, err)
		assert.Empty(t, header.Get("Idempotency-Key"))
		assert.Empty(t, header.Get("X-Request-Id"))
		assert.Empty(t, header.Get("X-Idempotency-Scope"))
	})
}
