/*
Copyright 2018 The KubeVirt Authors
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/ssh/native.go
*/

package ssh

import (
	"errors"
	"fmt"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
	"k8s.io/klog/v2"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	subv1alpha3 "github.com/deckhouse/virtualization/api/subresources/v1alpha3"
)

func (o *SSH) nativeSSH(namespace, name string, virtClient kubeclient.Client) error {
	conn := NewNativeSSHConnection(virtClient, o.options)
	client, err := conn.PrepareSSHClient(namespace, name)
	if err != nil {
		return err
	}
	return conn.StartSession(client, o.command)
}

func NewNativeSSHConnection(virtClient kubeclient.Client, options SSHOptions) *NativeSSHConnection {
	return &NativeSSHConnection{
		virtClient: virtClient,
		options:    options,
	}
}

type NativeSSHConnection struct {
	virtClient kubeclient.Client
	options    SSHOptions
}

func (o *NativeSSHConnection) PrepareSSHClient(namespace, name string) (*ssh.Client, error) {
	streamer, err := o.prepareSSHTunnel(namespace, name)
	if err != nil {
		return nil, err
	}

	conn := streamer.AsConn()
	addr := fmt.Sprintf("%s.%s:%d", name, namespace, o.options.SSHPort)
	authMethods := o.getAuthMethods(namespace, name)

	hostKeyCallback := ssh.InsecureIgnoreHostKey()
	if len(o.options.KnownHostsFilePath) > 0 {
		hostKeyCallback, err = InteractiveHostKeyCallback(o.options.KnownHostsFilePath)
		if err != nil {
			return nil, err
		}
	} else {
		fmt.Println("WARNING: skipping hostkey check, provide --known-hosts to fix this")
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn,
		addr,
		&ssh.ClientConfig{
			HostKeyCallback: hostKeyCallback,
			Auth:            authMethods,
			User:            o.options.SSHUsername,
		},
	)
	if err != nil {
		return nil, err
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

func (o *NativeSSHConnection) getAuthMethods(namespace, name string) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	methods = o.trySSHAgent(methods)
	methods = o.tryPrivateKey(methods)

	methods = append(methods, ssh.PasswordCallback(func() (secret string, err error) {
		password, err := readPassword(fmt.Sprintf("%s@%s.%s's password: ", o.options.SSHUsername, name, namespace))
		fmt.Println()
		return string(password), err
	}))

	return methods
}

func (o *NativeSSHConnection) trySSHAgent(methods []ssh.AuthMethod) []ssh.AuthMethod {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if len(socket) < 1 {
		return methods
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		klog.Error("no connection to ssh agent, skipping agent authentication:", err)
		return methods
	}
	agentClient := agent.NewClient(conn)

	return append(methods, ssh.PublicKeysCallback(agentClient.Signers))
}

func (o *NativeSSHConnection) tryPrivateKey(methods []ssh.AuthMethod) []ssh.AuthMethod {
	// If the identity file at the default does not exist but was
	// not explicitly provided, don't add the authentication mechanism.
	if !o.options.IdentityFilePathProvided {
		if _, err := os.Stat(o.options.IdentityFilePath); errors.Is(err, os.ErrNotExist) {
			klog.V(3).Infof("No ssh key at the default location %q found, skipping RSA authentication.", o.options.IdentityFilePath)
			return methods
		}
	}

	callback := ssh.PublicKeysCallback(func() (signers []ssh.Signer, err error) {
		key, err := os.ReadFile(o.options.IdentityFilePath)
		if err != nil {
			return nil, err
		}

		signer, err := ssh.ParsePrivateKey(key)
		var passphraseMissingError *ssh.PassphraseMissingError
		if errors.As(err, &passphraseMissingError) {
			signer, err = o.parsePrivateKeyWithPassphrase(key)
		}

		if err != nil {
			return nil, err
		}

		return []ssh.Signer{signer}, nil
	})

	return append(methods, callback)
}

func (o *NativeSSHConnection) parsePrivateKeyWithPassphrase(key []byte) (ssh.Signer, error) {
	password, err := readPassword(fmt.Sprintf("Key %s requires a password: ", o.options.IdentityFilePath))
	fmt.Println()
	if err != nil {
		return nil, err
	}

	return ssh.ParsePrivateKeyWithPassphrase(key, password)
}

func readPassword(reason string) ([]byte, error) {
	fmt.Print(reason)
	return term.ReadPassword(int(os.Stdin.Fd()))
}

func (o *NativeSSHConnection) StartSession(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = os.Stdin
	session.Stderr = os.Stderr
	session.Stdout = os.Stdout

	if command != "" {
		if err := session.Run(command); err != nil {
			return err
		}
		return nil
	}

	restore, err := setupTerminal()
	if err != nil {
		return err
	}
	defer restore()

	if err := requestPty(session); err != nil {
		return err
	}
	if err := session.Shell(); err != nil {
		return err
	}

	err = session.Wait()
	var exitError *ssh.ExitError
	if !errors.As(err, &exitError) {
		return err
	}
	return nil
}

func (o *NativeSSHConnection) prepareSSHTunnel(namespace, name string) (virtualizationv1alpha2.StreamInterface, error) {
	opts := subv1alpha3.VirtualMachinePortForward{
		Port:     o.options.SSHPort,
		Protocol: "tcp",
	}
	stream, err := o.virtClient.VirtualMachines(namespace).PortForward(name, opts)
	if err != nil {
		return nil, fmt.Errorf("can't access VM %s: %w", name, err)
	}

	return stream, nil
}
