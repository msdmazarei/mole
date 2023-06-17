package shared

import (
	"errors"

	"github.com/looplab/fsm"
	"github.com/msdmazarei/mole/packets"
)

const (
	StateConnected      = "connected"
	StateAuthenticating = "authenticating"
	StateAuthenticated  = "aunthenticated"
	StateUnauthorized   = "unauthorized"
	StateDisconnecting  = "disconnecting"
	StateDisconnected   = "disconnected"

	EventSendAuthRequest = "ev_send_auth_request"
	EventRecvAuthRequest = "ev_recv_auth_request"

	EventSendAuthRejected = "ev_send_auth_rejected"
	EventRecvAuthRejected = "ev_recv_auth_rejected"

	EventSendAuthAccepted = "ev_send_auth_accepted"
	EventRecvAuthAccepted = "ev_recv_auth_accepted"

	EventSendPing = "ev_send_ping"
	EventRecvPing = "ev_recv_ping"

	EventSendPong = "ev_send_pong"
	EventRecvPong = "ev_recv_pong"

	EventSendReport = "ev_send_report"
	EventRecvReport = "ev_recv_report"

	EventSendDevPkt = "ev_send_dev_pkt"
	EventRecvDevPkt = "ev_recv_dev_pkt"

	EventSendDisconnectRequest = "ev_send_disconnect_request"
	EventRecvDisconnectRequest = "ev_recv_disconnect_request"

	EventSendDisconnectAccepted = "ev_send_disconnect_accepted"
	EventRecvDisconnectAccepted = "ev_recv_disconnect_accepted"

	EventSendEncapsulatedNetPacket = "EventSendEncapsulatedNetPacket"
	EventRecvEncapsulatedNetPacket = "EventRecvEncapsulatedNetPacket"

	EventInternalValidSecret          = "EventInternalValidSecret"
	EventInternalInvalidSecret        = "EventInternalInvalidSecret"
	EventInternalAuthTimeout          = "EventInternalAuthTimeout"
	EventInternalPacketRecieveTimeout = "event_internal_packet_recv_timeout"
)

var RecvPktTypeToEvent map[packets.PacketType]string = map[packets.PacketType]string{
	packets.AuthAcceptType:               EventRecvAuthAccepted,
	packets.AuthRejectType:               EventRecvAuthRejected,
	packets.AuthRequestType:              EventRecvAuthRequest,
	packets.PingType:                     EventRecvPing,
	packets.PongType:                     EventRecvPong,
	packets.ReportType:                   EventRecvReport,
	packets.DisconnectRequestType:        EventRecvDisconnectRequest,
	packets.DisconnectAcceptType:         EventRecvDisconnectAccepted,
	packets.EncapsulatedNetDevPacketType: EventRecvEncapsulatedNetPacket,
	packets.ContainerPacketType:          "",
}

var SendPktTypeToEvent map[packets.PacketType]string = map[packets.PacketType]string{
	packets.AuthAcceptType:               EventSendAuthAccepted,
	packets.AuthRejectType:               EventSendAuthRejected,
	packets.AuthRequestType:              EventSendAuthRequest,
	packets.PingType:                     EventSendPing,
	packets.PongType:                     EventSendPong,
	packets.ReportType:                   EventSendReport,
	packets.DisconnectRequestType:        EventSendDisconnectRequest,
	packets.DisconnectAcceptType:         EventSendDisconnectAccepted,
	packets.EncapsulatedNetDevPacketType: EventSendEncapsulatedNetPacket,
	packets.ContainerPacketType:          "",
}

var (
	ErrUnauthorized          = errors.New("unauthorized")
	ErrServerIsNotResponding = errors.New("server is not responding")
	ErrTimeout               = errors.New("timeout")
	ErrConnectionTimeout     = errors.New("connection timeout")

	ClientFsmEvents = fsm.Events{
		{
			Name: EventInternalPacketRecieveTimeout,
			Src:  []string{StateDisconnecting, StateDisconnected},
			Dst:  StateDisconnected,
		},
		{
			Name: EventSendDisconnectRequest,
			Src:  []string{StateAuthenticated, StateAuthenticating, StateConnected, StateDisconnecting},
			Dst:  StateDisconnecting,
		},
		{
			Name: EventInternalPacketRecieveTimeout,
			Src:  []string{StateAuthenticated, StateAuthenticating, StateConnected},
			Dst:  StateDisconnecting,
		},
		{
			Name: EventRecvAuthAccepted,
			Src:  []string{StateConnected, StateAuthenticating},
			Dst:  StateAuthenticated,
		},
		{
			Name: EventRecvAuthRejected,
			Src:  []string{StateConnected, StateAuthenticating},
			Dst:  StateUnauthorized,
		},
		{
			Name: EventInternalAuthTimeout,
			Src:  []string{StateConnected, StateAuthenticating},
			Dst:  StateUnauthorized,
		},
		{
			Name: EventSendAuthRequest,
			Src:  []string{StateConnected, StateAuthenticating},
			Dst:  StateAuthenticating,
		},
		{
			Name: EventRecvAuthAccepted,
			Src:  []string{StateAuthenticating},
			Dst:  StateAuthenticated,
		},
		{
			Name: EventRecvAuthRejected,
			Src:  []string{StateAuthenticating},
			Dst:  StateUnauthorized,
		},
		{
			Src:  []string{StateUnauthorized, StateAuthenticating, StateAuthenticated},
			Dst:  StateDisconnecting,
			Name: EventRecvDisconnectRequest,
		},
		{
			Src:  []string{StateDisconnecting},
			Dst:  StateDisconnected,
			Name: EventRecvDisconnectAccepted,
		},
		{
			Src:  []string{StateDisconnecting, StateDisconnected},
			Dst:  StateDisconnected,
			Name: EventSendDisconnectAccepted,
		},
		{
			Src:  []string{StateDisconnecting},
			Dst:  StateDisconnected,
			Name: EventSendDisconnectAccepted,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventRecvEncapsulatedNetPacket,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventSendEncapsulatedNetPacket,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventRecvPing,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventSendPing,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventSendPong,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventRecvPong,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventRecvReport,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventSendReport,
		},
	}

	ServerFsmEvents = fsm.Events{
		{
			Name: EventRecvPing,
			Src:  []string{StateDisconnecting},
			Dst:  StateDisconnecting,
		},
		{
			Name: EventRecvPong,
			Src:  []string{StateDisconnecting},
			Dst:  StateDisconnecting,
		},

		{
			Name: EventRecvAuthRequest,
			Src:  []string{StateConnected, StateAuthenticating},
			Dst:  StateAuthenticating,
		},
		{
			Name: EventInternalValidSecret,
			Src:  []string{StateAuthenticating, StateConnected},
			Dst:  StateAuthenticated,
		},
		{
			Name: EventInternalInvalidSecret,
			Src:  []string{StateAuthenticating, StateConnected},
			Dst:  StateUnauthorized,
		},
		{
			Src:  []string{StateUnauthorized, StateAuthenticating, StateAuthenticated},
			Dst:  StateDisconnecting,
			Name: EventRecvDisconnectRequest,
		},
		{
			Src:  []string{StateUnauthorized, StateAuthenticating, StateAuthenticated},
			Dst:  StateDisconnecting,
			Name: EventSendDisconnectRequest,
		},
		{
			Src:  []string{StateDisconnecting},
			Dst:  StateDisconnected,
			Name: EventRecvDisconnectAccepted,
		},
		{
			Src:  []string{StateDisconnecting, StateDisconnected},
			Dst:  StateDisconnected,
			Name: EventSendDisconnectAccepted,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventRecvEncapsulatedNetPacket,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventSendEncapsulatedNetPacket,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventRecvPing,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventSendPing,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventSendPong,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventRecvPong,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventRecvReport,
		},
		{
			Src:  []string{StateAuthenticated},
			Dst:  StateAuthenticated,
			Name: EventSendReport,
		},
		{
			Src:  []string{StateConnected, StateAuthenticating},
			Dst:  StateDisconnecting,
			Name: EventInternalAuthTimeout,
		},
	}
)
