package main

import (
	"testing"

	"smartdoc-vision/backend/internal/config"
)

func TestApplyListenOverrides(t *testing.T) {
	base := config.Config{ServerHost: "0.0.0.0", ServerPort: "8080"}

	cfg := base
	if err := applyListenOverrides(&cfg, "", ""); err != nil {
		t.Fatal(err)
	}
	if cfg.ServerHost != "0.0.0.0" || cfg.ServerPort != "8080" {
		t.Fatalf("empty flags must keep the configured address: %+v", cfg)
	}

	cfg = base
	if err := applyListenOverrides(&cfg, "127.0.0.1", "9000"); err != nil {
		t.Fatal(err)
	}
	if cfg.ServerHost != "127.0.0.1" || cfg.ServerPort != "9000" {
		t.Fatalf("flags must override the configured address: %+v", cfg)
	}
	if got := cfg.ListenAddr(); got != "127.0.0.1:9000" {
		t.Fatalf("ListenAddr() = %q", got)
	}

	for _, bad := range []string{"0", "65536", "abc", "-1"} {
		cfg = base
		if err := applyListenOverrides(&cfg, "", bad); err == nil {
			t.Fatalf("expected -port %q to be rejected", bad)
		}
	}
}

func TestDisplayAddr(t *testing.T) {
	tests := []struct {
		host string
		port string
		want string
	}{
		{host: "0.0.0.0", port: "8080", want: "127.0.0.1:8080"},
		{host: "", port: "8080", want: "127.0.0.1:8080"},
		{host: "::", port: "8080", want: "127.0.0.1:8080"},
		{host: "192.168.1.10", port: "9000", want: "192.168.1.10:9000"},
	}
	for _, tc := range tests {
		cfg := config.Config{ServerHost: tc.host, ServerPort: tc.port}
		if got := displayAddr(cfg); got != tc.want {
			t.Fatalf("displayAddr(%q:%q) = %q, want %q", tc.host, tc.port, got, tc.want)
		}
	}
}
