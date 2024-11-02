/*
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
*/

package filesystem

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	logutil "github.com/deckhouse/kube-api-rewriter/pkg/log"
	"github.com/deckhouse/kube-api-rewriter/pkg/tls/util"
)

type FileCertificateManager struct {
	stopCh             chan struct{}
	certAccessLock     sync.Mutex
	cert               *tls.Certificate
	certBytesPath      string
	keyBytesPath       string
	errorRetryInterval time.Duration
}

func NewFileCertificateManager(certBytesPath, keyBytesPath string) *FileCertificateManager {
	return &FileCertificateManager{
		certBytesPath:      certBytesPath,
		keyBytesPath:       keyBytesPath,
		stopCh:             make(chan struct{}),
		errorRetryInterval: 1 * time.Minute,
	}
}

func (f *FileCertificateManager) Start() {
	objectUpdated := make(chan struct{}, 1)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("failed to create an inotify watcher", logutil.SlogErr(err))
	}
	defer watcher.Close()

	certDir := filepath.Dir(f.certBytesPath)
	err = watcher.Add(certDir)
	if err != nil {
		slog.Error(fmt.Sprintf("failed to establish a watch on %s", f.certBytesPath), logutil.SlogErr(err))
	}
	keyDir := filepath.Dir(f.keyBytesPath)
	if keyDir != certDir {
		err = watcher.Add(keyDir)
		if err != nil {
			slog.Error(fmt.Sprintf("failed to establish a watch on %s", f.keyBytesPath), logutil.SlogErr(err))
		}
	}

	go func() {
		for {
			select {
			case _, ok := <-watcher.Events:
				if !ok {
					return
				}
				select {
				case objectUpdated <- struct{}{}:
				default:
					slog.Debug("Dropping redundant wakeup for cert reload")
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				slog.Error(fmt.Sprintf("An error occurred when watching certificates files %s and %s", f.certBytesPath, f.keyBytesPath), logutil.SlogErr(err))
			}
		}
	}()

	// ensure we load the certificates on startup
	objectUpdated <- struct{}{}

sync:
	for {
		select {
		case <-objectUpdated:
			if err := f.rotateCerts(); err != nil {
				go func() {
					time.Sleep(f.errorRetryInterval)
					select {
					case objectUpdated <- struct{}{}:
					default:
						slog.Debug("Dropping redundant wakeup for cert reload")
					}
				}()
			}
		case <-f.stopCh:
			break sync
		}
	}
}

func (f *FileCertificateManager) Stop() {
	f.certAccessLock.Lock()
	defer f.certAccessLock.Unlock()
	select {
	case <-f.stopCh:
	default:
		close(f.stopCh)
	}
}

func (f *FileCertificateManager) rotateCerts() error {
	crt, err := f.loadCertificates()
	if err != nil {
		return fmt.Errorf("failed to load the certificate %s and %s: %w", f.certBytesPath, f.keyBytesPath, err)
	}

	f.certAccessLock.Lock()
	defer f.certAccessLock.Unlock()
	// update after the callback, to ensure that the reconfiguration succeeded
	f.cert = crt
	slog.Info(fmt.Sprintf("certificate with common name '%s' retrieved.", crt.Leaf.Subject.CommonName))
	return nil
}

func (f *FileCertificateManager) loadCertificates() (serverCrt *tls.Certificate, err error) {
	// #nosec No risk for path injection. Used for specific cert file for key rotation
	certBytes, err := os.ReadFile(f.certBytesPath)
	if err != nil {
		return nil, err
	}
	// #nosec No risk for path injection. Used for specific cert file for key rotation
	keyBytes, err := os.ReadFile(f.keyBytesPath)
	if err != nil {
		return nil, err
	}

	crt, err := tls.X509KeyPair(certBytes, keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w\n", err)
	}

	leaf, err := util.ParseCertsPEM(certBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to load leaf certificate: %w\n", err)
	}
	crt.Leaf = leaf[0]
	return &crt, nil
}

func (f *FileCertificateManager) Current() *tls.Certificate {
	f.certAccessLock.Lock()
	defer f.certAccessLock.Unlock()
	return f.cert
}
