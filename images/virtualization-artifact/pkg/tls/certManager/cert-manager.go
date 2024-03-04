package certManager

import (
	"crypto/tls"
)

type CertificateManager interface {
	Start()
	Stop()
	Current() *tls.Certificate
}
