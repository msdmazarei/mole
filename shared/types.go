package shared

import (
	"context"
	"io"
	"net"
	"time"
)

type (
	UDPLikeConn interface {
		io.ReadWriter
		Close() error
		SetReadDeadline(t time.Time) error
		SetWriteDeadline(t time.Time) error
		ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error)
		WriteToUDP(b []byte, addr *net.UDPAddr) (int, error)
	}
	UDPParams struct {
		GetTunDev        func() (io.ReadWriteCloser, error)
		OnFinish         context.CancelCauseFunc
		OnRemovingClient func(clientAddress string)
		Address          net.UDPAddr
		Conn             UDPLikeConn
		MTU              uint16
		AuthTimeout      time.Duration
		Secret           string
	}
)
