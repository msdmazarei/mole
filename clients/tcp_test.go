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

var _ = Describe("tcp", Label("tcp"), func() {
	var (
		clientPB       TCPClientPB
		ctx            context.Context
		cancel         context.CancelFunc
		err            error
		server, client net.Conn
		tcpClient      *TCPClient

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
			client, server = net.Pipe()
			chErr = make(chan error, 1)
			clientPB = TCPClientPB{
				TCPParams: TCPParams{
					Address:     net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3285},
					AuthTimeout: time.Second,
					Conn:        client,
					OnFinish:    onFinish,
					GetTunDev: func(string, *TunDevProps) (io.ReadWriteCloser, error) {
						_, processDevSide = net.Pipe()
						return processDevSide, nil
					},
				},
			}
			tcpClient, _ = NewTCPClient(ctx, clientPB)
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
			Expect(tcpClient.fsm.Current()).To(Equal(StateAuthenticated))
		})

	})
	Context("connected", Label("connected"), func() {
		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			server, client = net.Pipe()
			chErr = make(chan error, 1)
			clientPB = TCPClientPB{
				TCPParams: TCPParams{
					Address:     net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3285},
					AuthTimeout: time.Second * 5,
					Conn:        client,
					OnFinish:    onFinish,

					GetTunDev: func(string, *TunDevProps) (io.ReadWriteCloser, error) {
						sysDevSide, processDevSide = net.Pipe()
						return processDevSide, nil
					},
				},
				PingInterval: time.Millisecond * 1500,
			}
			tcpClient, _ = NewTCPClient(ctx, clientPB)
			// make client connectd.
			Expect(server.SetReadDeadline(time.Now().Add(clientPB.AuthTimeout))).NotTo(HaveOccurred())
			_, err = server.Read(buf[:])
			Expect(err).To(BeNil())
			Expect(cpkt.FromBytes(buf[:])).To(BeNil())
			Expect(cpkt.PacketType).To(Equal(packets.AuthRequestType))
			Expect(sendAuthAccepted(server)).To(BeNil())
			<-time.After(time.Millisecond)
			Expect(tcpClient.fsm.Current()).To(Equal(StateAuthenticated))

		})
		AfterEach(func() {
			cancel()
			<-time.After(time.Millisecond * 10)
			sysDevSide.Close()
			processDevSide.Close()
			client.Close()
			server.Close()
			<-time.After(time.Millisecond * 10)
		})
		It("sends captured packet", Label("send-encap"), func() {
			bufPkt := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}
			_, err = sysDevSide.Write(bufPkt)
			Expect(err).NotTo(HaveOccurred())
			pkt := waitToRecieveSpecPkt(server, packets.EncapsulatedNetDevPacketType, tcpClient.AuthTimeout)
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
