package servers

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/looplab/fsm"
	"github.com/msdmazarei/mole/packets"
	. "github.com/msdmazarei/mole/shared"
	"github.com/sirupsen/logrus"
)

const (
	extraBufSize = 1000
)

type (
	TCPServerPB struct {
		TCPParams
		MaxConnection    int
		Authenticate     func(packets.AuthRequestPacket) bool
		OnRemovingClient func(
			clientRemoteAddress string,
			username string,
			localDevNet io.ReadWriteCloser,
			tunDev *TunDevProps)
	}
	TCPServer struct {
		ctx        context.Context
		cancel     context.CancelFunc
		clients    map[string]*tcpServerClient
		clientLock sync.RWMutex

		TCPServerPB
	}
	tcpServerClient struct {
		ctx                context.Context
		cancel             context.CancelFunc
		conn               net.Conn
		server             *TCPServer
		fsm                *fsm.FSM
		localNetDev        io.ReadWriteCloser
		connectedTime      time.Time
		diconnectTime      *time.Time
		remoteReport       packets.ReportPacket
		localReport        packets.ReportPacket
		lastSent, lastRecv time.Time
		authPkt            packets.AuthRequestPacket
		tunDevProps        *TunDevProps
	}
)

func NewTCPServer(ctx context.Context, params TCPServerPB) *TCPServer {
	rtn := &TCPServer{
		TCPServerPB: params,
		clients:     make(map[string]*tcpServerClient),
		clientLock:  sync.RWMutex{},
	}
	rtn.ctx, rtn.cancel = context.WithCancel(ctx)
	go rtn.start()
	return rtn
}

func (t *TCPServer) start() {
	var (
		listener = t.Listener
		err      error
		conn     net.Conn
	)
	defer func() {
		//close all established connection
		//t.clientLock.RLock()
		//for i := range t.clients {
		//	t.removeClient(i)
		//}
		//t.clientLock.RUnlock()
		t.OnFinish(err)
	}()

	err = t.ctx.Err()
	for err == nil && t.ctx.Err() == nil {
		conn, err = listener.Accept()
		if err != nil {
			continue
		}
		logrus.Info("new connection is accepted,", conn.RemoteAddr().String())
		t.newTCPServerClient(conn)

		err = t.ctx.Err()

	}

}

func (t *TCPServer) removeClient(client string) {
	t.clientLock.Lock()
	c := t.clients[client]
	c.cancel()
	t.OnRemovingClient(client, c.authPkt.Username, c.localNetDev, c.tunDevProps)
	c.conn.Close()
	delete(t.clients, client)
	t.clientLock.Unlock()
}

func (t *TCPServer) newTCPServerClient(conn net.Conn) *tcpServerClient {
	defer t.clientLock.Unlock()
	t.clientLock.Lock()
	rtn := &tcpServerClient{
		server:        t,
		conn:          conn,
		fsm:           fsm.NewFSM(StateConnected, ServerFsmEvents, fsm.Callbacks{}),
		connectedTime: time.Now(),
	}
	rtn.ctx, rtn.cancel = context.WithCancel(t.ctx)
	t.clients[conn.RemoteAddr().String()] = rtn
	go rtn.readFromConn()
	return rtn
}

func (tsc *tcpServerClient) readFromConn() error {

	var (
		err          error
		currentState string
		pkt          *packets.MoleContainerPacket
		recvTimeout  time.Duration = tsc.server.AuthTimeout * 5
	)
	tsc.lastRecv = time.Now()
	defer func() {
		if err == nil {
			err = tsc.ctx.Err()
		}
		if currentState == StateAuthenticated {
			tsc.sendDisconnectRequest()
			tsc.event(EventSendDisconnectRequest)
			tsc.waitForSpecPacket(packets.DisconnectAcceptType, tsc.server.AuthTimeout)
			tsc.event(EventRecvDisconnectAccepted)
		}
		tsc.server.removeClient(tsc.conn.RemoteAddr().String())

	}()
	if err = tsc.authenticating(); err != nil {
		return err
	}

	currentState = tsc.fsm.Current()
	if currentState != StateAuthenticated {
		return err
	}

	for err == nil && tsc.ctx.Err() == nil && currentState == StateAuthenticated {
		if time.Since(tsc.lastRecv) > recvTimeout {
			err = ErrTimeout
			continue
		}
		pkt, err = tsc.readPacket()
		if err != nil {
			return err
		}
		if pkt == nil {
			continue
		}
		currentState = tsc.fsm.Current()
		tsc.lastRecv = time.Now()

		switch pkt.PacketType {
		case packets.DisconnectAcceptType:
			err = tsc.processDisconnecting(currentState)
			if err != nil {
				return err
			}
		case packets.DisconnectRequestType:
			err = tsc.processDisconnect(currentState)
			if err != nil {
				return err
			}
		case packets.EncapsulatedNetDevPacketType:
			err = tsc.processAuthenticated(currentState, pkt)
			if err != nil {
				return err
			}
		case packets.PingType:
			err = tsc.event(EventSendPing)
			if err != nil {
				return err
			}
			err = tsc.sendPong()
			if err != nil {
				return err
			}
		case packets.ReportType:
			tsc.processReport(pkt)
		default:
		}
		currentState = tsc.fsm.Current()
	}
	return err
}
func (tsc *tcpServerClient) processDisconnect(currentState string) error {
	var err error
	if !Contains([]string{StateDisconnecting, StateDisconnected, StateAuthenticated}, currentState) {
		return nil
	}
	err = tsc.event(EventRecvDisconnectRequest)
	if err != nil {
		logrus.Error("error in event processing", err)
		return err
	}
	// send accepted
	err = tsc.sendDisconnectAccepted()
	if err != nil {
		logrus.Error("error in sending msg", err)
		return err
	}
	err = tsc.event(EventSendDisconnectAccepted)
	if err != nil {
		logrus.Error("error in event procssing - send_disconnect", err)
		return err
	}
	return nil
}

func (tsc *tcpServerClient) processDisconnecting(currentState string) error {
	var err error
	if currentState == StateDisconnecting {
		err = tsc.event(EventRecvDisconnectAccepted)
		if err != nil {
			logrus.Error("error in event processing for State_disconnecting and event disconnect accepted", err)
			return err
		}
	}
	return nil
}
func (tsc *tcpServerClient) sendPacketToNet(pkt *packets.MoleContainerPacket) error {
	var err error

	var buf [10000]byte

	err = pkt.WriteTo(buf[:])
	if err != nil {
		return err
	}
	tsc.conn.SetWriteDeadline(time.Now().Add(time.Millisecond * 100))
	_, err = tsc.conn.Write(buf[:pkt.TotalLength()])
	if err != nil {
		logrus.Warn("error in sending packet out", err)
		return err
	}
	tsc.localReport.SentBytes += uint64(pkt.TotalLength())
	tsc.localReport.SentPackets++
	tsc.lastSent = time.Now()
	return nil
}

func (tsc *tcpServerClient) sendDisconnectRequest() error {
	n := time.Now()
	tsc.diconnectTime = &n
	dis := packets.NewDisconnectRequestPacket("request")
	cpkt := packets.NewMoleContainerPacket(&dis)
	return tsc.sendPacketToNet(&cpkt)

}
func (tsc *tcpServerClient) waitForSpecPacket(pktType packets.PacketType, timeout time.Duration) (*packets.MoleContainerPacket, error) {
	var (
		err       error
		pkt       *packets.MoleContainerPacket
		startTime = time.Now()
	)

	pkt, err = tsc.readPacket()
	for err == nil {
		if pkt != nil && pkt.PacketType == pktType {
			break
		}
		if time.Since(startTime) > timeout {
			return nil, ErrTimeout
		}
		if tsc.ctx.Err() != nil {
			return nil, tsc.ctx.Err()
		}
		pkt, err = tsc.readPacket()
	}
	return pkt, err
}
func (tsc *tcpServerClient) sendAuthRejectedPacket() error {
	auth := packets.NewAuthRejectPacket(packets.RejectCodeBadSecret, "random_string")
	cpkt := packets.NewMoleContainerPacket(&auth)
	return tsc.sendPacketToNet(&cpkt)
}
func (tsc *tcpServerClient) onSuccessAuthentication(authReq packets.AuthRequestPacket) error {
	var err error
	tsc.tunDevProps = nil // ToDo: maybe in future we decide to add some fw rules for this connection
	tsc.authPkt = authReq
	tsc.localNetDev, err = tsc.server.GetTunDev(authReq.Username, tsc.tunDevProps)
	if err != nil {
		return err
	}
	// make it enable
	go tsc.pipeLocalPacketsToRemote()

	return nil
}
func (tsc *tcpServerClient) pipeLocalPacketsToRemote() {
	const (
		extraBufSize = uint16(100)
	)
	var (
		buf         = make([]byte, tsc.server.MTU+extraBufSize)
		sendBuf     = make([]byte, tsc.server.MTU+extraBufSize)
		n           int
		err         error
		errNet      net.Error
		molePacket  packets.MoleContainerPacket
		encapPacket packets.EncapsulatedNetDevPacket
	)
	defer logrus.Info("exit from piping local to remote", tsc.conn.RemoteAddr().String())
	logrus.Info("start piping local dev net to ", tsc.conn.RemoteAddr().String())
	for {
		if tsc.localNetDev == nil {
			return
		}
		n, err = tsc.localNetDev.Read(buf)
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
		tsc.conn.SetWriteDeadline(time.Now().Add(time.Second))
		_, err = tsc.conn.Write(sendBuf[:molePacket.TotalLength()])
		if err != nil {
			if errors.As(err, &errNet) && errNet.Timeout() {
				continue
			}
			logrus.Error("msg ", " could not send over buffer", " err", err.Error(), " canceling context and closing connection")
			tsc.cancel()
			tsc.conn.Close()
			return
		}
	}

}
func isNetworkTimeout(err error) bool{
	var errNet net.Error
 	return errors.As(err, &errNet) && errNet.Timeout()
}

func (t *tcpServerClient) ReadNByteWithTimeout(buf []byte, timeout time.Duration) error {
	var (
		err error
		readBytes int
		m int
	)

	err = t.conn.SetReadDeadline(time.Now().Add(timeout))
	if err!=nil {
		return err
	}	
	readBytes, err = t.conn.Read(buf)
	if err!=nil {
		return err
	}
	for readBytes < len(buf) {
		err = t.conn.SetReadDeadline(time.Now().Add(timeout))
		if err!=nil {
			return err
		}			
		m, err = t.conn.Read(buf[readBytes:])
		if err!=nil {
			//because timeout in this step causes bad buffer, so its  not timeout, it means client is down
			if isNetworkTimeout(err){
				err = io.ErrNoProgress
			}
			return err
		}
		readBytes += m
	}
	return nil
}


func (tsc *tcpServerClient) authenticating() error {

	var (
		err     error
		authPkt *packets.AuthRequestPacket
		pkt     *packets.MoleContainerPacket
	)
	if !tsc.fsm.Is(StateConnected) {
		// because if it it other than connected, it means connection is authenticated
		return nil
	}
	pkt, err = tsc.waitForSpecPacket(packets.AuthRequestType, tsc.server.AuthTimeout)
	if err != nil && !errors.Is(err, ErrTimeout) {
		return err
	} else if errors.Is(err, ErrTimeout) {
		_ = tsc.event(EventInternalAuthTimeout)
		_ = tsc.sendDisconnectRequest()
		_ = tsc.event(EventSendDisconnectRequest)

		return err
	}
	authPkt, _ = pkt.MolePacket.(*packets.AuthRequestPacket)
	if !tsc.server.Authenticate(*authPkt) {
		err = tsc.event(EventInternalInvalidSecret)
		_ = tsc.sendAuthRejectedPacket()
		_ = tsc.sendDisconnectRequest()
		_ = tsc.event(EventSendDisconnectRequest)
		return err
	}

	err = tsc.event(EventInternalValidSecret)
	if err != nil {
		logrus.Error("error in event processing", err)
		return err
	}

	err = tsc.onSuccessAuthentication(*authPkt)
	if err != nil {
		logrus.Error("on launching client requirements", err)
		return err
	}

	err = tsc.sendAuthAcceptedPacket()
	return err
}

func (tsc *tcpServerClient) sendAuthAcceptedPacket() error {
	auth := packets.NewAuthAcceptPacket(packets.AcceptCodeOK, "random_string")
	cpkt := packets.NewMoleContainerPacket(&auth)
	return tsc.sendPacketToNet(&cpkt)
}

func (tsc *tcpServerClient) readPacket() (*packets.MoleContainerPacket, error) {
	var (
		err    error
		buf    []byte = make([]byte, tsc.server.MTU+extraBufSize)
		n      uint16
	)
	defer func(){
		if err!=nil {
			logrus.Warn("closing cause of : ", err)
		}
	}()
	err = tsc.ReadNByteWithTimeout(buf[:3], time.Second)
	if isNetworkTimeout(err){
		err = nil
		return nil, err
	}
	n = binary.BigEndian.Uint16(buf)
	if n > uint16(len(buf)) {
		logrus.Warn("bad serialized packet, with len: ", n, " closing connection")
		tsc.conn.Close()
		err = io.ErrShortBuffer 
		return nil, err
	}
	if n == 0 {
		logrus.Warn("received buf with len 0")
		err = nil
		return nil, err
	}
	err = tsc.ReadNByteWithTimeout(buf[3:n], time.Second)
	if err!=nil {
		if isNetworkTimeout(err){
			err = io.ErrNoProgress
		}
		return nil, err
	}

	rtn := &packets.MoleContainerPacket{}
	err = rtn.FromBytes(buf[:n])
	if err != nil {
		logrus.Warn(fmt.Sprintf("bad packet format : %+v", buf[:n]))
		//nolint:nilerr // bad packet format, ignore it
		return nil, err
	}

err = nil
	return rtn, nil
}

func (tsc *tcpServerClient) event(ev string) error {
	var err = tsc.fsm.Event(tsc.server.ctx, ev)
	if err != nil && !errors.Is(err, fsm.NoTransitionError{}) {
		logrus.Warn("msg", "error in FSM event", "raddr", tsc.conn.RemoteAddr().String(), "err", err)
		return err
	}
	return nil
}

func (tsc *tcpServerClient) sendDisconnectAccepted() error {
	dis := packets.NewDisconnectAcceptedPacket("accepted")
	cpkt := packets.NewMoleContainerPacket(&dis)
	return tsc.sendPacketToNet(&cpkt)
}

func (tsc *tcpServerClient) processAuthenticated(currentState string, pkt *packets.MoleContainerPacket) error {
	var err error
	if currentState == StateAuthenticated && tsc.localNetDev != nil {
		_, err = pkt.MolePacket.(*packets.EncapsulatedNetDevPacket).WriteContentTo(tsc.localNetDev)
		if err != nil {
			return err
		}
	}
	return nil
}

func (tsc *tcpServerClient) sendPong() error {
	const (
		minPongMsgLen = 10
		maxPongMsgLen = 100
	)
	pingMsgLen := rand.Intn(maxPongMsgLen-minPongMsgLen) + minPongMsgLen
	buf := make([]byte, pingMsgLen)
	rand.Read(buf)
	pkt := packets.NewPongPacket(string(buf))
	cpkt := packets.NewMoleContainerPacket(&pkt)
	return tsc.sendPacketToNet(&cpkt)
}

func (tsc *tcpServerClient) processReport(pkt *packets.MoleContainerPacket) {
	// update remote packet
	if tsc.remoteReport.SystemTime <= pkt.MolePacket.(*packets.ReportPacket).SystemTime {
		tsc.remoteReport = *pkt.MolePacket.(*packets.ReportPacket)
	}
}
