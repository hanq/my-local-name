//go:build !windows

package mdns

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// reuseControl allows sharing UDP port 5353 with mDNSResponder (macOS) or
// Avahi (Linux). SO_REUSEPORT is required on Linux and supported on macOS.
func reuseControl(_, _ string, c syscall.RawConn) error {
	var sockErr error
	err := c.Control(func(fd uintptr) {
		if e := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); e != nil {
			sockErr = e
			return
		}
		if e := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); e != nil {
			sockErr = e
		}
	})
	if err != nil {
		return err
	}
	return sockErr
}
