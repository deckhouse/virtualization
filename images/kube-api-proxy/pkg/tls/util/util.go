package util

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
)

const CertificateBlockType string = "CERTIFICATE"

func ParseCertsPEM(pemCerts []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	for len(pemCerts) > 0 {
		var block *pem.Block
		block, pemCerts = pem.Decode(pemCerts)
		if block == nil {
			break
		}
		// Only use PEM "CERTIFICATE" blocks without extra headers
		if block.Type != CertificateBlockType || len(block.Headers) != 0 {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return certs, err
		}

		certs = append(certs, cert)
	}

	if len(certs) == 0 {
		return nil, errors.New("data does not contain any valid RSA or ECDSA certificates")
	}
	return certs, nil
}
