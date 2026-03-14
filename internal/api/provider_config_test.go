package api

import (
	"testing"
)

// =============================================================================
// maskAPIKey tests
// =============================================================================

func TestMaskAPIKey_NormalKey(t *testing.T) {
	key := "sk-1234567890abcdef"
	masked := maskAPIKey(key)
	if masked != "sk-...cdef" {
		t.Errorf("maskAPIKey(%q) = %q, want %q", key, masked, "sk-...cdef")
	}
}

func TestMaskAPIKey_ShortKey(t *testing.T) {
	// Keys <= 8 chars should be fully masked with bullets
	key := "abcd"
	masked := maskAPIKey(key)
	if masked != "••••" {
		t.Errorf("maskAPIKey(%q) = %q, want %q", key, masked, "••••")
	}
}

func TestMaskAPIKey_ExactlyEightChars(t *testing.T) {
	key := "12345678"
	masked := maskAPIKey(key)
	// 8 chars => "••••••••"
	if masked != "••••••••" {
		t.Errorf("maskAPIKey(%q) = %q, want %q", key, masked, "••••••••")
	}
}

func TestMaskAPIKey_NineChars(t *testing.T) {
	key := "123456789"
	masked := maskAPIKey(key)
	// 9 chars > 8 => prefix 3 + "..." + suffix 4 = "123...6789"
	if masked != "123...6789" {
		t.Errorf("maskAPIKey(%q) = %q, want %q", key, masked, "123...6789")
	}
}

func TestMaskAPIKey_EmptyKey(t *testing.T) {
	key := ""
	masked := maskAPIKey(key)
	if masked != "" {
		t.Errorf("maskAPIKey(%q) = %q, want %q", key, masked, "")
	}
}

// =============================================================================
// providerSecretRef tests
// =============================================================================

func TestProviderSecretRef(t *testing.T) {
	tests := []struct {
		providerID string
		want       string
	}{
		{"anthropic", "provider_api_key:anthropic"},
		{"openai", "provider_api_key:openai"},
		{"google", "provider_api_key:google"},
		{"ollama", "provider_api_key:ollama"},
		{"", "provider_api_key:"},
	}
	for _, tt := range tests {
		got := providerSecretRef(tt.providerID)
		if got != tt.want {
			t.Errorf("providerSecretRef(%q) = %q, want %q", tt.providerID, got, tt.want)
		}
	}
}
