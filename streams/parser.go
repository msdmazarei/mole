package streams

import (
	"context"
	"io"

	"github.com/msdmazarei/mole/packets"
)

/*
   most important note is that because ParserdPacketChan is buffered channel,
   there is pissibilty to have onClose called but there are some ParsedPacket
   iniside ParsedPacketChan

*/
type StreamParser struct {
	ctx              context.Context
	reader           io.Reader
	buf              []byte
	ParsedPacketChan chan packets.MoleContainerPacket
	onClose          context.CancelCauseFunc
	myCtxCancel      context.CancelFunc
}

func NewStreamParser(ctx context.Context, reader io.Reader, onClose context.CancelCauseFunc) StreamParser {
	myCtx, myCancel := context.WithCancel(ctx)
	rtn := StreamParser{
		ctx:              myCtx,
		myCtxCancel:      myCancel,
		reader:           reader,
		buf:              make([]byte, 2000),
		ParsedPacketChan: make(chan packets.MoleContainerPacket, 5),
		onClose:          onClose,
	}
	go rtn.doParsing()
	return rtn
}

func (sp *StreamParser) doParsing() {
	var (
		err       error
		readBytes int
	)

	defer func() {
		if sp.ctx.Err() == nil {
			sp.myCtxCancel()
		}
		close(sp.ParsedPacketChan)
		sp.onClose(err)
	}()

	is_done := sp.ctx.Done()
	for {
		readBytes, err = sp.reader.Read(sp.buf)
		if err != nil {
			if err == io.EOF {
				//we reached to end of reader,
				err = nil
			}
			return
		}
		if readBytes == 0 {
			panic("reader has bad implemnation, Unexpected behaviour")
		}
		for i := 0; i < readBytes; {
			mole_packet := packets.MoleContainerPacket{}
			err = mole_packet.FromBytes(sp.buf[i:])
			if err != nil {
				return
			}
			select {
			case <-is_done:
				return
			case sp.ParsedPacketChan <- mole_packet:
				i += int(mole_packet.TotalLength())
			}
		}
	}
}
