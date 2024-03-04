package filesystem

import (
	"crypto/tls"
	"fmt"
	"github.com/deckhouse/virtualization-controller/pkg/tls/util"
	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileCertificateManager struct {
	stopCh             chan struct{}
	certAccessLock     sync.Mutex
	stopped            bool
	cert               *tls.Certificate
	certBytesPath      string
	keyBytesPath       string
	errorRetryInterval time.Duration
}

func NewFileCertificateManager(certBytesPath string, keyBytesPath string) *FileCertificateManager {
	return &FileCertificateManager{
		certBytesPath:      certBytesPath,
		keyBytesPath:       keyBytesPath,
		stopCh:             make(chan struct{}, 1),
		errorRetryInterval: 1 * time.Minute,
	}
}

func (f *FileCertificateManager) Start() {
	objectUpdated := make(chan struct{}, 1)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		klog.Errorf("failed to create an inotify watcher: %v", err)
	}
	defer watcher.Close()

	certDir := filepath.Dir(f.certBytesPath)
	err = watcher.Add(certDir)
	if err != nil {
		klog.Errorf("failed to establish a watch on %s: %v", f.certBytesPath, err)
	}
	keyDir := filepath.Dir(f.keyBytesPath)
	if keyDir != certDir {
		err = watcher.Add(keyDir)
		if err != nil {
			klog.Errorf("failed to establish a watch on %s: %v", f.keyBytesPath, err)
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
					klog.V(5).Infof("Dropping redundant wakeup for cert reload")
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				klog.ErrorS(err, fmt.Sprintf("An error occurred when watching certificates files %s and %s", f.certBytesPath, f.keyBytesPath))
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
						klog.V(5).Infof("Dropping redundant wakeup for cert reload")
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
	if f.stopped {
		return
	}
	close(f.stopCh)
	f.stopped = true
}

func (f *FileCertificateManager) rotateCerts() error {
	crt, err := f.loadCertificates()
	if err != nil {
		return fmt.Errorf("failed to load the certificate %s and %s", f.certBytesPath, f.keyBytesPath)
	}

	f.certAccessLock.Lock()
	defer f.certAccessLock.Unlock()
	// update after the callback, to ensure that the reconfiguration succeeded
	f.cert = crt
	klog.Infof("certificate with common name '%s' retrieved.", crt.Leaf.Subject.CommonName)
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
		return nil, fmt.Errorf("failed to load certificate: %v\n", err)
	}

	leaf, err := util.ParseCertsPEM(certBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to load leaf certificate: %v\n", err)
	}
	crt.Leaf = leaf[0]
	return &crt, nil
}

func (f *FileCertificateManager) Current() *tls.Certificate {
	f.certAccessLock.Lock()
	defer f.certAccessLock.Unlock()
	return f.cert
}
