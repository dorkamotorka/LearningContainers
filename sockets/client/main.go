package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"

	"golang.org/x/sys/unix"
)

func main() {
	dialer := &net.Dialer{
			Control: func(network, address string, conn syscall.RawConn) error {
					var operr error
					if err := conn.Control(func(fd uintptr) {
							operr = syscall.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.TCP_QUICKACK, 1)
					}); err != nil {
							return err
					}
					return operr
			},
	}

	client := &http.Client{
			Transport: &http.Transport{
					DialContext: dialer.DialContext,
			},
	}

	resp, err := client.Get("http://example.com")
	if err != nil {
			panic(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
			panic(err)
	}
	fmt.Println(string(body))
}