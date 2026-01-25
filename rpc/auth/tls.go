package auth

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"sync"

	"git.tatikoma.dev/corpix/atlas/errors"
)

type TLSConfigCertificateManager struct {
	cert       *tls.Certificate
	clientCert *tls.Certificate
	mu         sync.RWMutex
}

func (cm *TLSConfigCertificateManager) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.cert, nil
}

func (cm *TLSConfigCertificateManager) GetClientCertificate(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.clientCert, nil
}

func (cm *TLSConfigCertificateManager) LoadCertificate(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.cert = &cert

	return nil
}

func (cm *TLSConfigCertificateManager) LoadClientCertificate(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.clientCert = &cert

	return nil
}

func NewTLSConfigCertificateManager() *TLSConfigCertificateManager {
	return &TLSConfigCertificateManager{}
}

func NewTLSConfigWithManager(hostname string, certPool *x509.CertPool, manager *TLSConfigCertificateManager) *tls.Config {
	tc := newBaseTLSConfig(hostname, certPool)
	tc.GetCertificate = manager.GetCertificate
	tc.GetClientCertificate = manager.GetClientCertificate
	return tc
}

func NewTLSConfig(hostname, caPath, certPath, keyPath string) (*tls.Config, error) {
	certPool, err := NewCertPoolFromFile(caPath)
	if err != nil {
		return nil, err
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	tc := newBaseTLSConfig(hostname, certPool)
	tc.Certificates = []tls.Certificate{cert}
	return tc, nil
}

func NewCertPoolFromFile(caPath string) (*x509.CertPool, error) {
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read CA cert %q", caPath)
	}
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ca); !ok {
		return nil, errors.New("failed to append CA certificate")
	}
	return certPool, nil
}

func ApplyClientCertPolicy(tc *tls.Config) {
	tc.ClientAuth = tls.VerifyClientCertIfGiven
	tc.ClientCAs = tc.RootCAs
}

func newBaseTLSConfig(hostname string, certPool *x509.CertPool) *tls.Config {
	return &tls.Config{
		ServerName: hostname,
		RootCAs:    certPool,
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		NextProtos: []string{"h2", "http/1.1"},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}
}

func isClientCertificate(cert *x509.Certificate) bool {
	if len(cert.ExtKeyUsage) == 0 {
		return false
	}
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageClientAuth {
			return true
		}
	}
	return false
}
