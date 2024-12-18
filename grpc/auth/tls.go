package auth

import (
	"crypto/tls"
	"crypto/x509"
	"sync"
)

type TLSConfigCertificateManager struct {
	mu         sync.RWMutex
	cert       *tls.Certificate
	clientCert *tls.Certificate
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

func NewTLSConfig(hostname string, certPool *x509.CertPool, manager *TLSConfigCertificateManager) *tls.Config {
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
		GetCertificate:       manager.GetCertificate,
		GetClientCertificate: manager.GetClientCertificate,
	}
}
