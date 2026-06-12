//go:build !linux

package gocan

// linuxConfig 在非 Linux 平台是空 struct，零成本。
type linuxConfig struct{}

// applyPlatformOptions 在非 Linux 平台是 no-op。
func applyPlatformOptions(b *Bus, cfg *config) error { return nil }
