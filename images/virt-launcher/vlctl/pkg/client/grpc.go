package client

import (
	"context"
	"net"
	"time"

	"google.golang.org/grpc"
)

const (
	ConnectTimeoutSeconds = 2
)

func DialSocket(socketPath string) (*grpc.ClientConn, error) {
	return DialSocketWithTimeout(socketPath, 0)
}

func DialSocketWithTimeout(socketPath string, timeout int) (*grpc.ClientConn, error) {

	options := []grpc.DialOption{
		grpc.WithAuthority("localhost"),
		grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
		grpc.WithBlock(), // dial sync in order to catch errors early
	}

	if timeout > 0 {
		options = append(options,
			grpc.WithTimeout(time.Duration(timeout+ConnectTimeoutSeconds)*time.Second),
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), ConnectTimeoutSeconds*time.Second)
	defer cancel()

	return grpc.DialContext(ctx, socketPath, options...)
}
