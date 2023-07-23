package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/msdmazarei/mole/clients"
	"github.com/msdmazarei/mole/packets"
	"github.com/msdmazarei/mole/servers"
	"github.com/msdmazarei/mole/shared"
	"github.com/sirupsen/logrus"

	"github.com/songgao/water"
)

type (
	operationMode int
	netProtocol   string

	cfg struct {
		operationMode        operationMode
		address              net.IP
		port                 int
		proto                netProtocol
		username             string
		secret               string
		onConnectScript      string
		onDisconnectScript   string
		socks5ProxyIPAddress net.IP
		socks5ProxyPort      int
	}
)

const (
	exitCodeBadArg                    = 1
	defaultPortNumber                 = 3285
	minSecretLength                   = 5
	defaultMTU                        = 1500
	clientRetryDelay    time.Duration = time.Second * 5
	pingInterval                      = time.Second * 3
	authTimeout                       = time.Second * 10
	serverMaxConnection               = 10

	serverMode operationMode = 1
	clientMode operationMode = 2
)

var (
	giface     *water.Interface
	errChan    chan error
	srvDevList = make(map[string]*water.Interface)
	appCfg     cfg
)

func parseArgs() cfg {
	operationMode := flag.String(
		"operation_mode",
		"server",
		"valid values are server or client")

	address := flag.String(
		"address",
		"0.0.0.0",
		"in server mode specifies ip address to listen on, and in client mode it specifies remote server address to connect")

	port := flag.Int("port",
		defaultPortNumber,
		"in server mode specifies listening port "+"and in client mode specifies to server port to connect")
	proto := flag.String("proto", "udp", "valid values are udp and tcp")
	secret := flag.String("secret", "secret", "secret value btw client and server")
	username := flag.String("username", "username", "username value btw client and server")

	onConnectScript := flag.String("onconnect", "", "script path to execute with device name once get connected")
	onDisconnectScript := flag.String("ondisconnect", "", "script path to execute with device name once get disconnected")

	socks5Proxy := flag.String("socks5-proxy", "", "to use this address to proxy, it should be in IPv4:PORT format like: 127.0.0.1:1090")

	flag.Parse()
	rtn := cfg{}
	switch *operationMode {
	case "server":
		rtn.operationMode = serverMode
	case "client":
		rtn.operationMode = clientMode
	default:
		logrus.Error("opeation mode has wrong value")
		os.Exit(exitCodeBadArg)
	}

	addressIP := net.ParseIP(*address)
	if addressIP == nil {
		logrus.Error("address has wrong value\n")
		os.Exit(exitCodeBadArg)
	}
	rtn.address = addressIP

	if *port > 65535 || *port < 1 {
		logrus.Error("port has wrong value")
		os.Exit(exitCodeBadArg)
	}
	rtn.port = *port

	if len(*secret) < minSecretLength {
		logrus.Error("secret is too short")
		os.Exit(exitCodeBadArg)
	}
	rtn.secret = *secret
	rtn.username = *username

	if *proto != "udp" && *proto != "tcp" {
		logrus.Error("proto has wrong value")
		os.Exit(exitCodeBadArg)
	}
	rtn.proto = netProtocol(*proto)
	rtn.onConnectScript = *onConnectScript
	rtn.onDisconnectScript = *onDisconnectScript

	if *socks5Proxy != "" {
		parts := strings.Split(*socks5Proxy, ":")
		if len(parts) != 2 {
			logrus.Error("Bad format socks5-proxy, it should be IPv4:Port")
			os.Exit(exitCodeBadArg)
		}
		proxyIP := net.ParseIP(parts[0])
		if proxyIP == nil {
			logrus.Error("Bad IP Address in socks5-proxy")
			os.Exit(exitCodeBadArg)
		}
		proxyPort, e := strconv.ParseInt(parts[1], 10, 16)
		if e != nil {
			logrus.Error("Bad Port in socks5-proxy")
			os.Exit(exitCodeBadArg)
		} else if proxyPort < 0 || proxyPort > 65535 {
			logrus.Error("Bad Port in socks5-proxy")
			os.Exit(exitCodeBadArg)
		} else {

		}
		rtn.socks5ProxyIPAddress = proxyIP
		rtn.socks5ProxyPort = int(proxyPort)
	}
	return rtn
}
func main() {
	var (
		listener net.Listener
		err      error
	)
	appCfg = parseArgs()
	errChan = make(chan error)
	udpparams := shared.UDPParams{
		GetTunDev: getTunDev,
		OnFinish: func(err error) {
			errChan <- err
		},
		Address:     net.UDPAddr{IP: appCfg.address, Port: appCfg.port},
		MTU:         defaultMTU,
		AuthTimeout: authTimeout,
	}
	tcpparams := shared.TCPParams{
		GetTunDev: getTunDevServer,
		OnFinish: func(err error) {
			errChan <- err
		},
		Address:     net.TCPAddr{IP: appCfg.address, Port: appCfg.port},
		MTU:         defaultMTU,
		AuthTimeout: authTimeout,
	}
	ctx, cancel := context.WithCancel(context.Background())
	udpAddress := net.UDPAddr{IP: appCfg.address, Port: appCfg.port}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			// sig is a ^C, handle it
			logrus.Info("ctrl+c is captured, canceling running instance")
			cancel()
			// close listener
			if appCfg.proto == "tcp" {
				listener.Close()
			}
		}
	}()

	switch appCfg.operationMode {
	case clientMode:
		switch appCfg.proto {
		case "udp":
			conn, err := net.DialUDP("udp", nil, &udpAddress)
			if err != nil {
				logrus.Error("error in dialing udp remote address", err)
				os.Exit(exitCodeBadArg)
			}
			udpparams.Conn = conn
			clientPB := clients.UDPClientPB{
				UDPParams:    udpparams,
				PingInterval: pingInterval,
				Secret:       appCfg.secret,
				Username:     appCfg.username,
			}
			runClient(ctx, clientPB)

		case "tcp":
			clientPB := clients.TCPClientPB{
				TCPParams:    tcpparams,
				PingInterval: pingInterval,
				Secret:       appCfg.secret,
				Username:     appCfg.username,
			}
			runTCPClient(ctx, clientPB)
		}

	case serverMode:
		switch appCfg.proto {
		case "udp":

			serverPB := servers.UDPServerPB{
				UDPParams:        udpparams,
				MaxConnection:    serverMaxConnection,
				OnRemovingClient: srvRemoveClient,
				Authenticate: func(authPkt packets.AuthRequestPacket) bool {
					return authPkt.Username == appCfg.username && authPkt.Authenticate(appCfg.secret)
				},
			}
			serverPB.GetTunDev = getTunDevServer

			runServer(ctx, udpAddress, serverPB)
		case "tcp":
			listener, err = net.ListenTCP("tcp", &net.TCPAddr{IP: appCfg.address, Port: appCfg.port})
			if err != nil {
				logrus.Error("error in listening tcp", err)
				os.Exit(exitCodeBadArg)
			}

			serverPB := servers.TCPServerPB{
				TCPParams:        tcpparams,
				MaxConnection:    serverMaxConnection,
				OnRemovingClient: srvRemoveClient,
				Authenticate: func(authPkt packets.AuthRequestPacket) bool {
					return authPkt.Username == appCfg.username && authPkt.Authenticate(appCfg.secret)
				},
			}
			serverPB.GetTunDev = getTunDevServer
			serverPB.Listener = listener
			runTCPServer(ctx, net.TCPAddr{IP: appCfg.address, Port: appCfg.port}, serverPB)
		}

	}
}
func runServer(ctx context.Context, udpAddress net.UDPAddr, serverPB servers.UDPServerPB) {
	conn, err := net.ListenUDP("udp", &udpAddress)
	if err != nil {
		logrus.Error("udp address:", udpAddress, " err:", err)
		os.Exit(exitCodeBadArg)
	}
	serverPB.Conn = conn
	for ctx.Err() == nil {
		servers.NewUDPServer(ctx, serverPB)
		err = <-errChan
		if err != nil {
			logrus.Error("server udp exited cause of ", err)
		}
	}
}
func runTCPServer(ctx context.Context, udpAddress net.TCPAddr, serverPB servers.TCPServerPB) {
	var err error
	serverPB.Conn = nil
	for ctx.Err() == nil {
		servers.NewTCPServer(ctx, serverPB)
		err = <-errChan
		if err != nil {
			logrus.Error("server tcp exited cause of ", err)
		}
	}
}
func runClient(ctx context.Context, clientPB clients.UDPClientPB) {
	var err error
	for ctx.Err() == nil {
		_, err = clients.NewUDPClient(ctx, clientPB)
		if err != nil {
			logrus.Error("error in creating client:", err)
		}
		select {
		case err = <-errChan:
			if appCfg.onDisconnectScript != "" && giface != nil {
				logrus.Info("executing on-disconnect script", appCfg.onDisconnectScript, " with arg ", giface.Name())
				e := exec.Command(appCfg.onDisconnectScript, giface.Name()).Run()
				if e != nil {
					logrus.Error("failure on executing on-disconnect command", e)
				}
			}
			logrus.Error("client exited caused of ", err)
			logrus.Info("retry to reconnect again After 5 second")
			<-time.After(clientRetryDelay)

		case <-ctx.Done():
		}
	}
}
func runTCPClient(ctx context.Context, clientPB clients.TCPClientPB) {
	var (
		conn net.Conn
		err  error
	)
	for ctx.Err() == nil {
		if appCfg.socks5ProxyIPAddress != nil {
			conn, err = net.DialTCP("tcp", nil, &net.TCPAddr{IP: appCfg.socks5ProxyIPAddress, Port: appCfg.socks5ProxyPort})
			if err != nil {
				logrus.Error("error in dialing socks proxy remote address", err)
				os.Exit(exitCodeBadArg)
			}
			conn, err = connectThroughSocks5TCP(conn)
			if err != nil {
				logrus.Error("error in connecting through socks proxy remote address", err)
				<-time.After(clientRetryDelay)
				continue
			}
		} else {
			conn, err = net.DialTCP("tcp", nil, &net.TCPAddr{IP: appCfg.address, Port: appCfg.port})
		}

		if err != nil {
			logrus.Error("error in dialing udp remote address", err)
			os.Exit(exitCodeBadArg)
		}
		clientPB.Conn = conn
		_, err = clients.NewTCPClient(ctx, clientPB)
		if err != nil {
			logrus.Error("error in creating client:", err)
		}
		select {
		case err = <-errChan:
			if appCfg.onDisconnectScript != "" && giface != nil {
				logrus.Info("executing on-disconnect script", appCfg.onDisconnectScript, " with arg ", giface.Name())
				e := exec.Command(appCfg.onDisconnectScript, giface.Name()).Run()
				if e != nil {
					logrus.Error("failure on executing on-disconnect command", e)
				}
			}
			logrus.Error("client exited caused of ", err)
			logrus.Info("retry to reconnect again After 5 second")
			<-time.After(clientRetryDelay)

		case <-ctx.Done():
		}
	}
}
func getTunDev(string, *shared.TunDevProps) (io.ReadWriteCloser, error) {
	var (
		err   error
		iface *water.Interface
	)
	defer func() {
		if err == nil && appCfg.onConnectScript != "" {
			logrus.Info("running onConnectScript...", appCfg.onConnectScript, " with devname:", giface.Name())
			err = exec.Command(appCfg.onConnectScript, giface.Name()).Run()
			if err != nil {
				logrus.Error("error in executing connect script", err)
			}
		}
	}()
	cfg := water.Config{
		DeviceType: water.TUN,
	}
	if giface != nil {
		cfg.Name = giface.Name()
	}
	iface, err = water.New(cfg)
	if err != nil {
		if err.Error() == "ioctl: device or resource busy" {
			logrus.Info("using tun dev", giface.Name())
			return giface, nil
		}
		logrus.Error("error in getting tun dev:", err)
		return nil, err
	}
	logrus.Info("successfully tun dev is create:", iface.Name())
	giface = iface
	return giface, nil
}

func srvRemoveClient(_ string,
	username string,
	localDevNet io.ReadWriteCloser,
	_ *shared.TunDevProps) {
	iface, ok := localDevNet.(*water.Interface)

	if !ok {
		logrus.Error("removing client is getting invalid tun device, its not water.interface")
		return
	}
	// need to do it using lock but its ok for now
	delete(srvDevList, iface.Name())
	if appCfg.onDisconnectScript != "" {
		logrus.Info(
			"running onDisconnectScript...", appCfg.onDisconnectScript,
			" with devname:", iface.Name(),
			" username: ", username,
		)
		go func() {
			err := exec.Command(appCfg.onDisconnectScript, iface.Name(), username).Run()
			if err != nil {
				logrus.Error("error in executing connect script", err)
			}
		}()
	}
}

var tundevcounter int

func getTunDevServer(username string, _ *shared.TunDevProps) (io.ReadWriteCloser, error) {
	var (
		err   error
		cfg   water.Config
		iface *water.Interface
		exist bool
	)
	defer func() {
		if err == nil && appCfg.onConnectScript != "" {
			logrus.Info(
				"running onConnectScript...", appCfg.onConnectScript,
				" with devname:", iface.Name(),
				" username: ", username)
			err = exec.Command(appCfg.onConnectScript, iface.Name(), username).Run()
			if err != nil {
				logrus.Error("error in executing connect script", err)
			}
		}
	}()
	iface, exist = srvDevList[username]
	if exist && iface != nil {
		// make piping process exit and running connection to be disconnetd
		iface.Close()
	}
	tundevcounter++
	cfg = water.Config{
		DeviceType: water.TUN,
	}
	cfg.Name = fmt.Sprintf("tun%d", tundevcounter)

	iface, err = water.New(cfg)
	if err != nil {
		logrus.Error("in creation tun dev", " err:", err)
		return nil, err
	}

	logrus.Info("successfully tun dev is:", iface.Name())
	srvDevList[username] = iface
	return iface, nil
}
