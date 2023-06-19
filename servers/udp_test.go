package servers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"time"

	"github.com/msdmazarei/mole/packets"
	. "github.com/msdmazarei/mole/shared"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func testToBytes(inp packets.MolePacketer) []byte {
	pkt := packets.NewMoleContainerPacket(inp)
	buf := make([]byte, pkt.TotalLength())
	Expect(pkt.WriteTo(buf)).To(BeNil())
	return buf
}

func testAuthReqMsg(username, secret string) []byte {
	authReq := packets.NewAuhRequestPacket(username, secret)
	return testToBytes(&authReq)
}

func testPingMsg() []byte {
	ping := packets.NewPingPacket("ping")
	return testToBytes(&ping)
}

var _ = Describe("UDP", func() {
	var (
		ctx            context.Context
		cancel         context.CancelFunc
		readBuf        [5000]byte
		err            error
		devSystemSide  io.ReadWriteCloser
		devProcessSide io.ReadWriteCloser
		// udp_server UDPServer
		onFinish = func(err error) {
			if errors.Is(err, io.ErrClosedPipe) {
				err = nil
			}
			Expect(err).NotTo(HaveOccurred())

		}
		getTunDev = func(string, *TunDevProps) (io.ReadWriteCloser, error) {
			devSystemSide, devProcessSide = net.Pipe()
			return devProcessSide, nil
		}
		MTU              = 1000
		secret           = "masoudisgoodboy"
		username         = "msd"
		client           net.Conn
		fakeUDPConn      *FakeUDP
		AuthTimeout      = time.Second
		onRemovingClient = func(c string, username string, d io.ReadWriteCloser, t *TunDevProps) {}

		newUDPServer = func() UDPServer {
			fakeUDPConn, _ = NewFakeUDP(ServerSide)
			client = fakeUDPConn.Client

			udppb := UDPServerPB{
				MaxConnection: 10,
				Authenticate: func(authPkt packets.AuthRequestPacket) bool {
					return authPkt.Username == username && authPkt.Authenticate(secret)
				},
				UDPParams: UDPParams{
					GetTunDev:   getTunDev,
					OnFinish:    onFinish,
					Conn:        fakeUDPConn,
					MTU:         uint16(MTU),
					AuthTimeout: AuthTimeout,
				},
				OnRemovingClient: onRemovingClient,
			}

			return NewUDPServer(ctx, udppb)
		}
	)
	When("Basic Operation", func() {
		var (
			removedClient string
		)
		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())

			onRemovingClient = func(c string, u string, d io.ReadWriteCloser, t *TunDevProps) {
				removedClient = c
				Expect(c).NotTo(Equal(""))
			}
			newUDPServer()
		})
		AfterEach(func() {
			cancel()
			fakeUDPConn.Close()
		})

		It("should remove conenction after AuthTimeout", func() {
			buf := testPingMsg()
			_, err = client.Write(buf)
			Expect(err).NotTo(HaveOccurred())
			<-time.After(AuthTimeout + time.Second)
			Expect(removedClient).NotTo(Equal(""))
		})

	})
	When("Authenticating", Label("auth"), func() {
		var (
			expectedPacketType packets.PacketType
		)
		JustBeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			newUDPServer()
		})
		AfterEach(func() {
			_, err = client.Read(readBuf[:])
			Expect(err).To(BeNil())
			pkt := packets.MoleContainerPacket{}
			err = pkt.FromBytes(readBuf[:])
			Expect(err).To(BeNil())
			Expect(pkt.PacketType).To(Equal(expectedPacketType))

			cancel()
			fakeUDPConn.Close()
		})
		It("reject client when it sends wrong secret", func() {
			buf := testAuthReqMsg(username, secret+":")
			_, _ = client.Write(buf)
			expectedPacketType = packets.AuthRejectType
		})
		It("accept client when it send correct secret", func() {
			buf := testAuthReqMsg(username, secret)
			_, _ = client.Write(buf)
			expectedPacketType = packets.AuthAcceptType
		})
		// It("should reject new client once there is an authenticated connected client", func() {
		// how to simulate multiple client?
		// })

	})
	When("Authenticated", Label("auth"), func() {
		var (
			checkExpectedPkt bool
			expectedPktType  packets.PacketType
			clientIsRemoved  = false
		)
		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			clientIsRemoved = false
			onRemovingClient = func(string, string, io.ReadWriteCloser, *TunDevProps) {
				clientIsRemoved = true
			}
			newUDPServer()
			buf := testAuthReqMsg(username, secret)
			_, err = client.Write(buf)
			Expect(err).NotTo(HaveOccurred())
			_, err = client.Read(readBuf[:])
			Expect(err).To(BeNil())
			pkt := packets.MoleContainerPacket{}
			err = pkt.FromBytes(readBuf[:])
			Expect(err).To(BeNil())
			Expect(pkt.PacketType).To(Equal(packets.AuthAcceptType))
			checkExpectedPkt = true

		})
		AfterEach(func() {
			if checkExpectedPkt {
				_, err = client.Read(readBuf[:])
				Expect(err).To(BeNil())
				pkt := packets.MoleContainerPacket{}
				err = pkt.FromBytes(readBuf[:])
				Expect(err).To(BeNil())
				Expect(pkt.PacketType).To(Equal(expectedPktType))
			}

			cancel()
			fakeUDPConn.Close()
		})

		It("response with pong to ping", Label("ping"), func() {
			buf := testPingMsg()
			_, err = client.Write(buf)
			Expect(err).NotTo(HaveOccurred())
			expectedPktType = packets.PongType
		})
		It("Should Disconnect IDLE and not active client", Label("idle"), func() {
			// should get disconnect request from server
			checkExpectedPkt = false

			_, err = client.Read(readBuf[:])
			Expect(err).To(BeNil())
			pkt := packets.MoleContainerPacket{}
			err = pkt.FromBytes(readBuf[:])
			Expect(err).To(BeNil())
			Expect(pkt.PacketType).To(Equal(packets.DisconnectRequestType))

			<-time.After(AuthTimeout * 2)
			Expect(clientIsRemoved).To(Equal(true))
		})

		It("send disconnect accepted in response of disconenct request", Label("disconnect"), func() {
			checkExpectedPkt = false

			disPkt := packets.NewDisconnectRequestPacket("disconnect")
			_, err = client.Write(testToBytes(&disPkt))
			Expect(err).To(BeNil())
			_, err = client.Read(readBuf[:])
			Expect(err).To(BeNil())
			pkt := packets.MoleContainerPacket{}
			err = pkt.FromBytes(readBuf[:])
			Expect(err).To(BeNil())
			Expect(pkt.PacketType).To(Equal(packets.DisconnectAcceptType))
			<-time.After(AuthTimeout + time.Millisecond)
			Expect(clientIsRemoved).To(Equal(true))
		})

		It("send encap packet once captured from dev", Label("encap-out"), func() {
			checkExpectedPkt = false

			for i := byte(1); i < 10; i++ {
				capturedPkt := []byte{1 + i, 2 + i, 3 + i, 4 + i, 5 + i}
				_, err = devSystemSide.Write(capturedPkt)
				Expect(err).NotTo(HaveOccurred())
				_, err = client.Read(readBuf[:])
				Expect(err).To(BeNil())

				pkt := packets.MoleContainerPacket{}
				err = pkt.FromBytes(readBuf[:])
				Expect(err).To(BeNil())
				Expect(pkt.PacketType).To(Equal(packets.EncapsulatedNetDevPacketType))

				encap, ok := pkt.MolePacket.(*packets.EncapsulatedNetDevPacket)
				Expect(ok).To(Equal(true))
				bw := bytes.NewBuffer([]byte{})
				_, err = encap.WriteContentTo(bw)
				Expect(err).NotTo(HaveOccurred())
				Expect(bw.Bytes()[:encap.TotalLength()]).To(Equal(capturedPkt))

				<-time.After(time.Millisecond)

			}
		})

		It("write received encap packet to dev", Label("encap-in"), func() {
			checkExpectedPkt = false
			for i := byte(0); i < 10; i++ {
				origBuf := []byte{1 + i, 2 + i, 3 + i, 4 + i, 5 + i}
				encap := packets.NewEncapsulateNetDevPacket(origBuf)
				_, err = client.Write(testToBytes(&encap))
				Expect(err).NotTo(HaveOccurred())
				n, e := devSystemSide.Read(readBuf[:])
				Expect(e).NotTo(HaveOccurred())
				Expect(n).To(Equal(len(origBuf)))
				Expect(readBuf[:n]).To(Equal(origBuf))
			}
		})

	})
})
