package clients

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/looplab/fsm"
	"github.com/msdmazarei/mole/packets"
	. "github.com/msdmazarei/mole/shared"
	"github.com/sirupsen/logrus"
)

type (
	TCPClientPB struct {
		TCPParams
		Username, Secret string
		NetDevProps      *TunDevProps
		PingInterval     time.Duration
	}
	TCPClient struct {
		ctx    context.Context
		cancel context.CancelFunc
		fsm    *fsm.FSM
		TCPClientPB
		lastPingTime    time.Time
		lastRecvPktTime time.Time
	}
)

func NewTCPClient(ctx context.Context, params TCPClientPB) (*TCPClient, error) {
	rtn := TCPClient{
		TCPClientPB: params,
	}

	rtn.ctx, rtn.cancel = context.WithCancel(ctx)
	rtn.fsm = fsm.NewFSM(StateConnected, ClientFsmEvents, fsm.Callbacks{})
	go rtn.start()
	return &rtn, nil
}

func (t *TCPClient) event(event string) error {
	var (
		err error
	)
	ostate := t.fsm.Current()

	err = t.fsm.Event(t.ctx, event)
	if err != nil && !errors.Is(err, fsm.NoTransitionError{}) {
		logrus.Warn("msg", "error in FSM event", "err=", err)
		return err
	}
	if t.fsm.Current() == StateDisconnected {
		logrus.Info("event", event, "odtate", ostate)
	}
	return nil
}

func (t *TCPClient) start() {
	var (
		err    error
		netDev io.ReadWriteCloser
	)
	logrus.Info("msg", "client is started")
	defer func() {
		logrus.Info("msg", "exiting client processing", "ctx.err=", t.ctx.Err(), "err=", err, t.checkContext())

		if t.ctx.Err() == nil {
			t.cancel()
		}
		if t.OnFinish != nil {
			t.OnFinish(err)
		}
	}()

	// try to authenticate
	if t.fsm.Current() == StateConnected {
		logrus.Info("msg", "authenticating...")
		err = t.authenticate()
		if err != nil {
			return
		}
	}

	logrus.Info("authenticated")
	netDev, err = t.GetTunDev(t.Username, t.NetDevProps)
	if err != nil {
		return
	}
	t.lastRecvPktTime = time.Now()

	go t.captureLocalDev(netDev)

	for t.checkContext() == nil && t.fsm.Current() != StateDisconnected {
		// sending ping if its time.
		err = t.sendPing()
		if err != nil {
			logrus.Error("send ping error", err)
			return
		}

		err = t.fetchAndProcessPkt(netDev)
		if err != nil {
			logrus.Error("fetch and process packet error: ", err)
			return
		}

		err = t.checkIfStillConnected()
		if err != nil {
			logrus.Warn("error in still connect checking", err.Error())
			return
		}
	}
	logrus.Error("for codition error", "state", t.fsm.Current())
}
func isNetworkTimeout(err error) bool {
	var errNet net.Error
	return errors.As(err, &errNet) && errNet.Timeout()
}
func (t *TCPClient) ReadNByteWithTimeout(buf []byte, timeout time.Duration) error {
	var (
		err       error
		readBytes int
		m         int
	)

	err = t.Conn.SetReadDeadline(time.Now().Add(timeout))
	if err != nil {
		return err
	}
	readBytes, err = t.Conn.Read(buf)
	if err != nil {
		return err
	}
	for readBytes < len(buf) {
		err = t.Conn.SetReadDeadline(time.Now().Add(timeout))
		if err != nil {
			return err
		}
		m, err = t.Conn.Read(buf[readBytes:])
		if err != nil {
			//because timeout in this step causes bad buffer, so its  not timeout, it means server is down
			if isNetworkTimeout(err) {
				err = io.ErrNoProgress
			}
			return err
		}
		readBytes += m
	}
	return nil
}
func (t *TCPClient) fetchAndProcessPkt(netDev io.ReadWriteCloser) error {
	var (
		err  error
		cpkt packets.MoleContainerPacket
		n    uint16
		buf  = make([]byte, t.MTU+extraBufferBytes)
	)
	defer func() {
		if err != nil {
			logrus.Warn("closing connection cause of :", err)
			netDev.Close() // not sure if should be here
		}

	}()
	err = t.ReadNByteWithTimeout(buf[:3], time.Millisecond)
	if isNetworkTimeout(err) {
		err = nil
		return err
	}

	n = binary.BigEndian.Uint16(buf)
	if n == 0 {
		return nil
	}
	if n > uint16(len(buf)) {
		logrus.Warn("received packet with len: ", n)
		err = io.ErrShortBuffer
		return err
	}
	err = t.ReadNByteWithTimeout(buf[3:n], time.Second)
	if err != nil {
		if isNetworkTimeout(err) {
			err = io.ErrNoProgress
		}
		return err
	}

	err = cpkt.FromBytes(buf[:n])
	if err != nil {
		logrus.Warn(fmt.Sprintf("bad packet format : %+v", buf[:n]))
		//nolint:nilerr // bad packet format, ignore it
		return err
	}

	t.lastRecvPktTime = time.Now()
	err = t.processPacket(netDev, &cpkt)
	if err != nil {
		logrus.Error("process packet error", err, " netDev:", netDev)
	}
	return err
}
func (t *TCPClient) checkIfStillConnected() error {
	var err error
	if time.Since(t.lastRecvPktTime) < t.AuthTimeout {
		return nil
	}

	logrus.Warn("server didnt send packet for long time last time was", time.Now(), t.lastRecvPktTime)

	err = t.event(EventInternalPacketRecieveTimeout)
	if err != nil {
		return err
	}

	if t.fsm.Current() == StateDisconnecting {
		err = t.sendDisReqPkt()
		if err != nil {
			return err
		}
		err = t.event(EventSendDisconnectRequest)
		if err != nil {
			return err
		}
	}
	return ErrServerIsNotResponding
}
func (t *TCPClient) processPacket(netDev io.Writer, cpkt *packets.MoleContainerPacket) error {
	var err error
	err = t.event(RecvPktTypeToEvent[cpkt.PacketType])
	if err != nil {
		return err
	}
	switch cpkt.PacketType {
	case packets.EncapsulatedNetDevPacketType:
		encap, _ := cpkt.MolePacket.(*packets.EncapsulatedNetDevPacket)
		_, err = encap.WriteContentTo(netDev)
		return err
	case packets.DisconnectRequestType:
		err = t.sendDisAcceptedPkt()
		if err != nil {
			return err
		}
		err = t.event(EventSendDisconnectAccepted)
		if err != nil {
			return err
		}

	case packets.DisconnectAcceptType:
		// do nothing.

	case packets.PingType:
		return t.sendPong()
	case packets.PongType:
		// do nothing.
	case packets.ReportType:
		// do nothing.
	case packets.AuthRequestType, packets.AuthRejectType, packets.AuthAcceptType, packets.ContainerPacketType:
		// do nothing.
	default:
		logrus.Error("msg", "unknown packet type", "cpkt", cpkt)
	}
	return nil
}
func (t *TCPClient) sendPing() error {
	var err error
	if time.Since(t.lastPingTime) > t.PingInterval {
		ping := packets.NewPingPacket("ping")
		err = t.sendPkt(&ping)
		if err != nil {
			return err
		}
		t.lastPingTime = time.Now()
	}
	return nil
}
func (t *TCPClient) captureLocalDev(netDev io.Reader) {
	var (
		n         int
		err       error
		errNet    net.Error
		buf       = make([]byte, t.MTU+extraBufferBytes)
		sentBuf   = make([]byte, len(buf))
		sentBytes = uint64(0)
		sentPkts  = uint64(0)
	)
	defer logrus.Info("exiting from capture local dev", err)
	defer t.cancel()
	logrus.Info("start piping netdev packet to TCP connection")
	for t.checkContext() == nil {
		n, err = netDev.Read(buf)
		if err != nil {
			logrus.Error("error in reading netdev:", err)
			return
		}
		encap := packets.NewEncapsulateNetDevPacket(buf[:n])
		cpkt := packets.NewMoleContainerPacket(&encap)
		err = cpkt.WriteTo(sentBuf)
		if err != nil {
			logrus.Error("can not convert encap packet cause of:", err)
			continue
		}
		err = t.Conn.SetWriteDeadline(time.Now().Add(t.AuthTimeout))
		if err != nil {
			logrus.Error("error in setting TCP deadline:", err)
			return
		}
		_, err = t.Conn.Write(sentBuf[:cpkt.TotalLength()])
		if err != nil {
			if errors.As(err, &errNet) && errNet.Timeout() {
				continue
			}
			logrus.Error("error in sending TCP:", err)
			return
		}
		sentPkts++
		sentBytes += uint64(cpkt.TotalLength())
	}
}
func (t *TCPClient) checkContext() error {
	// maybe its better to check it every 100ms not at every single moment.
	return t.ctx.Err()
}
func (t *TCPClient) authenticate() error {
	var (
		buf    [1000]byte
		cpkt   packets.MoleContainerPacket
		e, err error
		n      int
	)

	startTime := time.Now()
	i := 0
	for time.Since(startTime) < t.AuthTimeout {
		i++
		err = t.sendAuthRequest()
		if err != nil {
			logrus.Error("sending auth request faces error:", err)
			return err
		}

		n, err = t.authNetRead(i, buf[:])
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}

		if cpkt.FromBytes(buf[:n]) != nil {
			// wrong packet is received.
			continue
		}

		switch cpkt.PacketType {
		case packets.AuthRejectType:
			if e = t.event(EventRecvAuthRejected); e != nil {
				return e
			}
			return ErrUnauthorized
		case packets.AuthAcceptType:
			if e = t.event(EventRecvAuthAccepted); e != nil {
				return e
			}

			return nil
		default:
			continue
		}
	}
	if e = t.event(EventInternalAuthTimeout); e != nil {
		return e
	}

	return ErrServerIsNotResponding
}
func (t *TCPClient) authNetRead(i int, buf []byte) (int, error) {
	var (
		err    error
		n      int
		netErr net.Error
		conn   = t.Conn
	)

	err = conn.SetReadDeadline(time.Now().Add(time.Millisecond * 100 * time.Duration(i)))
	if err != nil {
		logrus.Error("setting connection deadline failed:", err)
		return 0, err
	}

	n, err = conn.Read(buf)
	if errors.Is(err, context.DeadlineExceeded) {
		return 0, nil
	}
	if errors.As(err, &netErr) && netErr.Timeout() {
		return 0, nil
	}

	if err != nil {
		return 0, err
	}
	return n, nil
}
func (t *TCPClient) sendAuthRequest() error {
	auth := packets.NewAuhRequestPacket(t.Username, t.Secret)
	cpkt := packets.NewMoleContainerPacket(&auth)
	buf := make([]byte, cpkt.TotalLength())
	e := cpkt.WriteTo(buf)
	if e != nil {
		return e
	}
	_ = t.Conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_, e = t.Conn.Write(buf)
	return e
}

func (t *TCPClient) sendPong() error {
	pong := packets.NewPongPacket("pong")
	return t.sendPkt(&pong)
}

func (t *TCPClient) sendDisReqPkt() error {
	dis := packets.NewDisconnectRequestPacket("disconnect")
	return t.sendPkt(&dis)
}

func (t *TCPClient) sendDisAcceptedPkt() error {
	dis := packets.NewDisconnectAcceptedPacket("accepted")
	return t.sendPkt(&dis)
}

func (t *TCPClient) sendPkt(pkt packets.MolePacketer) error {
	var (
		netError net.Error
	)
	cpkt := packets.NewMoleContainerPacket(pkt)
	buf := make([]byte, cpkt.TotalLength())
	e := cpkt.WriteTo(buf)
	if e != nil {
		return e
	}
	_ = t.Conn.SetWriteDeadline(time.Now().Add(time.Millisecond))
	_, e = t.Conn.Write(buf)
	if errors.As(e, &netError) && netError != nil && netError.Timeout() {
		e = nil
	}
	return e
}
