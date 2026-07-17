package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailVerificationMessageLocalizesByLanguage(t *testing.T) {
	require.NoError(t, i18n.Init())

	tests := []struct {
		name        string
		lang        string
		subjectText string
		bodyText    string
	}{
		{name: "english", lang: i18n.LangEn, subjectText: "email verification", bodyText: "Your verification code is"},
		{name: "simplified Chinese", lang: i18n.LangZhCN, subjectText: "邮箱验证邮件", bodyText: "您的验证码为"},
		{name: "traditional Chinese", lang: i18n.LangZhTW, subjectText: "電子郵件驗證", bodyText: "您的驗證碼為"},
		{name: "unsupported falls back to English", lang: "id", subjectText: "email verification", bodyText: "Your verification code is"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, body := emailVerificationMessage(tt.lang, "Glimo Lab AI Gateway", "123456", 10)
			assert.Contains(t, subject, "Glimo Lab AI Gateway")
			assert.Contains(t, subject, tt.subjectText)
			assert.Contains(t, body, tt.bodyText)
			assert.Contains(t, body, "123456")
			assert.Contains(t, body, "10")
		})
	}
}

func TestPasswordResetMessageLocalizesByLanguage(t *testing.T) {
	require.NoError(t, i18n.Init())
	link := "https://llm.glimolab.com/user/reset?token=test"

	tests := []struct {
		lang        string
		subjectText string
		bodyText    string
	}{
		{lang: i18n.LangEn, subjectText: "password reset", bodyText: "reset your password"},
		{lang: i18n.LangZhCN, subjectText: "密码重置", bodyText: "重置密码"},
		{lang: i18n.LangZhTW, subjectText: "密碼重設", bodyText: "重設密碼"},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			subject, body := passwordResetMessage(tt.lang, "Glimo Lab AI Gateway", link, 10)
			assert.Contains(t, subject, tt.subjectText)
			assert.Contains(t, body, tt.bodyText)
			assert.Contains(t, body, link)
		})
	}
}
