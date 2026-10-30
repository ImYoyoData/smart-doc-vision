package api

import (
	"testing"

	"smartdoc-vision/backend/internal/security"
)

func testNotificationServer(t *testing.T) *Server {
	t.Helper()
	cipher, err := security.NewCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	return &Server{cipher: cipher}
}

func validNotificationInput() notificationChannelInput {
	return notificationChannelInput{
		Name: "  业务通知  ", Type: "email", Provider: "QQ", Status: "active",
		Config: notificationEmailConfig{
			Host: "  smtp.qq.com ", Port: 465, Security: "ssl",
			Username: " notice@qq.com ", Password: "auth-code",
			FromAddress: " notice@qq.com ", TimeoutSec: 10,
		},
	}
}

func TestBuildNotificationChannelEmail(t *testing.T) {
	server := testNotificationServer(t)
	channel, err := server.buildNotificationChannel(1, validNotificationInput(), nil)
	if err != nil {
		t.Fatalf("build channel: %v", err)
	}
	if channel.Name != "业务通知" || channel.Type != "EMAIL" || channel.Provider != "qq" {
		t.Fatalf("unexpected channel: %+v", channel)
	}
	cfg, err := server.decryptNotificationConfig(channel)
	if err != nil {
		t.Fatalf("decrypt config: %v", err)
	}
	if cfg.Host != "smtp.qq.com" || cfg.Username != "notice@qq.com" || cfg.Password != "auth-code" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.FromName != "业务通知" {
		t.Fatalf("from name = %q, want channel name", cfg.FromName)
	}
	view := server.notificationChannelView(channel)
	if !view.HasPassword || view.Config.Password != "" {
		t.Fatalf("view must expose has_password but not password: %+v", view)
	}
}

func TestBuildNotificationChannelKeepsPasswordOnUpdate(t *testing.T) {
	server := testNotificationServer(t)
	existing, err := server.buildNotificationChannel(1, validNotificationInput(), nil)
	if err != nil {
		t.Fatalf("build existing: %v", err)
	}
	req := validNotificationInput()
	req.Name = "更新后的通知"
	req.Config.Password = ""
	updated, err := server.buildNotificationChannel(1, req, &existing)
	if err != nil {
		t.Fatalf("update channel: %v", err)
	}
	cfg, err := server.decryptNotificationConfig(updated)
	if err != nil {
		t.Fatalf("decrypt updated config: %v", err)
	}
	if cfg.Password != "auth-code" {
		t.Fatalf("password not preserved: %q", cfg.Password)
	}
}

func TestBuildNotificationChannelRejectsInvalid(t *testing.T) {
	server := testNotificationServer(t)
	cases := []struct {
		name   string
		mutate func(*notificationChannelInput)
	}{
		{"unsupported type", func(req *notificationChannelInput) { req.Type = "SMS" }},
		{"missing host", func(req *notificationChannelInput) { req.Config.Host = "" }},
		{"invalid from address", func(req *notificationChannelInput) { req.Config.FromAddress = "not-an-email" }},
		{"missing password on create", func(req *notificationChannelInput) { req.Config.Password = "" }},
		{"invalid security", func(req *notificationChannelInput) { req.Config.Security = "tls1.3" }},
	}
	for _, tc := range cases {
		req := validNotificationInput()
		tc.mutate(&req)
		if _, err := server.buildNotificationChannel(1, req, nil); err == nil {
			t.Fatalf("%s: expected validation error", tc.name)
		}
	}
}

func TestDefaultSMTPPort(t *testing.T) {
	if port := defaultSMTPPort("ssl"); port != 465 {
		t.Fatalf("ssl port = %d", port)
	}
	if port := defaultSMTPPort("starttls"); port != 587 {
		t.Fatalf("starttls port = %d", port)
	}
	if port := defaultSMTPPort("none"); port != 25 {
		t.Fatalf("none port = %d", port)
	}
}
