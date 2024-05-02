package certmanager

import (
	"crypto/tls"
)

type CertificateManager interface {
	Start()
	Stop()
	Current() *tls.Certificate
}
