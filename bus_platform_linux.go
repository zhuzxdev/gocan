//go:build linux

package gocan

// linuxConfig 聚合 Linux 专属 Option 的可配置项。
// 所有字段使用 pointer / 0 哨兵：未传 Option 时不调对应 setsockopt。
type linuxConfig struct {
	// 字段在后续 Task 中陆续添加（WithLoopback 等）。
}

// applyPlatformOptions 在 Initialize 成功后、startReader 之前调用，
// 把 cfg.linux 的字段写到底层 socket 上。任意一项失败 → Uninitialize 回滚 → 返回错误。
func applyPlatformOptions(b *Bus, cfg *config) error {
	// 后续 Task 在此追加调用。
	return nil
}
