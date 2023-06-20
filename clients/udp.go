package clients

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"github.com/looplab/fsm"
	"github.com/msdmazarei/mole/packets"
	. "github.com/msdmazarei/mole/shared"
	"github.com/sirupsen/logrus"
)

type (
	UDPClientPB struct {
		UDPParams
		Username, Secret string
		NetDevProps      *TunDevProps
		PingInterval     time.Duration
	}
	UDPClient struct {
		ctx    context.Context
		cancel context.CancelFunc
		fsm    *fsm.FSM
		UDPClientPB
		lastPingTime    time.Time
		lastRecvPktTime time.Time
	}
)

const (
	extraBufferBytes = 1000
	writeTimeout     = time.Millisecond * 10
)

var ()

func NewUDPClient(ctx context.Context, params UDPClientPB) (*UDPClient, error) {
	rtn := UDPClient{
		UDPClientPB: params,
	}

	rtn.ctx, rtn.cancel = context.WithCancel(ctx)
	rtn.fsm = fsm.NewFSM(StateConnected, ClientFsmEvents, fsm.Callbacks{})
	go rtn.start()
	return &rtn, nil
}

func (u *UDPClient) event(event string) error {
	var (
		err error
	)
	ostate := u.fsm.Current()

	err = u.fsm.Event(u.ctx, event)
	if err != nil && !errors.Is(err, fsm.NoTransitionError{}) {
		logrus.Warn("msg", "error in FSM event", "err=", err)
		return err
	}
	if u.fsm.Current() == StateDisconnected {
		logrus.Info("event", event, "odtate", ostate)
		panic(ostate)
	}
	return nil
}

func (u *UDPClient) start() {
	var (
		err    error
		netDev io.ReadWriteCloser
	)
	logrus.Info("msg", "client is started")
	defer func() {
		logrus.Info("msg", "exiting client processing", "ctx.err=", u.ctx.Err(), "err=", err, u.checkContext())

		if u.ctx.Err() == nil {
			u.cancel()
		}
		if u.OnFinish != nil {
			u.OnFinish(err)
		}
	}()

	// try to authenticate
	if u.fsm.Current() == StateConnected {
		logrus.Info("msg", "authenticating...")
		err = u.authenticate()
		if err != nil {
			return
		}
	}

	logrus.Info("authenticated")
	netDev, err = u.GetTunDev(u.Username, u.NetDevProps)
	if err != nil {
		return
	}
	u.lastRecvPktTime = time.Now()

	go u.captureLocalDev(netDev)

	for u.checkContext() == nil && u.fsm.Current() != StateDisconnected {
		// sending ping if its time.
		err = u.sendPing()
		if err != nil {
			logrus.Error("send ping error", err)
			return
		}

		err = u.fetchAndProcessPkt(netDev)
		if err != nil {
			logrus.Error("fetch and process packet error", err)
			return
		}

		err = u.checkIfStillConnected()
		if err != nil {
			logrus.Warn("error in still connect checking", err.Error())
			return
		}
	}
	logrus.Error("for codition error", "state", u.fsm.Current())
}
func (u *UDPClient) fetchAndProcessPkt(netDev io.ReadWriteCloser) error {
	var (
		err    error
		netErr net.Error
		cpkt   packets.MoleContainerPacket
		n      int
		buf    = make([]byte, u.MTU+extraBufferBytes)
	)
	err = u.Conn.SetReadDeadline(time.Now().Add(time.Millisecond))
	if err != nil {
		return err
	}
	n, err = u.Conn.Read(buf)
	if err != nil {
		if !errors.As(err, &netErr) {
			return err
		}
		if netErr != nil && !netErr.Timeout() {
			logrus.Error("read error", err)
			return err
		}
	}

	if n == 0 {
		return nil
	}
	err = cpkt.FromBytes(buf[:n])
	if err != nil {
		//nolint:nilerr // bad packet format, ignore it
		return nil
	}

	u.lastRecvPktTime = time.Now()
	err = u.processPacket(netDev, &cpkt)
	if err != nil {
		logrus.Error("process packet error", err, " netDev:", netDev)
	}
	return err
}
func (u *UDPClient) checkIfStillConnected() error {
	var err error
	if time.Since(u.lastRecvPktTime) < u.AuthTimeout {
		return nil
	}

	logrus.Warn("server didnt send packet for long time last time was", time.Now(), u.lastRecvPktTime)

	err = u.event(EventInternalPacketRecieveTimeout)
	if err != nil {
		return err
	}

	if u.fsm.Current() == StateDisconnecting {
		err = u.sendDisReqPkt()
		if err != nil {
			return err
		}
		err = u.event(EventSendDisconnectRequest)
		if err != nil {
			return err
		}
	}
	return ErrServerIsNotResponding
}
func (u *UDPClient) processPacket(netDev io.Writer, cpkt *packets.MoleContainerPacket) error {
	var err error
	err = u.event(RecvPktTypeToEvent[cpkt.PacketType])
	if err != nil {
		return err
	}
	switch cpkt.PacketType {
	case packets.EncapsulatedNetDevPacketType:
		encap, _ := cpkt.MolePacket.(*packets.EncapsulatedNetDevPacket)
		_, err = encap.WriteContentTo(netDev)
		return err
	case packets.DisconnectRequestType:
		err = u.sendDisAcceptedPkt()
		if err != nil {
			return err
		}
		err = u.event(EventSendDisconnectAccepted)
		if err != nil {
			return err
		}

	case packets.DisconnectAcceptType:
		// do nothing.

	case packets.PingType:
		return u.sendPong()
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
func (u *UDPClient) sendPing() error {
	var err error
	if time.Since(u.lastPingTime) > u.PingInterval {
		ping := packets.NewPingPacket("ping")
		err = u.sendPkt(&ping)
		if err != nil {
			return err
		}
		u.lastPingTime = time.Now()
	}
	return nil
}
func (u *UDPClient) captureLocalDev(netDev io.Reader) {
	var (
		n         int
		err       error
		buf       = make([]byte, u.MTU+extraBufferBytes)
		sentBuf   = make([]byte, len(buf))
		sentBytes = uint64(0)
		sentPkts  = uint64(0)
	)
	defer logrus.Info("exiting from capture local dev", err)
	defer u.cancel()
	logrus.Info("start piping netdev packet to udp connection")
	for u.checkContext() == nil {
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
		err = u.Conn.SetWriteDeadline(time.Now().Add(u.AuthTimeout))
		if err != nil {
			logrus.Error("error in setting UDP deadline:", err)
			return
		}
		_, err = u.Conn.Write(sentBuf[:cpkt.TotalLength()])
		if err != nil {
			logrus.Error("error in sending UDP:", err)
			return
		}
		sentPkts++
		sentBytes += uint64(cpkt.TotalLength())
	}
}
func (u *UDPClient) checkContext() error {
	// maybe its better to check it every 100ms not at every single moment.
	return u.ctx.Err()
}
func (u *UDPClient) authenticate() error {
	var (
		buf    [1000]byte
		cpkt   packets.MoleContainerPacket
		e, err error
		n      int
	)

	startTime := time.Now()
	i := 0
	for time.Since(startTime) < u.AuthTimeout {
		i++
		err = u.sendAuthRequest()
		if err != nil {
			logrus.Error("sending auth request faces error:", err)
			return err
		}

		n, err = u.authNetRead(i, buf[:])
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
			if e = u.event(EventRecvAuthRejected); e != nil {
				return e
			}
			return ErrUnauthorized
		case packets.AuthAcceptType:
			if e = u.event(EventRecvAuthAccepted); e != nil {
				return e
			}

			return nil
		default:
			continue
		}
	}
	if e = u.event(EventInternalAuthTimeout); e != nil {
		return e
	}

	return ErrServerIsNotResponding
}
func (u *UDPClient) authNetRead(i int, buf []byte) (int, error) {
	var (
		err    error
		n      int
		netErr net.Error
		conn   = u.Conn
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
func (u *UDPClient) sendAuthRequest() error {
	auth := packets.NewAuhRequestPacket(u.Username, u.Secret)
	cpkt := packets.NewMoleContainerPacket(&auth)
	buf := make([]byte, cpkt.TotalLength())
	e := cpkt.WriteTo(buf)
	if e != nil {
		return e
	}
	_ = u.Conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_, e = u.Conn.Write(buf)
	return e
}

func (u *UDPClient) sendPong() error {
	pong := packets.NewPongPacket("pong")
	return u.sendPkt(&pong)
}

func (u *UDPClient) sendDisReqPkt() error {
	dis := packets.NewDisconnectRequestPacket("disconnect")
	return u.sendPkt(&dis)
}

func (u *UDPClient) sendDisAcceptedPkt() error {
	dis := packets.NewDisconnectAcceptedPacket("accepted")
	return u.sendPkt(&dis)
}

func (u *UDPClient) sendPkt(pkt packets.MolePacketer) error {
	var (
		netError net.Error
	)
	cpkt := packets.NewMoleContainerPacket(pkt)
	buf := make([]byte, cpkt.TotalLength())
	e := cpkt.WriteTo(buf)
	if e != nil {
		return e
	}
	_ = u.Conn.SetWriteDeadline(time.Now().Add(time.Millisecond))
	_, e = u.Conn.Write(buf)
	if errors.As(e, &netError) && netError != nil && netError.Timeout() {
		e = nil
	}
	return e
}
