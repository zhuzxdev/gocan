//go:build !linux

package gocan

// SetErrFilter 在非 Linux 平台返回 ErrNotSupported。
func (b *Bus) SetErrFilter(mask uint32) error { return ErrNotSupported }

// SetJoinFilters 在非 Linux 平台返回 ErrNotSupported。
func (b *Bus) SetJoinFilters(and bool) error { return ErrNotSupported }
