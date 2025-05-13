/*
Copyright 2018 The KubeVirt Authors.
Copyright 2024 Flant JSC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/utils/utils.go
*/

package util

import (
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

var ErrorInterrupt = errors.New("interrupt")

func AttachConsole(stdinReader, stdoutReader *io.PipeReader, stdinWriter, stdoutWriter *io.PipeWriter, name string, resChan <-chan error) (err error) {
	writeStop := make(chan error)
	readStop := make(chan error)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		state, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("make raw terminal failed: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), state)
	}

	fmt.Fprintf(os.Stderr, "Successfully connected to %s console. The escape sequence is ^]\n", name)

	out := os.Stdout
	go func() {
		defer close(readStop)
		_, err := io.Copy(out, stdoutReader)
		readStop <- err
	}()

	stdinCh := make(chan []byte)
	go func() {
		in := os.Stdin
		defer close(stdinCh)
		buf := make([]byte, 1024)
		for {
			// reading from stdin
			n, err := in.Read(buf)
			if err != nil && err != io.EOF {
				return
			}
			if n == 0 && err == io.EOF {
				return
			}

			// the escape sequence
			if buf[0] == 29 {
				return
			}

			stdinCh <- buf[0:n]
		}
	}()

	go func() {
		defer close(writeStop)

		stdinWriter.Write([]byte("\r"))
		if err == io.EOF {
			return
		}

		for b := range stdinCh {
			_, err = stdinWriter.Write(b)
			if err == io.EOF {
				return
			}
		}

		os.Exit(0)
	}()

	select {
	case err = <-writeStop:
		return ErrorInterrupt
	case err = <-readStop:
		return ErrorInterrupt
	case err = <-resChan:
		return err
	}
}
