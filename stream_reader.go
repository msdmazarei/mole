package main

import (
	"net"
	"time"
)

func readStream(conn net.Conn, buf []byte, timeout time.Duration) error {
	var (
		i   int = 0
		n   int
		err error
	)

	for {
		err = conn.SetReadDeadline(time.Now().Add(timeout))
		if err != nil {
			return err
		}
		n, err = conn.Read(buf[i:])
		if err != nil {
			return err
		}
		i += n
		if i == len(buf) {
			return nil
		}
	}
}

func writeStream(conn net.Conn, buf []byte, timeout time.Duration) error {
	var (
		i   int = 0
		n   int
		err error
	)
	for {
		err = conn.SetWriteDeadline(time.Now().Add(timeout))
		if err != nil {
			return err
		}
		n, err = conn.Write(buf[i:])
		if err != nil {
			return err
		}
		i += n
		if i == len(buf) {
			return nil
		}
	}
}
