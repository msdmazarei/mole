package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

const (
	socks5MethodNoAuth    byte = 0
	socks5CommandConnect  byte = 0x1
	socks5ATyp            byte = 0x1
	socks5ReplySuccess         = 0x00
	socks5ReplyIPv4            = 0x1
	socks5ReplyIPv6            = 0x4
	socks5ReplyDomainName      = 0x3
)

var (
	errProxyVersionMismatch            = errors.New("proxy version is mismatched")
	errProxyNeedsAuth                  = errors.New("proxy needs auth")
	errProxyReturnErrorResponse        = errors.New("proxy server responded with error")
	errProxyResponseContainsDomainName = errors.New("domian name is not supported in proxy response")
)

func connectThroughSocks5TCP(conn net.Conn) (net.Conn, error) {
	const (
		version = 5
	)
	var (
		err error
		buf [32]byte
	)
	err = writeStream(conn, []byte{version, 1, socks5MethodNoAuth}, authTimeout)
	if err != nil {
		return nil, err
	}
	err = readStream(conn, buf[:2], authTimeout)
	if err != nil {
		return nil, err
	}

	if buf[0] != version {
		return nil, errProxyVersionMismatch
	}
	if buf[1] != socks5MethodNoAuth {
		return nil, errProxyNeedsAuth
	}
	buf[0] = version
	buf[1] = socks5CommandConnect
	buf[2] = 0x00
	buf[3] = socks5ATyp
	// fmt.Printf("remote address: %#v %#v\n", appCfg.address, appCfg.address.To4())
	copy(buf[4:], appCfg.address.To4())
	binary.BigEndian.PutUint16(buf[8:], uint16(appCfg.port))
	fmt.Printf("connect requ: %#v\n", buf[:10])
	err = writeStream(conn, buf[:10], authTimeout)
	if err != nil {
		return nil, err
	}
	err = readStream(conn, buf[:4], authTimeout)
	fmt.Printf("response: %#v\n", buf[:4])
	if err != nil {
		return nil, err
	}
	if buf[0] != version {
		return nil, errProxyVersionMismatch
	}
	if buf[1] != socks5ReplySuccess {
		return nil, errProxyReturnErrorResponse
	}
	remainingBytes := 2 //port
	switch buf[3] {
	case socks5ReplyDomainName:
		return nil, errProxyResponseContainsDomainName
	case socks5ReplyIPv4:
		remainingBytes += 4
	case socks5ReplyIPv6:
		remainingBytes += 16
	}
	err = readStream(conn, buf[:remainingBytes], authTimeout)
	if err != nil {
		return nil, err

	}
	return conn, nil
}
