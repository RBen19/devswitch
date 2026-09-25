//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package fileutil

import "fmt"

func WithLock(path string, fn func() error) error {
	return fmt.Errorf("profile updates require a supported Unix platform")
}
