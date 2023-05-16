package streams

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/looplab/fsm"
	"github.com/msdmazarei/mole/packets"
)

type MolePeerType byte

const (
	MolePeerUknown MolePeerType = iota
	MolePeerClient
	MolePeerServer
)

type MolePeerPB struct {
	NetworkIO                 io.ReadWriter
	NetDevIO                  io.ReadWriter
	Secret                    string
	Context                   context.Context
	StartState                *string
	OnClose                   context.CancelCauseFunc
	AuthTimeout               *time.Duration
	AuthRetry                 *time.Duration
	DisconnectAcceptedTimeout *time.Duration
	DisconnectRetry           *time.Duration
	PingRetry                 *time.Duration
	DropConnectionTimeout     *time.Duration
	ReportInterval            *time.Duration
	MolePeerType              *MolePeerType
}

type MolePeer struct {
	net_stream   io.ReadWriter
	dev_stream   io.ReadWriter
	myCtx        context.Context
	myCancel     context.CancelFunc
	start_state  string
	onClose      context.CancelCauseFunc
	molePeerType MolePeerType

	//below fields only should be accessed by startProcessing goroutine not
	//anyonce else
	npp                     StreamParser
	state                   *fsm.FSM
	secret                  string
	StopError               error
	auth_timeout            time.Duration
	auth_retry              time.Duration
	disconnect_duration     time.Duration
	disconnect_retry        time.Duration
	ping_retry              time.Duration
	drop_connection_timeout time.Duration
	report_interval         time.Duration
	remote_report           packets.ReportPacket

	//accessible by report go routine
	report_update chan<- packets.ReportPacket
	report_read   <-chan packets.ReportPacket
}

func NewMolePeer(pb MolePeerPB) *MolePeer {
	rtn := &MolePeer{
		net_stream: pb.NetworkIO,
		dev_stream: pb.NetDevIO,
	}
	//	var pcancel context.CancelFunc
	rtn.myCtx, _ = context.WithCancel(pb.Context)
	rtn.myCancel = func(){
		panic("no one should cancel this context")
	}
	if pb.StartState == nil {
		rtn.start_state = state_connected
	} else {
		rtn.start_state = *pb.StartState
	}
	if pb.AuthTimeout == nil {
		rtn.auth_timeout = time.Second * 15
	} else {
		rtn.auth_timeout = *pb.AuthTimeout
	}
	if pb.AuthRetry == nil {
		rtn.auth_retry = time.Second * 3
	} else {
		rtn.auth_retry = *pb.AuthRetry
	}
	if pb.DisconnectAcceptedTimeout == nil {
		rtn.disconnect_duration = time.Second * 5
	} else {
		rtn.disconnect_duration = *pb.DisconnectAcceptedTimeout
	}
	if pb.DisconnectRetry == nil {
		rtn.disconnect_retry = time.Second
	} else {
		rtn.disconnect_retry = *pb.DisconnectRetry
	}
	if pb.PingRetry == nil {
		rtn.ping_retry = time.Second * 5
	} else {
		rtn.ping_retry = *pb.PingRetry
	}
	if pb.DropConnectionTimeout == nil {
		rtn.drop_connection_timeout = 5 * rtn.ping_retry
	} else {
		rtn.drop_connection_timeout = *pb.DropConnectionTimeout
	}
	if pb.ReportInterval == nil {
		rtn.report_interval = 5 * time.Second
	} else {
		rtn.report_interval = *pb.ReportInterval
	}
	if pb.MolePeerType == nil {
		rtn.molePeerType = MolePeerClient
	} else {
		rtn.molePeerType = *pb.MolePeerType
	}
	if rtn.molePeerType == MolePeerUknown {
		return nil
	}

	rtn.remote_report = packets.NewReportPacket(uint64(time.Now().Unix()), 0, 0, 0, 0)
	rtn.onClose = pb.OnClose

	rtn.secret = pb.Secret
	go rtn.startProcessing()
	return rtn
}
func (mc *MolePeer) report(ctx context.Context) (<-chan packets.ReportPacket, chan<- packets.ReportPacket) {
	r := make(chan packets.ReportPacket)
	u := make(chan packets.ReportPacket)
	go func() {
		local_report := packets.NewReportPacket(uint64(time.Now().Unix()), 0, 0, 0, 0)
		done := ctx.Done()

		for {
			select {
			case <-done:
				return
			case r := <-u:
				local_report.RecvBytes += r.RecvBytes
				local_report.RecvPackets += r.RecvPackets
				local_report.SentBytes += r.SentBytes
				local_report.SentPackets += r.SentPackets
			case r <- local_report:
				continue

			}
		}

	}()

	return r, u
}
func (mc *MolePeer) startProcessing() {
	var (
		err                     error           = nil
		done                    <-chan struct{} = mc.myCtx.Done()
		keep_loop               bool            = true
		piping_local_dev_err    chan error      = make(chan error)
		ping_retry              *time.Ticker
		connection_drop_watcher *time.Ticker
		send_report_tick        *time.Ticker
		last_recv_packet        time.Time = time.Now()
		rpt_ctx                 context.Context
		rpt_cancel              context.CancelFunc
	)
	rpt_ctx, rpt_cancel = context.WithCancel(context.Background())

	defer func() {
		//finally call onclose callback
		cstate := mc.state.Current()
		if cstate == state_authenticated || cstate == state_unauthorized {
			mc.disconnect()
		}
		fmt.Printf("err: %+v stoperr: %+v\n", err, mc.StopError)

		mc.onClose(err)
		rpt_cancel()
	}()
	defer mc.myCancel()
	mc.npp = NewStreamParser(mc.myCtx, mc.net_stream, mc.onNetPaserClose)
	mc.state = mc.setUpFSM(mc.start_state)
	mc.report_read, mc.report_update = mc.report(rpt_ctx)

	if !mc.state.Is(state_connected) && !mc.state.Is(state_authenticated) {
		return
	}
	err = mc.authenticating()
	if err != nil {
		return
	}

	//authenticate
	ping_retry = time.NewTicker(mc.ping_retry)
	defer ping_retry.Stop()

	connection_drop_watcher = time.NewTicker(mc.drop_connection_timeout)
	defer connection_drop_watcher.Stop()

	send_report_tick = time.NewTicker(mc.report_interval)
	defer send_report_tick.Stop()

	go mc.sendLocalDevToNet(piping_local_dev_err)
	for keep_loop {
		select {
		case err = <-piping_local_dev_err:
			fmt.Print("local dev reading error\n")

			keep_loop = false
		case <-done:
			fmt.Print("ctx is done\n")

			keep_loop = false
			continue
		case <-ping_retry.C:
			if time.Since(last_recv_packet).Seconds() <= mc.ping_retry.Seconds() {
				continue
			}
			mc.sendPing()
		case <-connection_drop_watcher.C:
			if time.Since(last_recv_packet).Seconds() > mc.drop_connection_timeout.Seconds() {
				fmt.Print("no activity\n")
				keep_loop = false
				err = ErrConnectionTimeout
			}
		case <-send_report_tick.C:
			err = mc.sendReport()
		case pkt, more := <-mc.npp.ParsedPacketChan:
			if !more {
				// there is no more packet, connection is closed or got an error
				fmt.Print("no more received packet\n")
				keep_loop = false
				continue
			}
			last_recv_packet = time.Now()
			mc.report_update <- packets.NewReportPacket(0, 0, 1, 0, uint64(pkt.TotalLength()))
			err = mc.event(recvPktTypeToEvent[pkt.PacketType])
			if err != nil {
				fmt.Print("event update failed\n")
				keep_loop = false
				continue
			}

			err = mc.processReceivedPacket(&pkt)
			if err != nil {
				fmt.Print("processing received packet failed\n")
				keep_loop = false
				continue
			}
			if mc.state.Is(state_disconnected) {
				fmt.Print("disconnected\n")
				keep_loop = false
			}
		}
	}
}
func (mc *MolePeer) disconnect() {
	cstate := mc.state.Current()
	if mc.isClient() && cstate != state_authenticated && cstate != state_authenticating {
		return
	}
	if mc.isServer() && (cstate == state_disconnected || cstate == state_disconnecting) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), mc.disconnect_duration)
	defer cancel()
	mc.sendDisconnectRequest()
	mc.state.Event(ctx, event_send_disconnect_request)
	retry_timer := time.NewTicker(mc.disconnect_retry)
	defer retry_timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-retry_timer.C:
			mc.sendDisconnectRequest()
			mc.state.Event(ctx, event_send_disconnect_request)

		case pkt, more := <-mc.npp.ParsedPacketChan:
			mc.state.Event(ctx, recvPktTypeToEvent[pkt.PacketType])
			if mc.state.Is(state_disconnected) {
				return
			}
			if !more {
				return
			}

		}
	}
}
func (mc *MolePeer) processReceivedPacket(pkt *packets.MoleContainerPacket) (err error) {
	current_state := mc.state.Current()
	defer func() {
		if err != nil && !errors.Is(err, fsm.NoTransitionError{}) {
			err = nil
		}
	}()
	switch pkt.PacketType {
	case packets.DisconnectRequestType:
		if current_state == state_disconnecting || current_state == state_disconnected {
			//send accepted
			err = mc.sendDisconnectAccepted()
			if err != nil {
				return
			}
			err = mc.event(event_send_disconnect_accepted)
		}
	case packets.EncapsulatedNetDevPacketType:
		if current_state == state_authenticated {
			_, err = pkt.MolePacket.(*packets.EncapsulatedNetDevPacket).WriteContentTo(mc.dev_stream)

		}
	case packets.PingType:
		err = mc.event(event_send_ping)
		if err != nil {
			return
		}
		err = mc.sendPong()
	case packets.ReportType:
		//update remote packet
		if mc.remote_report.SystemTime <= pkt.MolePacket.(*packets.ReportPacket).SystemTime {
			mc.remote_report = *pkt.MolePacket.(*packets.ReportPacket)
		}
	default:
		break
	}
	return
}
func (mc *MolePeer) Stop() {
	mc.myCancel()
}
func (mc *MolePeer) sendLocalDevToNet(err_chan chan error) {
	var (
		buf   [5000]byte
		encap packets.EncapsulatedNetDevPacket
		conta packets.MoleContainerPacket
		e     error
		n     int
	)
	for {
		n, e = mc.dev_stream.Read(buf[:])
		if e != nil {
			err_chan <- e
			return
		}
		encap = packets.NewEncapsulateNetDevPacket(buf[:n])
		conta = packets.NewMoleContainerPacket(&encap)
		_, e = conta.Write(mc.net_stream)
		mc.report_update <- packets.NewReportPacket(0, 1, 0, uint64(conta.TotalLength()), 0)
		if e != nil {
			err_chan <- e
			return
		}
	}

}
func (mc *MolePeer) sendPacketToNet(pkt *packets.MoleContainerPacket) error {
	_, e := pkt.Write(mc.net_stream)
	if e != nil {
		mc.StopError = e
		mc.myCancel()
	}
	mc.report_update <- packets.NewReportPacket(0, 1, 0, uint64(pkt.TotalLength()), 0)
	return e
}
func (mc *MolePeer) sendReport() error {
	pkt := packets.ReportPacket(<-mc.report_read)
	pkt.SystemTime = uint64(time.Now().UnixMicro())
	cpkt := packets.NewMoleContainerPacket(&pkt)
	return mc.sendPacketToNet(&cpkt)
}
func (mc *MolePeer) sendPing() error {
	ping_msg_len := rand.Intn(100) + 10
	buf := make([]byte, ping_msg_len)
	rand.Read(buf)
	pkt := packets.NewPingPacket(string(buf))
	cpkt := packets.NewMoleContainerPacket(&pkt)
	return mc.sendPacketToNet(&cpkt)
}
func (mc *MolePeer) sendPong() error {
	ping_msg_len := rand.Intn(100) + 10
	buf := make([]byte, ping_msg_len)
	rand.Read(buf)
	pkt := packets.NewPongPacket(string(buf))
	cpkt := packets.NewMoleContainerPacket(&pkt)
	return mc.sendPacketToNet(&cpkt)
}

func (mc *MolePeer) sendDisconnectRequest() error {
	dis := packets.NewDisconnectRequestPacket("request")
	cpkt := packets.NewMoleContainerPacket(&dis)
	return mc.sendPacketToNet(&cpkt)
}

func (mc *MolePeer) sendDisconnectAccepted() error {
	dis := packets.NewDisconnectAcceptedPacket("accepted")
	cpkt := packets.NewMoleContainerPacket(&dis)
	return mc.sendPacketToNet(&cpkt)
}
func (mc *MolePeer) sendAuthRequestPacket() error {
	auth := packets.NewAuhRequestPacket(mc.secret)
	cpkt := packets.NewMoleContainerPacket(&auth)
	return mc.sendPacketToNet(&cpkt)
}
func (mc *MolePeer) sendAuthAcceptedPacket() error {
	auth := packets.NewAuthAcceptPacket(packets.AcceptCodeOK, "random_string")
	cpkt := packets.NewMoleContainerPacket(&auth)
	return mc.sendPacketToNet(&cpkt)
}
func (mc *MolePeer) sendAuthRejectedPacket() error {
	auth := packets.NewAuthRejectPacket(packets.RejectCodeBadSecret, "random_string")
	cpkt := packets.NewMoleContainerPacket(&auth)
	return mc.sendPacketToNet(&cpkt)
}

func (mc *MolePeer) event(event string) (err error) {
	err = mc.state.Event(mc.myCtx, event)
	if err != nil && errors.Is(err, fsm.NoTransitionError{}) {
		err = nil
	}
	return
}
func (mc *MolePeer) isServer() bool {
	return mc.molePeerType == MolePeerServer
}
func (mc *MolePeer) isClient() bool {
	return mc.molePeerType == MolePeerClient
}
func (mc *MolePeer) sendAuthAcceptedIfServer() (err error) {
	err = nil
	if !mc.isServer() {
		return
	}
	err = mc.sendAuthAcceptedPacket()
	if err != nil {
		err = mc.event(event_send_auth_accepted)
	}
	return
}
func (mc *MolePeer) sendAuthRejectedIfServer() (err error) {
	err = nil
	if !mc.isServer() {
		return
	}
	err = mc.sendAuthRejectedPacket()
	if err != nil {
		err = mc.event(event_send_auth_rejected)
	}
	return
}
func (mc *MolePeer) checkAuthIfServer(pkt *packets.MoleContainerPacket) (e error) {
	if mc.isServer() && pkt.PacketType == packets.AuthRequestType {
		if pkt.MolePacket.(*packets.AuthRequestPacket).Authenticate(mc.secret) {
			e = mc.event(event_internal_valid_secret)
		} else {
			e = mc.event(event_internal_invalid_secret)
		}
	}

	return
}
func (mc *MolePeer) authenticating() (e error) {
	// possible error
	// ErrUnauthorized
	// ErrServerIsNotResponding
	var (
		retry <-chan time.Time
	)
	defer func() {
		mc.StopError = e
	}()
	if !mc.state.Is(state_connected) {
		return
	}
	if mc.isClient() {
		//send auth request
		if e = mc.sendAuthRequestPacket(); e != nil {
			return
		}
		if e = mc.state.Event(mc.myCtx, event_send_auth_request); e != nil {
			return
		}
	}

	if mc.isClient() {
		//retry send chan
		r := time.NewTicker(mc.auth_retry)
		defer r.Stop()
		retry = r.C

	} else {
		//for server mode we should not retry
		retry = nil
	}
	//authnetication timeout
	auth_timeout := time.After(mc.auth_timeout)

	done := mc.myCtx.Done()
	for {
		select {
		case <-done:
			return
		case <-auth_timeout:
			switch mc.molePeerType {
			case MolePeerClient:
				e = ErrServerIsNotResponding
			default:
				e = ErrTimeout
			}
			return
		case <-retry:
			mc.sendAuthRequestPacket()
		case pkt, more := <-mc.npp.ParsedPacketChan:
			if !more {
				return io.EOF
			}
			if e = mc.event(recvPktTypeToEvent[pkt.PacketType]); e != nil {
				return
			}

			e = mc.checkAuthIfServer(&pkt)
			if e != nil {
				return
			}

			switch mc.state.Current() {
			case state_authenticated:
				e = mc.sendAuthAcceptedIfServer()
				return
			case state_unauthorized:
				mc.sendAuthRejectedIfServer()
				e = ErrUnauthorized
				return
			default:
				//ignore packet
				continue
			}

		}
	}

}
func (mc *MolePeer) onNetPaserClose(err error) {
	defer mc.myCancel()
	if err != nil {
		fmt.Printf("onNetParserClose: %+v\n", err)
		//something went wrong with parsing
		//send disconnect request to remote party
	}
}
func (mc *MolePeer) setUpFSM(start_state string) *fsm.FSM {
	switch mc.molePeerType {
	case MolePeerClient:
		return fsm.NewFSM(start_state, client_fsm_events, fsm.Callbacks{})
	case MolePeerServer:
		return fsm.NewFSM(start_state, server_fsm_events, fsm.Callbacks{})
	default:
		return nil
	}
}
