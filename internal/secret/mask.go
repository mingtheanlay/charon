// Package secret handles masking of sensitive values and platform keychains.
package secret

// Mask returns a display-safe version of a secret, keeping only a short
// prefix and suffix so profiles can be told apart without leaking the key.
func Mask(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= 10 {
		return "••••"
	}
	return string(r[:6]) + "…" + string(r[len(r)-4:])
}
