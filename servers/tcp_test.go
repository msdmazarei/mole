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

type (
	mockListner struct {
		connChan chan net.Conn
		ctx      context.Context
		cancel   context.CancelFunc
	}
)

func NewFakeListner(ctx context.Context, connChan chan net.Conn) *mockListner {
	rtn := &mockListner{
		connChan: connChan,
	}
	rtn.ctx, rtn.cancel = context.WithCancel(ctx)
	return rtn
}
func (m *mockListner) Accept() (net.Conn, error) {
	select {
	case c := <-m.connChan:
		return c, nil
	case <-m.ctx.Done():
		return nil, io.ErrClosedPipe
	}
}
func (m *mockListner) Close() error {
	m.cancel()
	return nil
}
func (m *mockListner) Addr() net.Addr {
	return &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

var _ = Describe("Tcp", func() {
	var (
		ctx            context.Context
		cancel         context.CancelFunc
		readBuf        [5000]byte
		err            error
		devSystemSide  io.ReadWriteCloser
		devProcessSide io.ReadWriteCloser
		onFinish       = func(err error) {
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
		server           net.Conn
		AuthTimeout      = time.Second
		onRemovingClient = func(c string, username string, d io.ReadWriteCloser, t *TunDevProps) {}

		newTCPServer = func() *TCPServer {
			client, server = net.Pipe()
			connChan := make(chan net.Conn, 1)
			listener := NewFakeListner(ctx, connChan)
			connChan <- server

			tcppb := TCPServerPB{
				MaxConnection: 10,
				Authenticate: func(authPkt packets.AuthRequestPacket) bool {
					return authPkt.Username == username && authPkt.Authenticate(secret)
				},
				TCPParams: TCPParams{
					GetTunDev:   getTunDev,
					OnFinish:    onFinish,
					Listener:    listener,
					MTU:         uint16(MTU),
					AuthTimeout: AuthTimeout,
				},
				OnRemovingClient: onRemovingClient,
			}

			return NewTCPServer(ctx, tcppb)
		}
	)

	When("Basic Operation", Label("tcp", "server"), func() {
		var (
			removedClient string
		)
		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())

			onRemovingClient = func(c string, _ string, _ io.ReadWriteCloser, t *TunDevProps) {
				removedClient = c
				Expect(c).NotTo(Equal(""))
			}
			newTCPServer()
		})
		AfterEach(func() {
			cancel()
			server.Close()
		})

		It("should remove conenction after AuthTimeout", func() {
			buf := testPingMsg()
			_, err = client.Write(buf)
			Expect(err).NotTo(HaveOccurred())
			<-time.After(AuthTimeout + time.Second)
			Expect(removedClient).NotTo(Equal(""))
		})

	})
	When("Authenticating", Label("tcp", "auth"), func() {
		var (
			expectedPacketType packets.PacketType
		)
		JustBeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			newTCPServer()
		})
		AfterEach(func() {
			_, err = client.Read(readBuf[:])
			Expect(err).To(BeNil())
			pkt := packets.MoleContainerPacket{}
			err = pkt.FromBytes(readBuf[:])
			Expect(err).To(BeNil())
			Expect(pkt.PacketType).To(Equal(expectedPacketType))

			cancel()
			server.Close()
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
	When("Authenticated", Label("tcp", "auth", "authenticated"), func() {
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
			newTCPServer()
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
			server.Close()
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
