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

package usbip

type Interface interface {
	ServerInterface
	ClientInterface
}

type ServerInterface interface {
	USBBinder
}

type ClientInterface interface {
	USBAttacher
}

type USBBinder interface {
	Bind(busID string) error
	Unbind(busID string) error
	IsBound(busID string) (bool, error)
}

type USBAttacher interface {
	Attach(host, busID string, port int) (int, error)
	Detach(port int) error
	AttachInfoGetter
}

type AttachInfoGetter interface {
	GetAttachInfo() ([]AttachInfo, error)
}

type AttachInfo struct {
	Port, Busnum, Devnum int
	LocalBusID           string
}

type serverImpl struct {
	USBBinder
}

func NewServer(binder USBBinder) ServerInterface {
	return &serverImpl{USBBinder: binder}
}

type clientImpl struct {
	USBAttacher
}

func NewClient(attacher USBAttacher) ClientInterface {
	return &clientImpl{USBAttacher: attacher}
}

type interfaceImpl struct {
	ServerInterface
	ClientInterface
}

func NewInterface(server ServerInterface, client ClientInterface) Interface {
	return &interfaceImpl{
		ServerInterface: server,
		ClientInterface: client,
	}
}

func New() Interface {
	binder := NewUSBBinder()
	attacher := NewUSBAttacher()

	server := NewServer(binder)
	client := NewClient(attacher)

	return NewInterface(server, client)
}
