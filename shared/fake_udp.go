package shared

import (
	"net"
	"time"
)

type (
	ConnSide int

	FakeUDP struct {
		Server       net.Conn
		Client       net.Conn
		selectedSide ConnSide
	}
)

const (
	ServerSide  ConnSide = 1
	ClientSide  ConnSide = 2
	defaultPort          = 3285
)

var (
	localhost = net.IPv4(127, 0, 0, 1)
)

func NewFakeUDP(side ConnSide) (*FakeUDP, error) {
	rtn := &FakeUDP{
		selectedSide: side,
	}
	rtn.Server, rtn.Client = net.Pipe()
	return rtn, nil
}

func (f *FakeUDP) Close() error {
	switch f.selectedSide {
	case ServerSide:
		return f.Server.Close()
	case ClientSide:
		return f.Client.Close()
	}
	return nil
}
func (f *FakeUDP) SetReadDeadline(t time.Time) error {
	switch f.selectedSide {
	case ClientSide:
		return f.Client.SetReadDeadline(t)
	case ServerSide:
		return f.Server.SetReadDeadline(t)
	}
	return nil
}
func (f *FakeUDP) SetWriteDeadline(t time.Time) error {
	switch f.selectedSide {
	case ClientSide:
		return f.Client.SetWriteDeadline(t)
	case ServerSide:
		return f.Server.SetWriteDeadline(t)
	}
	return nil
}
func (f *FakeUDP) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	var (
		n    int
		addr *net.UDPAddr
		err  error
	)
	switch f.selectedSide {
	case ServerSide:
		n, err = f.Server.Read(b)
	case ClientSide:
		n, err = f.Client.Read(b)
	}
	addr = &net.UDPAddr{IP: localhost, Port: defaultPort}
	return n, addr, err
}
func (f *FakeUDP) WriteToUDP(b []byte, _ *net.UDPAddr) (int, error) {
	switch f.selectedSide {
	case ServerSide:
		return f.Server.Write(b)
	case ClientSide:
		return f.Client.Write(b)
	}
	return 0, nil
}
func (f *FakeUDP) Write(b []byte) (int, error) {
	switch f.selectedSide {
	case ServerSide:
		return f.Server.Write(b)
	case ClientSide:
		return f.Client.Write(b)
	}
	return 0, nil
}

func (f *FakeUDP) Read(b []byte) (int, error) {
	switch f.selectedSide {
	case ServerSide:
		return f.Server.Read(b)
	case ClientSide:
		return f.Client.Read(b)
	}
	return 0, nil
}
