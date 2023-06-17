package servers

import (
	"context"
	"errors"
	"github.com/looplab/fsm"
	"github.com/msdmazarei/mole/packets"
	. "github.com/msdmazarei/mole/shared"
	"github.com/sirupsen/logrus"
	"io"
	"math/rand"
	"net"
	"time"
)

const (
	ContextDoneStatusInterval = time.Millisecond * 100
)

var (
	ErrConnectionsExceed = errors.New("connection count exceeds max_connection")
)

type (
	UDPServerPB struct {
		UDPParams
		MaxConnection int
	}
	UDPServer struct {
		ctx               context.Context
		cancel            context.CancelFunc
		conn              UDPLikeConn
		establishedClient string
		lastZombieCheck   time.Time

		UDPServerPB
	}
	udpServerClient struct {
		server             *UDPServer
		rAddr              net.UDPAddr
		fsm                *fsm.FSM
		localNetDev        io.ReadWriteCloser
		connectedTime      time.Time
		diconnectTime      *time.Time
		remoteReport       packets.ReportPacket
		localReport        packets.ReportPacket
		lastSent, lastRecv time.Time
	}
)

func NewUDPServer(ctx context.Context, params UDPServerPB) UDPServer {
	rtn := UDPServer{
		UDPServerPB: params,
	}
	rtn.ctx, rtn.cancel = context.WithCancel(ctx)
	rtn.conn = params.Conn
	go rtn.start()
	return rtn
}

func (u *UDPServer) start() {
	// we have only support one connected connection,
	// firts connection that passes it would be considered as
	// client till get disconnected

	const (
		fnname       = "UDPServer.start"
		extraBufSize = uint16(1000)
	)
	var (
		err              error
		rAddr            *net.UDPAddr
		lastCtxDoneCheck time.Time
		buf              []byte
		n                int
		clients          = make(map[string]*udpServerClient)
	)
	// set variable initial values
	lastCtxDoneCheck = time.Now()
	buf = make([]byte, u.MTU+extraBufSize)

	defer func() {
		if u.conn != nil {
			u.conn.Close()
		}
		if errors.Is(err, ErrTimeout) {
			// what should be done
			return
		}
		// cancel context
		u.cancel()
		// call onFinish func
		u.OnFinish(err)
	}()
	isDone := false

	logrus.Info("start listening")
	for !isDone {
		isDone, lastCtxDoneCheck = u.checkIfContextIsDone(lastCtxDoneCheck)
		n, rAddr, err = u.readNet(buf)
		if err != nil {
			return
		}
		u.deleteZombieConnection(clients)
		u.processPkt(rAddr, clients, n, buf)
	}
}
func (u *UDPServer) processPkt(rAddr *net.UDPAddr, clients map[string]*udpServerClient, n int, buf []byte) {
	var (
		err        error
		molePacket packets.MoleContainerPacket
	)

	for i := 0; i < n; {
		err = molePacket.FromBytes(buf[i:])
		if err != nil {
			logrus.Warn("rAddr", rAddr.String(), "msg", "error in parsing received packet")
			return
		}
		if u.processPacket(rAddr, clients, &molePacket) != nil {
			return
		}
		i += int(molePacket.TotalLength())
	}
}
func (u *UDPServer) readNet(buf []byte) (int, *net.UDPAddr, error) {
	var (
		n     int
		err   error
		e     net.Error
		rAddr *net.UDPAddr
	)
	err = u.conn.SetReadDeadline(time.Now().Add(ContextDoneStatusInterval))
	if err != nil {
		return 0, nil, err
	}
	n, rAddr, err = u.conn.ReadFromUDP(buf)
	if err != nil {
		if errors.As(err, &e) {
			if !e.Timeout() {
				logrus.Error("msg", "error in reading from udp connection", "err", err)
				return 0, nil, err
			}
		} else {
			logrus.Error("msg", "error in reading from udp connection", "err", err)
			return 0, nil, err
		}
	}
	return n, rAddr, nil
}
func (u *UDPServer) getListOfZombies(clients map[string]*udpServerClient) ([]string, []string) {
	var toBeDeleted []string
	var toBeDisconnected []string
	for i := range clients {
		if time.Since(clients[i].lastRecv) < u.AuthTimeout {
			continue
		}
		state := clients[i].fsm.Current()
		if !Contains([]string{StateAuthenticated, StateDisconnecting}, state) {
			toBeDeleted = append(toBeDeleted, i)
			continue
		}
		switch state {
		case StateDisconnected:
			toBeDeleted = append(toBeDeleted, i)
		case StateDisconnecting:
			if time.Since(*clients[i].diconnectTime) >= u.AuthTimeout {
				toBeDeleted = append(toBeDeleted, i)
			}
		default:
			toBeDisconnected = append(toBeDisconnected, i)
		}
	}
	return toBeDisconnected, toBeDeleted
}
func (u *UDPServer) deleteZombieConnection(clients map[string]*udpServerClient) {
	var toBeDeleted []string
	var toBeDisconnected []string
	if time.Since(u.lastZombieCheck) < u.AuthTimeout {
		return
	}
	u.lastZombieCheck = time.Now()
	toBeDisconnected, toBeDeleted = u.getListOfZombies(clients)
	if len(toBeDeleted) > 0 {
		for _, client := range toBeDeleted {
			u.removeClient(clients, client)
		}
	}
	if len(toBeDisconnected) > 0 {
		for _, client := range toBeDisconnected {
			logrus.Info("client=", client,
				" last recv=", clients[client].lastRecv,
				" authtimeout=", u.AuthTimeout,
				" difference=", time.Since(clients[client].lastRecv))
			err := clients[client].disconnect()
			if err != nil {
				logrus.Warn("error in disconning client:", client, " error:", err)
			}
		}
	}
}
func (u *UDPServer) processPacket(
	raddr *net.UDPAddr,
	clients map[string]*udpServerClient,
	pkt *packets.MoleContainerPacket) error {
	var (
		usc   *udpServerClient
		exist bool
		sAddr string
		err   error
	)
	sAddr = raddr.String()
	usc, exist = clients[sAddr]
	if !exist {
		if len(clients) < u.MaxConnection {
			usc = u.newUDPServerClient(raddr)
			clients[sAddr] = usc
		} else {
			logrus.Error("msg", "total numbe of clients are reached, we have to wait")
			return ErrConnectionsExceed
		}
	}

	err = usc.processPacket(pkt)
	if err != nil {
		return err
	}
	state := usc.fsm.Current()
	if state == StateDisconnected {
		if sAddr == u.establishedClient {
			u.establishedClient = ""
			if usc.localNetDev != nil {
				usc.localNetDev.Close()
			}
		}
		// remove from clients
		u.removeClient(clients, raddr.String())
	} else if state == StateAuthenticated {
		u.establishedClient = sAddr
	}

	return err
}
func (u *UDPServer) removeClient(clients map[string]*udpServerClient, s string) {
	logrus.Info("removing client from clients list", "client", s)
	if u.OnRemovingClient != nil {
		u.OnRemovingClient(s)
	}
	delete(clients, s)
	if s == u.establishedClient {
		u.establishedClient = ""
	}
}
func (u *UDPServer) newUDPServerClient(r *net.UDPAddr) *udpServerClient {
	return &udpServerClient{
		server:        u,
		rAddr:         *r,
		fsm:           fsm.NewFSM(StateConnected, ServerFsmEvents, fsm.Callbacks{}),
		connectedTime: time.Now(),
	}
}
func (u *UDPServer) checkIfContextIsDone(t time.Time) (bool, time.Time) {
	if time.Since(t) > ContextDoneStatusInterval {
		select {
		case <-u.ctx.Done():
			return true, time.Now()
		default:
			return false, time.Now()
		}
	}
	return false, t
}
func (usc *udpServerClient) disconnect() error {
	var err error
	err = usc.sendDisconnectRequest()
	if err != nil {
		logrus.Error("Error in sending disconnect message", err)
		return err
	}
	err = usc.event(EventSendDisconnectRequest)
	if err != nil {
		logrus.Error("error in event processing for disconnect", err)
		return err
	}
	now := time.Now()
	usc.diconnectTime = &now
	return nil
}
func (usc *udpServerClient) processPacket(pkt *packets.MoleContainerPacket) error {
	var (
		err          error
		currentState string
	)
	usc.lastRecv = time.Now()

	if err = usc.authenticating(pkt); err != nil {
		return err
	}

	currentState = usc.fsm.Current()
	if Contains([]string{StateAuthenticating, StateConnected}, currentState) {
		// still waiting for client to send authentication packet
		return nil
	}
	switch pkt.PacketType {
	case packets.DisconnectAcceptType:
		err = usc.processDisconnecting(currentState)
		if err != nil {
			return err
		}
	case packets.DisconnectRequestType:
		err = usc.processDisconnect(currentState)
		if err != nil {
			return err
		}
	case packets.EncapsulatedNetDevPacketType:
		err = usc.processAuthenticated(currentState, pkt)
		if err != nil {
			return err
		}
	case packets.PingType:
		err = usc.event(EventSendPing)
		if err != nil {
			return err
		}
		err = usc.sendPong()
	case packets.ReportType:
		usc.processReport(pkt)
	default:
		break
	}
	return err
}
func (usc *udpServerClient) processReport(pkt *packets.MoleContainerPacket) {
	// update remote packet
	if usc.remoteReport.SystemTime <= pkt.MolePacket.(*packets.ReportPacket).SystemTime {
		usc.remoteReport = *pkt.MolePacket.(*packets.ReportPacket)
	}
}
func (usc *udpServerClient) processAuthenticated(currentState string, pkt *packets.MoleContainerPacket) error {
	var err error
	if currentState == StateAuthenticated && usc.localNetDev != nil {
		_, err = pkt.MolePacket.(*packets.EncapsulatedNetDevPacket).WriteContentTo(usc.localNetDev)
		if err != nil {
			return err
		}
	}
	return nil
}
func (usc *udpServerClient) processDisconnecting(currentState string) error {
	var err error
	if currentState == StateDisconnecting {
		err = usc.event(EventRecvDisconnectAccepted)
		if err != nil {
			logrus.Error("error in event processing for State_disconnecting and event disconnect accepted", err)
			return err
		}
	}
	return nil
}
func (usc *udpServerClient) processDisconnect(currentState string) error {
	var err error
	if !Contains([]string{StateDisconnecting, StateDisconnected, StateAuthenticated}, currentState) {
		return nil
	}
	err = usc.event(EventRecvDisconnectRequest)
	if err != nil {
		logrus.Error("error in event processing", err)
		return err
	}
	// send accepted
	err = usc.sendDisconnectAccepted()
	if err != nil {
		logrus.Error("error in sending msg", err)
		return err
	}
	err = usc.event(EventSendDisconnectAccepted)
	if err != nil {
		logrus.Error("error in event procssing - send_disconnect", err)
		return err
	}
	return nil
}
func (usc *udpServerClient) onSuccessAuthentication() error {
	var err error
	usc.localNetDev, err = usc.server.GetTunDev()
	if err != nil {
		return err
	}
	// make it enable
	go usc.pipeLocalPacketsToRemote()

	return nil
}
func (usc *udpServerClient) authenticating(pkt *packets.MoleContainerPacket) error {
	var (
		err     error
		authPkt *packets.AuthRequestPacket
	)
	if !usc.fsm.Is(StateConnected) {
		// because if it it other than connected, it means connection is authenticated
		return nil
	}
	if time.Since(usc.connectedTime) > usc.server.AuthTimeout {
		_ = usc.event(EventInternalAuthTimeout)
		_ = usc.sendDisconnectRequest()
		_ = usc.event(EventSendDisconnectRequest)
		return nil
	}
	if pkt.PacketType != packets.AuthRequestType {
		return nil
	}
	authPkt, _ = pkt.MolePacket.(*packets.AuthRequestPacket)
	if !authPkt.Authenticate(usc.server.Secret) {
		err = usc.event(EventInternalInvalidSecret)
		_ = usc.sendAuthRejectedPacket()
		_ = usc.sendDisconnectRequest()
		_ = usc.event(EventSendDisconnectRequest)
		return err
	}

	err = usc.event(EventInternalValidSecret)
	if err != nil {
		logrus.Error("error in event processing", err)
		return err
	}

	err = usc.onSuccessAuthentication()
	if err != nil {
		logrus.Error("on launching client requirements", err)
		return err
	}

	err = usc.sendAuthAcceptedPacket()
	return err
}
func (usc *udpServerClient) event(ev string) error {
	var err = usc.fsm.Event(usc.server.ctx, ev)
	if err != nil && !errors.Is(err, fsm.NoTransitionError{}) {
		logrus.Warn("msg", "error in FSM event", "raddr", usc.rAddr, "err", err)
		return err
	}
	return nil
}
func (usc *udpServerClient) pipeLocalPacketsToRemote() {
	const (
		extraBufSize = uint16(100)
	)
	var (
		buf         = make([]byte, usc.server.MTU+extraBufSize)
		sendBuf     = make([]byte, usc.server.MTU+extraBufSize)
		n           int
		err         error
		molePacket  packets.MoleContainerPacket
		encapPacket packets.EncapsulatedNetDevPacket
	)
	for {
		if usc.localNetDev == nil {
			return
		}
		n, err = usc.localNetDev.Read(buf)
		if err != nil {
			logrus.Error("msg", "could not read from local net dev", "err", err.Error())
			return
		}
		encapPacket = packets.NewEncapsulateNetDevPacket(buf[:n])
		molePacket = packets.NewMoleContainerPacket(&encapPacket)
		err = molePacket.WriteTo(sendBuf)
		if err != nil {
			logrus.Error("msg", "could not serialize mole packet", "err", err.Error())
			return
		}
		_, err = usc.server.conn.WriteToUDP(sendBuf[:molePacket.TotalLength()], &usc.rAddr)
		if err != nil {
			logrus.Error("msg", "could not send over listening buffer", "err", err.Error())
		}
	}
}

func (usc *udpServerClient) sendPacketToNet(pkt *packets.MoleContainerPacket) error {
	var err error

	var buf [10000]byte

	err = pkt.WriteTo(buf[:])
	if err != nil {
		return err
	}

	_, err = usc.server.conn.WriteToUDP(buf[:pkt.TotalLength()], &usc.rAddr)
	if err != nil {
		logrus.Warn("error in sending packet out", err)
		return err
	}
	usc.localReport.SentBytes += uint64(pkt.TotalLength())
	usc.localReport.SentPackets++
	usc.lastSent = time.Now()
	return nil
}

/*
	func (usc *udpServerClient) sendReport() error {
		cpkt := packets.NewMoleContainerPacket(&usc.localReport)
		return usc.sendPacketToNet(&cpkt)
	}

	func (usc *udpServerClient) sendPing() error {
		pingMsgLen := rand.Intn(100) + 10
		buf := make([]byte, pingMsgLen)
		rand.Read(buf)
		pkt := packets.NewPingPacket(string(buf))
		cpkt := packets.NewMoleContainerPacket(&pkt)
		return usc.sendPacketToNet(&cpkt)
	        }
*/
func (usc *udpServerClient) sendPong() error {
	const (
		minPongMsgLen = 10
		maxPongMsgLen = 100
	)
	pingMsgLen := rand.Intn(maxPongMsgLen-minPongMsgLen) + minPongMsgLen
	buf := make([]byte, pingMsgLen)
	rand.Read(buf)
	pkt := packets.NewPongPacket(string(buf))
	cpkt := packets.NewMoleContainerPacket(&pkt)
	return usc.sendPacketToNet(&cpkt)
}

func (usc *udpServerClient) sendDisconnectRequest() error {
	n := time.Now()
	usc.diconnectTime = &n
	dis := packets.NewDisconnectRequestPacket("request")
	cpkt := packets.NewMoleContainerPacket(&dis)
	return usc.sendPacketToNet(&cpkt)
}

func (usc *udpServerClient) sendDisconnectAccepted() error {
	dis := packets.NewDisconnectAcceptedPacket("accepted")
	cpkt := packets.NewMoleContainerPacket(&dis)
	return usc.sendPacketToNet(&cpkt)
}

/*
	func (usc *udpServerClient) sendAuthRequestPacket(secret string) error {
		auth := packets.NewAuhRequestPacket(secret)
		cpkt := packets.NewMoleContainerPacket(&auth)
		return usc.sendPacketToNet(&cpkt)
	        }
*/
func (usc *udpServerClient) sendAuthAcceptedPacket() error {
	auth := packets.NewAuthAcceptPacket(packets.AcceptCodeOK, "random_string")
	cpkt := packets.NewMoleContainerPacket(&auth)
	return usc.sendPacketToNet(&cpkt)
}
func (usc *udpServerClient) sendAuthRejectedPacket() error {
	auth := packets.NewAuthRejectPacket(packets.RejectCodeBadSecret, "random_string")
	cpkt := packets.NewMoleContainerPacket(&auth)
	return usc.sendPacketToNet(&cpkt)
}
