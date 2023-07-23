package main

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

func connectThroughHttpProxy(conn net.Conn, timeout time.Duration) error {
	var (
		err error
		res *http.Response
	)
	req := fmt.Sprintf("CONNECT %s:%d HTTP/1.1\r\n", appCfg.address, appCfg.port)

	if appCfg.httpForwardProxyUsername != "" {
		auth := appCfg.httpForwardProxyUsername + ":" + appCfg.httpForwardProxyPassword
		req += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", base64.StdEncoding.EncodeToString([]byte(auth)))
	}
	req += "\r\n"
	_, err = conn.Write([]byte(req))
	if err != nil {
		return err
	}
	res, err = http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return err
	}
	switch res.StatusCode {
	case 403:
		return errors.New("proxy authentication Failed")

	case 407:
		return errors.New("proxy authentication Failed")
	case 200:
		return nil
	default:
		return fmt.Errorf("proxy server returned %d error", res.StatusCode)
	}

}
