package clients

import (
	"bytes"
	"context"
	"io"

	"net"
	"time"

	"github.com/msdmazarei/mole/packets"
	. "github.com/msdmazarei/mole/shared"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Clients", func() {
	var (
		client   *UDPClient
		clientPB UDPClientPB
		ctx      context.Context
		cancel   context.CancelFunc
		err      error
		server   net.Conn
		fakeUDP  *FakeUDP
		secret   = "secret"
		chErr    chan error
		onFinish = func(e error) {
			select {
			case chErr <- e:
				break
			default:
			}

		}
		buf                        [1000]byte
		cpkt                       packets.MoleContainerPacket
		sysDevSide, processDevSide io.ReadWriteCloser
	)

	Context("auth", Label("auth"), func() {
		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			fakeUDP, _ = NewFakeUDP(ClientSide)
			server = fakeUDP.Server
			chErr = make(chan error, 1)
			clientPB = UDPClientPB{
				UDPParams: UDPParams{
					Address:     net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3285},
					AuthTimeout: time.Second,
					Secret:      secret,
					Conn:        fakeUDP,
					OnFinish:    onFinish,
					GetTunDev: func() (io.ReadWriteCloser, error) {
						_, processDevSide = net.Pipe()
						return processDevSide, nil
					},
				},
			}
			client, _ = NewUDPClient(ctx, clientPB)
		})
		AfterEach(func() {
			cancel()
			<-time.After(time.Millisecond * 100)
			close(chErr)
		})
		It("fails on getting unauthorized", Label("unauthorized"), func() {
			Expect(server.SetReadDeadline(time.Now().Add(clientPB.AuthTimeout))).NotTo(HaveOccurred())
			_, err = server.Read(buf[:])
			Expect(err).To(BeNil())
			// sending auth rejected response.
			Expect(cpkt.FromBytes(buf[:])).To(BeNil())
			Expect(cpkt.PacketType).To(Equal(packets.AuthRequestType))
			Expect(sendUnAuth(server)).To(BeNil())
			Expect(<-chErr).To(Equal(ErrUnauthorized))
		})
		It("fails on not getting response from server", Label("timeout"), func() {
			Expect(server.SetReadDeadline(time.Now().Add(clientPB.AuthTimeout))).NotTo(HaveOccurred())
			_, err = server.Read(buf[:])
			Expect(err).To(BeNil())
			Expect(cpkt.FromBytes(buf[:])).To(BeNil())
			Expect(cpkt.PacketType).To(Equal(packets.AuthRequestType))
			// keep reading
			go func() {
				for ctx.Err() == nil {
					_, e := server.Read(buf[:])
					if e != nil {
						return
					}
				}
			}()

			// sending auth rejected response.
			Expect(<-chErr).To(Equal(ErrServerIsNotResponding))
		})
		It("", Label("cancel"), func() {})

		It("goes to authenticated mode", Label("success"), func() {
			Expect(server.SetReadDeadline(time.Now().Add(clientPB.AuthTimeout))).NotTo(HaveOccurred())
			_, err = server.Read(buf[:])
			Expect(err).To(BeNil())
			// sending auth rejected response.
			Expect(cpkt.FromBytes(buf[:])).To(BeNil())
			Expect(cpkt.PacketType).To(Equal(packets.AuthRequestType))
			Expect(sendAuthAccepted(server)).To(BeNil())
			<-time.After(time.Millisecond)
			Expect(client.fsm.Current()).To(Equal(StateAuthenticated))
		})

	})
	Context("connected", Label("connected"), func() {
		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			fakeUDP, _ = NewFakeUDP(ClientSide)
			server = fakeUDP.Server
			chErr = make(chan error, 1)
			clientPB = UDPClientPB{
				UDPParams: UDPParams{
					Address:     net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3285},
					AuthTimeout: time.Second * 5,
					Secret:      secret,
					Conn:        fakeUDP,
					OnFinish:    onFinish,

					GetTunDev: func() (io.ReadWriteCloser, error) {
						sysDevSide, processDevSide = net.Pipe()
						return processDevSide, nil
					},
				},
				PingInterval: time.Millisecond * 1500,
			}
			client, _ = NewUDPClient(ctx, clientPB)
			// make client connectd.
			Expect(server.SetReadDeadline(time.Now().Add(clientPB.AuthTimeout))).NotTo(HaveOccurred())
			_, err = server.Read(buf[:])
			Expect(err).To(BeNil())
			Expect(cpkt.FromBytes(buf[:])).To(BeNil())
			Expect(cpkt.PacketType).To(Equal(packets.AuthRequestType))
			Expect(sendAuthAccepted(server)).To(BeNil())
			<-time.After(time.Millisecond)
			Expect(client.fsm.Current()).To(Equal(StateAuthenticated))

		})
		AfterEach(func() {
			cancel()
			<-time.After(time.Millisecond * 10)
			sysDevSide.Close()
			processDevSide.Close()
			fakeUDP.Close()
			server.Close()
			<-time.After(time.Millisecond * 10)
		})
		It("sends captured packet", Label("send-encap"), func() {
			bufPkt := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
			_, err = sysDevSide.Write(bufPkt)
			Expect(err).NotTo(HaveOccurred())
			pkt := waitToRecieveSpecPkt(server, packets.EncapsulatedNetDevPacketType, client.AuthTimeout)
			Expect(pkt).NotTo(BeNil())
			epkt, ok := pkt.(*packets.EncapsulatedNetDevPacket)
			Expect(ok).To(Equal(true))
			buf := bytes.NewBuffer([]byte{})
			_, err = epkt.WriteContentTo(buf)
			Expect(err).NotTo(HaveOccurred())
			Expect(buf.Bytes()).To(Equal(bufPkt))
		})
		It("write received encap", Label("write-encap"), func() {
			var n int
			bufPkt := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
			Expect(sendEncap(server, bufPkt)).To(BeNil())
			buf := make([]byte, len(bufPkt)+10)

			n, err = sysDevSide.Read(buf)
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(len(bufPkt)))
			Expect(buf[:n]).To(Equal(bufPkt))
		})
		It("clients send ping periodically", Label("ping"), func() {
			pingPkt := waitToRecieveSpecPkt(server, packets.PingType, clientPB.AuthTimeout*2)
			Expect(pingPkt).NotTo(BeNil())
		})
		It("clients will Drop if dont get any reply", Label("timeout"), func() {
			// keep reading in server and dont reply
			go func() {
				for ctx.Err() == nil {
					_, e := server.Read(buf[:])
					if e != nil {
						return
					}
				}
			}()

			err = <-chErr
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(ErrServerIsNotResponding))
		})

	})
})

func waitToRecieveSpecPkt(conn net.Conn, pktType packets.PacketType, waitFor time.Duration) packets.MolePacketer {
	var (
		buf       = make([]byte, 5000)
		startTime = time.Now()
		cpkt      packets.MoleContainerPacket
		err       error
	)
	for time.Since(startTime) < waitFor {
		_ = conn.SetReadDeadline(time.Now().Add(waitFor))
		_, err = conn.Read(buf)
		if err != nil {
			continue
		}
		err = cpkt.FromBytes(buf)
		if err == nil && cpkt.PacketType == pktType {
			return cpkt.MolePacket
		}
	}
	return nil
}
func sendPkt(conn net.Conn, pkt packets.MolePacketer) error {
	cpkt := packets.NewMoleContainerPacket(pkt)
	buf := make([]byte, cpkt.TotalLength())
	e := cpkt.WriteTo(buf)
	if e != nil {
		return e
	}
	_, e = conn.Write(buf)
	return e
}
func sendEncap(conn net.Conn, buf []byte) error {
	encap := packets.NewEncapsulateNetDevPacket(buf)
	return sendPkt(conn, &encap)
}
func sendUnAuth(conn net.Conn) error {
	unauth := packets.NewAuthRejectPacket(packets.RejectCodeBadSecret, "bad secret")
	return sendPkt(conn, &unauth)
}
func sendAuthAccepted(conn net.Conn) error {
	accepted := packets.NewAuthAcceptPacket(packets.AcceptCodeOK, "accepted")
	return sendPkt(conn, &accepted)
}
