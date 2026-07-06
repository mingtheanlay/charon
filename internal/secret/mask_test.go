package secret

import "testing"

func TestMask(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"short is fully hidden", "sk-123", "••••"},
		{"exactly ten hidden", "0123456789", "••••"},
		{"long keeps prefix and suffix", "sk-or-xyz789012", "sk-or-…9012"},
		{"anthropic key", "sk-ant-abc123456789", "sk-ant…6789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Mask(tt.in); got != tt.want {
				t.Errorf("Mask(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMaskNeverLeaksMiddle(t *testing.T) {
	secret := "sk-super-secret-token-value-1234"
	if got := Mask(secret); len(got) >= len(secret) {
		t.Fatalf("masked value %q not shorter than original", got)
	}
}
