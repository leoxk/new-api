package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaNotificationMessageLocalizesByUserLanguage(t *testing.T) {
	require.NoError(t, i18n.Init())
	link := "https://llm.glimolab.com/console/topup"

	tests := []struct {
		name         string
		lang         string
		notifyType   string
		subscription bool
		titleText    string
		bodyText     string
		hasHTMLLink  bool
	}{
		{name: "English email", lang: i18n.LangEn, notifyType: dto.NotifyTypeEmail, titleText: "balance is running low", bodyText: "remaining quota", hasHTMLLink: true},
		{name: "simplified Chinese subscription email", lang: i18n.LangZhCN, notifyType: dto.NotifyTypeEmail, subscription: true, titleText: "订阅额度即将用尽", bodyText: "当前剩余额度", hasHTMLLink: true},
		{name: "traditional Chinese text notification", lang: i18n.LangZhTW, notifyType: dto.NotifyTypeBark, titleText: "額度即將用完", bodyText: "目前剩餘額度"},
		{name: "unset language falls back to English", notifyType: dto.NotifyTypeGotify, titleText: "balance is running low", bodyText: "remaining quota"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, body := quotaNotificationMessage(tt.lang, tt.notifyType, tt.subscription, "$1.25", link)
			assert.Contains(t, title, tt.titleText)
			assert.Contains(t, body, tt.bodyText)
			assert.Contains(t, body, "$1.25")
			if tt.hasHTMLLink {
				assert.Contains(t, body, link)
				assert.Contains(t, body, "<a href=")
			} else {
				assert.NotContains(t, body, "<a href=")
			}
		})
	}
}
