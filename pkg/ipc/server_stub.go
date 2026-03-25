//go:build !linux

package ipc

import "net"

func getFormattedPeerIdentity(conn net.Conn) (string, bool) {
	return "", false
}
