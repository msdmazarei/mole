package streams

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/msdmazarei/mole/packets"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	client_rw, server_rw       io.ReadWriter
	net_dev_rw, net_dev_sys_rw io.ReadWriter

	ctx, ctx_srv, ctx_peer          context.Context
	cancel, cancel_srv, cancel_peer context.CancelFunc
)
var (
	parser                                  StreamParser
	parser_err_chan, peer_err_chan          chan error
	test_five_sec, test_two_sec, test_a_sec time.Duration = 10 * time.Millisecond, 5 * time.Millisecond, 1 * time.Millisecond
)

func setup_test(mtype MolePeerType, state string, secret string) {

	ctx, cancel = context.WithTimeout(context.Background(), 3*test_five_sec)
	ctx_peer, cancel_peer = context.WithTimeout(context.Background(), 2*test_five_sec)

	client_rw, server_rw = NewConnectedChannelReadWriter(ctx, make(chan []byte), make(chan []byte))

	net_dev_rw, net_dev_sys_rw = NewConnectedChannelReadWriter(ctx, make(chan []byte), make(chan []byte))

	parser_err_chan = make(chan error, 1)
	if mtype == MolePeerClient {
		parser = NewStreamParser(ctx, server_rw, func(err error) { parser_err_chan <- err })
	} else if mtype == MolePeerServer {
		parser = NewStreamParser(ctx, client_rw, func(err error) { parser_err_chan <- err })

	}
	peer_err_chan = make(chan error, 1)
	if net_dev_sys_rw != nil {
	}
	stream := client_rw
	if mtype == MolePeerClient {
		stream = client_rw
	} else if mtype == MolePeerServer {
		stream = server_rw
	}
	NewMolePeer(MolePeerPB{
		NetworkIO:                 stream,
		Secret:                    secret,
		NetDevIO:                  net_dev_rw,
		OnClose:                   func(err error) { peer_err_chan <- err },
		Context:                   ctx_peer,
		AuthTimeout:               &test_five_sec,
		AuthRetry:                 &test_a_sec,
		StartState:                &state,
		DisconnectAcceptedTimeout: &test_five_sec,
		DisconnectRetry:           &test_a_sec,
		DropConnectionTimeout:     &test_five_sec,
		ReportInterval:            &test_two_sec,
		MolePeerType:              &mtype,
	})

}
func teardown_after_test() {
	cancel_peer()
	cancel()
	<-time.After(test_a_sec)
	close(peer_err_chan)
	close(parser_err_chan)
}

var _ = Describe("Client", Label("client"), func() {
	const mtype = MolePeerClient
	Context("Operating", Label("operating"), func() {
		JustBeforeEach(func() {
			setup_test(mtype, state_authenticated, "secret")
		})
		JustAfterEach(func() {
			teardown_after_test()
		})

		It("reads packet from dev_net and send it to network", func() {
			var sample_packet []byte = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
			if _, e := net_dev_sys_rw.Write(sample_packet[:]); e != nil {
				panic(e)
			}
			for {
				select {
				case <-ctx.Done():
					panic("client is done without sending")
				case <-peer_err_chan:
					panic("client finished before sending request")
				case e := <-parser_err_chan:
					panic(e)
				case pkt := <-parser.ParsedPacketChan:
					if pkt.PacketType != packets.EncapsulatedNetDevPacketType {
						continue
					}
					w := bytes.NewBuffer(make([]byte, 0))
					pkt.MolePacket.(*packets.EncapsulatedNetDevPacket).WriteContentTo(w)
					Expect(w.Bytes()).To(Equal(sample_packet))
					return
				}
			}
		})

		It("writes received encap packet to dev_net", func() {
			//sending encap to client
			var sample_packet []byte = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
			encap := packets.NewEncapsulateNetDevPacket(sample_packet)
			cpkt := packets.NewMoleContainerPacket(&encap)
			cpkt.Write(server_rw)
			//reading from client net dev device to see if it was deliveredOB
			buf := make([]byte, len(sample_packet)+10)
			n, _ := net_dev_sys_rw.Read(buf)
			Expect(n).To(Equal(len(sample_packet)))
			Expect(buf[:n]).To(Equal(sample_packet))
		})
		//TODO: complete below tests and implementations
		It("sending ping periodically", Label("ping"), func() {
			//if server does not sending any packet
			//client should send ping
			waitForPacketType(ctx, packets.PingType, parser.ParsedPacketChan)
			waitForPacketType(ctx, packets.PingType, parser.ParsedPacketChan)
		})
		It("if server sends packet, client should not send ping while getting packet from server", Label("not-ping"), func() {
			//TODO: should be done!
		})

		It("should drop connection once didnt get any packet for specific time", Label("drop_conn"), func() {
			go keepReadingWhileCtxIsValid(ctx, parser.ParsedPacketChan)
			Expect(<-peer_err_chan).Error().To(Equal(ErrConnectionTimeout))
		})
		//TODO: report is tricky!
		It("", Label("report"), func() {})

	})
	Context("Disconnect", Label("disconnect"), func() {
		JustBeforeEach(func() {
			setup_test(mtype, state_authenticated, "secret")
		})
		JustAfterEach(func() {
			teardown_after_test()
		})

		It("send disconnect accepted once getting disconnect request and exit", Label("accept"), func() {
			send_disconnect(server_rw)
			pkt := waitForPacketType(ctx, packets.DisconnectAcceptType, parser.ParsedPacketChan)
			Expect(pkt).NotTo(BeNil())
			Expect(<-peer_err_chan).To(BeNil())
		})
		It("send disconnect request once ctx is done", Label("ctx", "done"), func() {
			go cancel_peer()
			dis_req_counter := 0
			for dis_req_counter < 4 {
				select {
				case <-peer_err_chan:
					panic("client finished before sending disconenct request")
					//				case e := <-parser_err_chan:
					//panic(e)
				case pkt := <-parser.ParsedPacketChan:
					if pkt.PacketType != packets.DisconnectRequestType {
						continue
					}
					dis_req_counter++
					return
				}

			}
			Expect(dis_req_counter).To(BeNumerically(">=", 4))
		})
	})

	Context("Authentication", Label("auth"), func() {
		JustBeforeEach(func() {
			setup_test(mtype, state_connected, "secret")
		})

		JustAfterEach(func() {
			teardown_after_test()
		})

		It("client should return ErrServerIsNotResponding", Label("noresponse"), func() {
			//keep reading from parsed pac
			go func() {
				waitForPacketType(ctx, packets.AuthAcceptType, parser.ParsedPacketChan)
			}()
			Expect(<-peer_err_chan).Error().To(Equal(ErrServerIsNotResponding))
		})

		It("client should retry sending auth if not getting response", Label("retry"), func() {
			auth_pkt_counter := 0
			go func() {
				for pkt := range parser.ParsedPacketChan {
					if pkt.PacketType == packets.AuthRequestType {
						auth_pkt_counter++
					}
				}

			}()
			Expect(<-peer_err_chan).Error().To(Equal(ErrServerIsNotResponding))
			Expect(auth_pkt_counter).To(BeNumerically(">=", 4))
		})

		It("client should exit once getting rejected from server", Label("reject"), func() {
			//first packet should be auth request
			waitForPacketType(ctx, packets.AuthRequestType, parser.ParsedPacketChan)
			reject := packets.NewAuthRejectPacket(packets.RejectCodeBadSecret, "rejected")
			p := packets.NewMoleContainerPacket(&reject)
			p.Write(server_rw)
			Expect(<-peer_err_chan).Error().To(Equal(ErrUnauthorized))

		})

	})
})

var _ = Describe("Server", Label("server"), func() {
	const mtype = MolePeerServer
	secret := "secret"
	Context("Authentication", Label("auth"), func() {

		JustBeforeEach(func() {
			setup_test(mtype, state_connected, "secret")
		})
		JustAfterEach(func() {
			teardown_after_test()
		})

		It("if client provide correct secret server should resposne with auth accepted", Label("correct"), func() {
			//send auth request
			send_auth_request(secret, client_rw)
			pkt := waitForPacketType(ctx_peer, packets.AuthAcceptType, parser.ParsedPacketChan)
			Expect(pkt).NotTo(BeNil())
		})
		It("if client provide incorrect secret server should resposne with auth rejected", Label("incorrect"), func() {
			//send auth request
			send_auth_request(secret+"1", client_rw)
			pkt := waitForPacketType(ctx, packets.AuthRejectType, parser.ParsedPacketChan)
			Expect(pkt).NotTo(BeNil())
			Expect(<-peer_err_chan).Error().To(Equal(ErrUnauthorized))
		})
		It("if client provide incorrect secret server should send disconnect request", Label("disconnect"), func() {
			//send auth request
			send_auth_request(secret+"1", client_rw)
			pkt := waitForPacketType(ctx, packets.DisconnectRequestType, parser.ParsedPacketChan)
			Expect(pkt).NotTo(BeNil())
			go keepReadingWhileCtxIsValid(ctx, parser.ParsedPacketChan)
			Expect(<-peer_err_chan).Error()
		})

	})
	Context("Disconnect", Label("server", "disconnect"), func() {
		JustBeforeEach(func() {
			setup_test(mtype, state_authenticated, secret)
		})
		JustAfterEach(func() {
			teardown_after_test()
		})

		It("should accept disconnect request and send disconnectaccepted message", Label("disconnectaccept"), func() {
			send_disconnect(client_rw)
			pkt := waitForPacketType(ctx_peer, packets.DisconnectAcceptType, parser.ParsedPacketChan)
			Expect(pkt).NotTo(BeNil())
			Expect(<-peer_err_chan).To(BeNil())
		})

	})
	Context("Operation", Label("operation"), func() {
		JustBeforeEach(func() {
			setup_test(mtype, state_authenticated, secret)

		})
		JustAfterEach(func() {
			teardown_after_test()
		})
		It("server should send pong on receiving ping", func() {
			send_ping(client_rw)
			pkt := waitForPacketType(ctx_peer, packets.PongType, parser.ParsedPacketChan)
			Expect(pkt).NotTo(BeNil())
		})
		It("server should periodically send report", func() {
			pkt := waitForPacketType(ctx_peer, packets.ReportType, parser.ParsedPacketChan)
			Expect(pkt).NotTo(BeNil())
		})
		It("send from local dev to net", Label("dev_to_net"), func() {
			sample_bytes := []byte{1, 2, 3, 4, 5, 3, 2, 8, 5}
			net_dev_sys_rw.Write(sample_bytes[:])
			pkt := waitForPacketType(ctx_peer, packets.EncapsulatedNetDevPacketType, parser.ParsedPacketChan)
			Expect(pkt).NotTo(BeNil())
			encap := pkt.MolePacket.(*packets.EncapsulatedNetDevPacket)
			buf := bytes.NewBuffer([]byte{})
			encap.WriteContentTo(buf)
			Expect(buf.Bytes()).To(Equal(sample_bytes))
		})
		It("write received buf to local dev", Label("net_to_dev"), func() {
			//sending encap to client
			var sample_packet []byte = []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
			encap := packets.NewEncapsulateNetDevPacket(sample_packet)
			cpkt := packets.NewMoleContainerPacket(&encap)
			cpkt.Write(client_rw)
			//reading from client net dev device to see if it was deliveredOB
			buf := make([]byte, len(sample_packet)+10)
			n, _ := net_dev_sys_rw.Read(buf)
			Expect(n).To(Equal(len(sample_packet)))
			Expect(buf[:n]).To(Equal(sample_packet))
		})

	})
})

func send_disconnect(w io.Writer) error {
	dis := packets.NewDisconnectRequestPacket("dis_request")
	cpkt := packets.NewMoleContainerPacket(&dis)
	_, e := cpkt.Write(w)
	return e
}
func send_ping(w io.Writer) (int, error) {
	ping := packets.NewPingPacket("ping")
	cpkt := packets.NewMoleContainerPacket(&ping)
	return cpkt.Write(w)
}

func send_pong(w io.Writer) (int, error) {
	ping := packets.NewPongPacket("pong")
	cpkt := packets.NewMoleContainerPacket(&ping)
	return cpkt.Write(w)
}
func send_auth_request(secret string, w io.Writer) error {
	a := packets.NewAuhRequestPacket(secret)
	cpkt := packets.NewMoleContainerPacket(&a)
	_, e := cpkt.Write(w)
	return e
}
func waitForPacketType(ctx context.Context, pt packets.PacketType, c chan packets.MoleContainerPacket) *packets.MoleContainerPacket {
	for {
		select {
		case <-ctx.Done():
			return nil
		case pkt := <-c:
			if pkt.PacketType == pt {
				return &pkt
			}
		}
	}
}
func keepReadingWhileCtxIsValid(ctx context.Context, c chan packets.MoleContainerPacket) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c:
			continue
		}
	}
}
