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

var _ = Describe("Parser", func() {
	var (
		reader    io.Reader
		errorChan chan error = make(chan error)
	)
	onError := func(cause error) {
		errorChan <- cause
	}
	ctx := context.Background()
	ping_pkt := packets.NewPingPacket("ping")
	mole_ping_pkt := packets.NewMoleContainerPacket(&ping_pkt)

	Context("Bad Input Stream", func() {
		It("serise of null bytes", func() {
			reader = bytes.NewReader([]byte{0, 0, 0, 0, 0, 0, 0})
			NewStreamParser(ctx, reader, onError)
			select {
			case <-time.After(time.Second * 30):
				panic("timeout")
			case err := <-errorChan:
				Expect(err).Error().To(Equal(packets.ErrBadPacketFormat))
			}
		})
		It("fail for bad bytes in middle of correct packets", func() {
			buf := make([]byte, mole_ping_pkt.TotalLength()*3)
			i := uint16(0)
			mole_ping_pkt.WriteTo(buf[i:])
			i += mole_ping_pkt.TotalLength()
			buf[i] = 0
			i++
			buf[i] = 0
			i++
			mole_ping_pkt.WriteTo(buf[i:])
			// <ping><0><0><ping>
			reader = bytes.NewReader(buf)
			NewStreamParser(ctx, reader, onError)
			select {
			case <-time.After(time.Second * 30):
				panic("timeout")
			case err := <-errorChan:
				Expect(err).Error().To(Equal(packets.ErrBadPacketFormat))
			}

		})
	})
	Context("Correct Input Should Properly Parsed", func() {
		It("Sending 30 ping and recieving 30 packets", func() {
			pl := mole_ping_pkt.TotalLength()
			buf := make([]byte, pl*30)
			for i := uint16(0); i < 30; i++ {
				mole_ping_pkt.WriteTo(buf[(i * pl):])
			}
			// <ping><ping>...<ping> 30 times
			reader = bytes.NewReader(buf)
			streamparsr := NewStreamParser(ctx, reader, onError)
			counter := 0
			for _ = range streamparsr.ParsedPacketChan {
				counter++
			}
			Expect(counter).To(Equal(30))
		})

		It("Return Result in proper order", Label("parser", "order"), func() {
			p1 := packets.NewAuhRequestPacket("secret")
			p2 := packets.NewAuthAcceptPacket(100, "msg")
			p3 := packets.NewEncapsulateNetDevPacket([]byte{1, 2, 3, 4, 5})
			p4 := packets.NewReportPacket(1, 2, 3, 4, 5)
			p5 := packets.NewDisconnectAcceptedPacket("msg")
			p6 := packets.NewPingPacket("msg")
			pkts := []packets.MolePacketer{&p1, &p2, &p3, &p4, &p5, &p6, &p1, &p2, &p3, &p4, &p5, &p6}
			cpkts := make([]packets.MoleContainerPacket, len(pkts))
			total_len := uint16(0)
			for i := 0; i < len(cpkts); i++ {
				cpkts[i] = packets.NewMoleContainerPacket(pkts[i])
				total_len += cpkts[i].TotalLength()
			}
			buf := make([]byte, total_len)
			j := uint16(0)
			for i := 0; i < len(cpkts); i++ {
				cpkts[i].WriteTo(buf[j:])
				j += cpkts[i].TotalLength()
			}
			reader = bytes.NewReader(buf)
			s := NewStreamParser(ctx, reader, onError)
			j = 0
			t := time.After(30 * time.Second)
			for {
				select {
				case e := <-errorChan:
					if e == nil && j!=uint16(len(pkts)) {
						//it seems there are some pakcets are not received yet
						continue
						
					}
					panic(e)
				case p := <-s.ParsedPacketChan:
					Expect(p.PacketType).To(Equal(pkts[j].GetPacketType()))
					j++
					if j == uint16(len(pkts)) {
						return
					}
				case <-t:
					panic("timeout")

				}
			}

		})

	})

})
