//go:build linux

package ipc

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

func getPeerIdentity(conn net.Conn) (uid, gid, pid int, ok bool) {
	// Must be a unix domain socket
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, 0, 0, false
	}

	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, 0, 0, false
	}

	var ucred *unix.Ucred
	err = raw.Control(func(fd uintptr) {
		ucred, err = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})

	if err != nil || ucred == nil {
		return 0, 0, 0, false
	}

	return int(ucred.Uid), int(ucred.Gid), int(ucred.Pid), true
}

func getFormattedPeerIdentity(conn net.Conn) (string, bool) {
	uid, gid, pid, ok := getPeerIdentity(conn)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("unix:%d:%d:%d", uid, gid, pid), true
}
