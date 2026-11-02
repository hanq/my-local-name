//go:build windows

package mdns

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// reuseControl allows sharing UDP port 5353 with the operating system's own
// mDNS responder (Dnscache on Windows).
func reuseControl(_, _ string, c syscall.RawConn) error {
	var sockErr error
	err := c.Control(func(fd uintptr) {
		h := windows.Handle(fd)
		if e := windows.SetsockoptInt(h, windows.SOL_SOCKET, windows.SO_REUSEADDR, 1); e != nil {
			sockErr = e
		}
	})
	if err != nil {
		return err
	}
	return sockErr
}
