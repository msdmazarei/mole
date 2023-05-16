package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/msdmazarei/mole/streams"
	"github.com/songgao/water"
)

var (
	ExitCodeBadArg = 1
)

type moleConfig struct {
	opeation_mode string
	address       net.IP
	port          int
	secret        string
	proto         string
	iface         *water.Interface
}

func main() {

	operation_mode := flag.String("operation_mode", "server", "valid values are server or client")
	address := flag.String("address", "0.0.0.0", "in server mode specifies ip address to listen on, and in client mode it specifies remote server address to connect")
	port := flag.Int("port", 3285, "in server mode specifies listening port and in client mode specifies to server port to connect")
	proto := flag.String("proto", "udp", "valid values are udp and tcp")
	secret := flag.String("secret", "secret", "secret value btw client and server")
	net_dev := flag.String("net_dev", "", "specifies network device name - TAP device name")

	flag.Parse()
	if *operation_mode != "server" && *operation_mode != "client" {
		fmt.Printf("opeation mode has wrong value\n")
		os.Exit(ExitCodeBadArg)
	}
	address_ip := net.ParseIP(*address)
	if address_ip == nil {
		fmt.Printf("address has wrong value\n")
		os.Exit(ExitCodeBadArg)
	}
	if *port > 65535 || *port < 1 {
		fmt.Print("port has wrong value")
		os.Exit(ExitCodeBadArg)
	}
	if len(*secret) < 5 {
		fmt.Print("secret is too short")
		os.Exit(ExitCodeBadArg)
	}
	if *net_dev == "" {
		fmt.Print("net_dev should have a value")
		os.Exit(ExitCodeBadArg)
	}
	if *proto != "udp" && *proto != "tcp" {
		fmt.Print("proto has wrong value")
		os.Exit(ExitCodeBadArg)
	}
	cfg := water.Config{
		DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{
			Name: *net_dev,
		},
	}
	iface, err := water.New(cfg)
	if err != nil {
		fmt.Printf("error: %s\n", err.Error())
		os.Exit(ExitCodeBadArg)
	}
	app_cfg := moleConfig{
		opeation_mode: *operation_mode,
		proto:         *proto,
		address:       address_ip,
		port:          *port,
		iface:         iface,
		secret:        *secret,
	}

	switch *operation_mode {
	case "client":
		runClientMode(app_cfg)
	case "server":
		runServerMode(app_cfg)
	}

}

func runServerMode(cfg moleConfig) error {
	var (
		buf        [5000]byte
		raddr      *net.UDPAddr = nil
		udp_stream *UdpStreamer
	)
	err_chan := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		fmt.Print("returning from runServerMode Function\n")
		cancel()
	}()
	udpaddr := net.UDPAddr{
		IP:   cfg.address,
		Port: cfg.port,
	}
	udp_con, err := net.ListenUDP("udp", &udpaddr)
	if err != nil {
		panic(err)
	}

	for {
		udp_con.SetReadDeadline(time.Now().Add(time.Millisecond))
		n, recv_addr, err := udp_con.ReadFromUDP(buf[:])
		if err != nil && err.(*net.OpError).Timeout() {
			select {
			case e := <-err_chan:
				fmt.Printf("sever is dropped, because: %+v\n", e)
				raddr = nil
				udp_stream = nil
				continue

			default:
				continue
			}
		}
		if err != nil {
			fmt.Printf("exit from server cause of: %+v\n", err)
			return err
		}

		if raddr != nil && recv_addr.String() != raddr.String() {
			fmt.Printf("ignore packet: raddr: %+v recv_arr: %+v\n", raddr, recv_addr)
			//ignore received packet
			continue
		}
		if raddr == nil {
			fmt.Printf("setup udp streamer\n")
			raddr = recv_addr
			udp_stream = NewUdpStreamer(udp_con, *recv_addr)
			server_type := streams.MolePeerServer
			mpb := streams.MolePeerPB{
				NetDevIO:     cfg.iface,
				NetworkIO:    udp_stream,
				Secret:       cfg.secret,
				Context:      ctx,
				OnClose:      func(e error) { err_chan <- e },
				MolePeerType: &server_type,
			}
			streams.NewMolePeer(mpb)
		}
		select {
		case e := <-err_chan:
			fmt.Printf("sever is dropped, because: %+v\n", e)
			raddr = nil
			udp_stream = nil

		default:
		}
		if n > 0 && udp_stream != nil {
			if buf[2] > 9 {
				fmt.Printf("wrong buf!, %+v\n", buf)
				panic("received wrong buffer")
			}
			udp_stream.SetRecvData(buf[:n])
		}
	}

}
func runClientMode(cfg moleConfig) error {
	client_err_chan := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	udpaddr := net.UDPAddr{
		IP:   cfg.address,
		Port: cfg.port,
	}
	udp_con, err := net.DialUDP("udp", nil, &udpaddr)
	if err != nil {
		panic(err)
	}
	client_type := streams.MolePeerClient
	mpb := streams.MolePeerPB{
		NetDevIO:     cfg.iface,
		NetworkIO:    udp_con,
		Secret:       cfg.secret,
		Context:      ctx,
		OnClose:      func(e error) { client_err_chan <- e },
		MolePeerType: &client_type,
	}
	streams.NewMolePeer(mpb)
	e := <-client_err_chan
	fmt.Printf("client is stopped cause of: %+\n", e)
	return e
}

type UdpStreamer struct {
	conn  *net.UDPConn
	raddr net.UDPAddr
	recv  chan []byte
}

func NewUdpStreamer(conn *net.UDPConn, raddr net.UDPAddr) *UdpStreamer {
	return &UdpStreamer{
		conn:  conn,
		raddr: raddr,
		recv:  make(chan []byte),
	}
}
func (us *UdpStreamer) Write(buf []byte) (int, error) {
	return us.conn.WriteToUDP(buf, &us.raddr)
}
func (us *UdpStreamer) Read(buf []byte) (n int, e error) {
	r := <-us.recv
	n = copy(buf, r)
	return n, nil
}
func (us *UdpStreamer) SetRecvData(buf []byte) {
	var b []byte = make([]byte,len(buf))
	copy(b, buf)
	select {
	case us.recv <- b:
	case <-time.After(time.Millisecond):
		fmt.Printf("server timeouted in reading\n")
	}

}
