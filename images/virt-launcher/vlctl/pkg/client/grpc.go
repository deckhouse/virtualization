/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
