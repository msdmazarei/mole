package streams

import (
	"errors"
	"github.com/looplab/fsm"
	"github.com/msdmazarei/mole/packets"
)

const (
	state_connected      = "connected"
	state_authenticating = "authenticating"
	state_authenticated  = "aunthenticated"
	state_unauthorized   = "unauthorized"
	state_disconnecting  = "disconnecting"
	state_disconnected   = "disconnected"

	event_send_auth_request = "ev_send_auth_request"
	event_recv_auth_request = "ev_recv_auth_request"

	event_send_auth_rejected = "ev_send_auth_rejected"
	event_recv_auth_rejected = "ev_recv_auth_rejected"

	event_send_auth_accepted = "ev_send_auth_accepted"
	event_recv_auth_accepted = "ev_recv_auth_accepted"

	event_send_ping = "ev_send_ping"
	event_recv_ping = "ev_recv_ping"

	event_send_pong = "ev_send_pong"
	event_recv_pong = "ev_recv_pong"

	event_send_report = "ev_send_report"
	event_recv_report = "ev_recv_report"

	event_send_dev_pkt = "ev_send_dev_pkt"
	event_recv_dev_pkt = "ev_recv_dev_pkt"

	event_send_disconnect_request = "ev_send_disconnect_request"
	event_recv_disconnect_request = "ev_recv_disconnect_request"

	event_send_disconnect_accepted = "ev_send_disconnect_accepted"
	event_recv_disconnect_accepted = "ev_recv_disconnect_accepted"

	event_send_encapsulated_net_packet = "event_send_encapsulated_net_packet"
	event_recv_encapsulated_net_packet = "event_recv_encapsulated_net_packet"

	event_internal_valid_secret   = "event_internal_valid_secret"
	event_internal_invalid_secret = "event_internal_invalid_secret"
)

var recvPktTypeToEvent map[packets.PacketType]string = map[packets.PacketType]string{
	packets.AuthAcceptType:               event_recv_auth_accepted,
	packets.AuthRejectType:               event_recv_auth_rejected,
	packets.AuthRequestType:              event_recv_auth_request,
	packets.PingType:                     event_recv_ping,
	packets.PongType:                     event_recv_pong,
	packets.ReportType:                   event_recv_report,
	packets.DisconnectRequestType:        event_recv_disconnect_request,
	packets.DisconnectAcceptType:         event_recv_disconnect_accepted,
	packets.EncapsulatedNetDevPacketType: event_recv_encapsulated_net_packet,
}

var sendPktTypeToEvent map[packets.PacketType]string = map[packets.PacketType]string{
	packets.AuthAcceptType:               event_send_auth_accepted,
	packets.AuthRejectType:               event_send_auth_rejected,
	packets.AuthRequestType:              event_send_auth_request,
	packets.PingType:                     event_send_ping,
	packets.PongType:                     event_send_pong,
	packets.ReportType:                   event_send_report,
	packets.DisconnectRequestType:        event_send_disconnect_request,
	packets.DisconnectAcceptType:         event_send_disconnect_accepted,
	packets.EncapsulatedNetDevPacketType: event_send_encapsulated_net_packet,
}

var (
	ErrUnauthorized          = errors.New("Unauthorized")
	ErrServerIsNotResponding = errors.New("Server is Not Responding")
	ErrTimeout               = errors.New("timeout")
	ErrConnectionTimeout     = errors.New("Connection Timeout")

	client_fsm_events = fsm.Events{
		{
			Name: event_send_auth_request,
			Src:  []string{state_connected, state_authenticating},
			Dst:  state_authenticating,
		},
		{
			Name: event_recv_auth_accepted,
			Src:  []string{state_authenticating},
			Dst:  state_authenticated,
		},
		{
			Name: event_recv_auth_rejected,
			Src:  []string{state_authenticating},
			Dst:  state_unauthorized,
		},
		{
			Src:  []string{state_unauthorized, state_authenticating, state_authenticated},
			Dst:  state_disconnecting,
			Name: event_recv_disconnect_request,
		},
		{
			Src:  []string{state_disconnecting},
			Dst:  state_disconnected,
			Name: event_recv_disconnect_accepted,
		},
		{
			Src:  []string{state_disconnecting, state_disconnected},
			Dst:  state_disconnected,
			Name: event_send_disconnect_accepted,
		},
		{
			Src:  []string{state_disconnecting},
			Dst:  state_disconnected,
			Name: event_send_disconnect_accepted,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_recv_encapsulated_net_packet,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_send_encapsulated_net_packet,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_recv_ping,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_send_ping,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_send_pong,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_recv_pong,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_recv_report,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_send_report,
		},
	}

	server_fsm_events = fsm.Events{
		{
			Name: event_recv_auth_request,
			Src:  []string{state_connected, state_authenticating},
			Dst:  state_authenticating,
		},
		{
			Name: event_internal_valid_secret,
			Src:  []string{state_authenticating},
			Dst:  state_authenticated,
		},
		{
			Name: event_internal_invalid_secret,
			Src:  []string{state_authenticating},
			Dst:  state_unauthorized,
		},
		{
			Src:  []string{state_unauthorized, state_authenticating, state_authenticated},
			Dst:  state_disconnecting,
			Name: event_recv_disconnect_request,
		},
		{
			Src:  []string{state_unauthorized, state_authenticating, state_authenticated},
			Dst:  state_disconnecting,
			Name: event_send_disconnect_request,
		},
		{
			Src:  []string{state_disconnecting},
			Dst:  state_disconnected,
			Name: event_recv_disconnect_accepted,
		},
		{
			Src:  []string{state_disconnecting, state_disconnected},
			Dst:  state_disconnected,
			Name: event_send_disconnect_accepted,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_recv_encapsulated_net_packet,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_send_encapsulated_net_packet,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_recv_ping,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_send_ping,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_send_pong,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_recv_pong,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_recv_report,
		},
		{
			Src:  []string{state_authenticated},
			Dst:  state_authenticated,
			Name: event_send_report,
		},
	}
)
