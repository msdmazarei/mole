package streams

import (
	"context"
	"io"
)

type ChannelReadWriter struct {
	ctx       context.Context
	readchan  chan []byte
	writechan chan []byte
}

func NewConnectedChannelReadWriter(ctx context.Context, c1, c2 chan []byte) (client, server *ChannelReadWriter) {
	return &ChannelReadWriter{ctx, c1, c2}, &ChannelReadWriter{ctx, c2, c1}
}
func (b *ChannelReadWriter) Read(buf []byte) (int, error) {
	
	for {
		
		select {
		case <-b.ctx.Done():
			return 0, io.EOF
		case r := <-b.readchan:
			copy(buf, r)
			return len(r), nil
		}
	}
}
func (b *ChannelReadWriter) Write(buf []byte) (int, error) {
	for {
		select {
		case <-b.ctx.Done():
			return 0, io.EOF
		case b.writechan <- buf:
			return len(buf), nil
		}
	}
}
