package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"syscall"

	"golang.org/x/sys/unix"
)

// Ref: https://iximiuz.com/en/posts/go-net-http-setsockopt-example/
func main() {

		// Ref: src/net/dial.go
		//func Listen(network, address string) (Listener, error) {
		//	var lc ListenConfig
		//	return lc.Listen(context.Background(), network, address)
		//}
		// So, Listen() is actually a method of some obscure ListenConfig type. 
    lc := net.ListenConfig{
				// And ListenConfig has a field called Control holding a function with the following signature and comment
				//
				// If Control is not nil, it is called after creating the network
				// connection but before binding it to the operating system.
        Control: func(network, address string, conn syscall.RawConn) error {
            var operr error
						// Control invokes f on the underlying connection's file descriptor or handle.
						// The file descriptor fd is guaranteed to remain valid while f executes but not after f returns.
						// Function definition: func (c RawConn) Control(f func(fd uintptr)) error
            if err := conn.Control(func(fd uintptr) {
								// A wrapper arounf setsockopt syscall which requires a file descriptor and we need to somehow unpack it from the http.Handle() or http.ListenAndServe()
                operr = syscall.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
            }); err != nil {
                return err
            }
            return operr
        },
    }

		// setting the socket options needs to happen before the bind() call.
		// Since net.Listen() call returns an already bound socket, we need to hook into a ListenConfig to figure out the fd.
    ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:8080")
    if err != nil {
        panic(err)
    }

    http.HandleFunc("/", func(w http.ResponseWriter, _req *http.Request) {
        w.Write([]byte("Hello, world!\n"))
    })

		fmt.Println("HTTP server is running...")
    if err := http.Serve(ln, nil); err != nil {
        panic(err)
    }
		/*
		For sockets in LISTEN states (passive):
			- Recv-Q: The size of the current accept queue, which are the connections that have 3-way handshake completed and are waiting for the application accept() TCP connections.
			- Send-Q: the maximum length of the accept queue, which is the size of the accept queue.
		
		For sockets in non-LISTEN state (active):
			-	Recv-Q: the number of bytes received but not read by the application.
			-	Send-Q: the number of bytes sent but not acknowledged.

		View it using `ss -ltp`
		*/
}