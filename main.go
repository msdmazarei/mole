package main

import (
	"context"
	"flag"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/msdmazarei/mole/clients"
	"github.com/msdmazarei/mole/servers"
	"github.com/msdmazarei/mole/shared"
	"github.com/sirupsen/logrus"

	"github.com/songgao/water"
)

const (
	exitCodeBadArg    = 1
	defaultPortNumber = 3285
	minSecretLength   = 5
	defaultMTU        = 1500
)

var (
	giface             *water.Interface
	onConnectScript    *string
	onDisconnectScript *string
	errChan            chan error
)

const (
	clientRetryDelay    time.Duration = time.Second * 5
	pingInterval                      = time.Second * 3
	authTimeout                       = time.Second * 10
	serverMaxConnection               = 10
)

func main() {
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
	onConnectScript = flag.String("onconnect", "", "script path to execute with device name once get connected")
	onDisconnectScript = flag.String("ondisconnect", "", "script path to execute with device name once get disconnected")

	flag.Parse()
	if *operationMode != "server" && *operationMode != "client" {
		logrus.Error("opeation mode has wrong value")
		os.Exit(exitCodeBadArg)
	}
	addressIP := net.ParseIP(*address)
	if addressIP == nil {
		logrus.Error("address has wrong value\n")
		os.Exit(exitCodeBadArg)
	}
	if *port > 65535 || *port < 1 {
		logrus.Error("port has wrong value")
		os.Exit(exitCodeBadArg)
	}
	if len(*secret) < minSecretLength {
		logrus.Error("secret is too short")
		os.Exit(exitCodeBadArg)
	}

	if *proto != "udp" && *proto != "tcp" {
		logrus.Error("proto has wrong value")
		os.Exit(exitCodeBadArg)
	}

	errChan = make(chan error)
	udpparams := shared.UDPParams{
		GetTunDev: getTunDev,
		OnFinish: func(err error) {
			errChan <- err
		},
		OnRemovingClient: func(c string) {
			logrus.Info("client is removed", c)
		},
		Address:     net.UDPAddr{IP: addressIP, Port: *port},
		MTU:         defaultMTU,
		AuthTimeout: authTimeout,
		Secret:      *secret,
	}
	ctx, cancel := context.WithCancel(context.Background())
	udpAddress := net.UDPAddr{IP: addressIP, Port: *port}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			// sig is a ^C, handle it
			logrus.Info("ctrl+c is captured, canceling running instance")
			cancel()
		}
	}()

	switch *operationMode {
	case "client":
		conn, err := net.DialUDP("udp", nil, &udpAddress)
		if err != nil {
			logrus.Error("error in dialing udp remote address", err)
			os.Exit(exitCodeBadArg)
		}
		udpparams.Conn = conn
		clientPB := clients.UDPClientPB{
			UDPParams:    udpparams,
			PingInterval: pingInterval,
		}
		runClient(ctx, clientPB)

	case "server":
		runServer(ctx, udpAddress, udpparams)
	}
}
func runServer(ctx context.Context, udpAddress net.UDPAddr, udpparams shared.UDPParams) {
	for ctx.Err() == nil {
		conn, err := net.ListenUDP("udp", &udpAddress)
		if err != nil {
			logrus.Error(err)
			os.Exit(exitCodeBadArg)
		}
		udpparams.Conn = conn
		serverPB := servers.UDPServerPB{
			UDPParams:     udpparams,
			MaxConnection: serverMaxConnection,
		}
		servers.NewUDPServer(ctx, serverPB)
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
			if *onDisconnectScript != "" {
				logrus.Info("executing on-disconnect script", *onDisconnectScript, " with arg ", giface.Name())
				e := exec.Command(*onDisconnectScript, giface.Name()).Run()
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
func getTunDev() (io.ReadWriteCloser, error) {
	var (
		err   error
		iface *water.Interface
	)
	defer func() {
		if err == nil && *onConnectScript != "" {
			logrus.Info("running onConnectScript...", *onConnectScript, " with devname:", giface.Name())
			err = exec.Command(*onConnectScript, giface.Name()).Run()
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
