package auth

import (
	"crypto/x509"
)

// SatelliteCertValidator validates Satellite certificates for mTLS
type SatelliteCertValidator struct {
	caPool *x509.CertPool
}

// NewSatelliteCertValidator creates a new satellite certificate validator
func NewSatelliteCertValidator(caPool *x509.CertPool) *SatelliteCertValidator {
	return &SatelliteCertValidator{
		caPool: caPool,
	}
}

// Validate verifies the client certificate against the CA pool
func (v *SatelliteCertValidator) Validate(clientCert *x509.Certificate) error {
	// Verify the client certificate against the CA pool
	opts := x509.VerifyOptions{
		Roots: v.caPool,
	}
	_, err := clientCert.Verify(opts)
	return err
}
