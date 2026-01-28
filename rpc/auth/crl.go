package auth

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"sync"
	"time"

	"git.tatikoma.dev/corpix/atlas/errors"
)

const (
	CRLPolicyLoose CRLPolicy = iota
	CRLPolicyStrict
)

type (
	CRLPolicy int

	CRLVerifier struct {
		path   string
		policy CRLPolicy

		mu      sync.Mutex
		modTime time.Time
		crl     *x509.RevocationList
	}
)

func NewCRLVerifier(path string, policy CRLPolicy) *CRLVerifier {
	return &CRLVerifier{path: path, policy: policy}
}

func ApplyCRLVerifier(tc *tls.Config, verifier *CRLVerifier) {
	prev := tc.VerifyPeerCertificate
	tc.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if prev != nil {
			err := prev(rawCerts, verifiedChains)
			if err != nil {
				return err
			}
		}
		return verifier.Verify(rawCerts, verifiedChains)
	}
}

func (v *CRLVerifier) Verify(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return nil
	}

	rl, err := v.load()
	if err != nil || rl == nil {
		return err
	}

	if !rl.NextUpdate.IsZero() && time.Now().After(rl.NextUpdate) {
		return v.policyError(errors.New("crl is expired"))
	}

	err = v.verifyCRLSig(rl, verifiedChains)
	if err != nil {
		return v.policyError(err)
	}

	leaf, err := v.leafFromPeer(rawCerts, verifiedChains)
	if err != nil {
		return err
	}

	if leaf == nil {
		// not found, should be fine
		return nil
	}
	if v.isSerialRevoked(rl, leaf.SerialNumber) {
		return errors.New("certificate is revoked")
	}

	return nil
}

func (v *CRLVerifier) load() (*x509.RevocationList, error) {
	info, err := os.Stat(v.path)
	if err != nil {
		return nil, v.policyError(err)
	}
	if info.Size() == 0 {
		return nil, v.policyError(errors.New("crl is empty"))
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if v.crl != nil && info.ModTime().Equal(v.modTime) {
		return v.crl, nil
	}

	// CRL changed, reloading
	data, err := os.ReadFile(v.path)
	if err != nil {
		return nil, v.policyError(err)
	}
	rl, err := parseCRL(data)
	if err != nil {
		return nil, v.policyError(err)
	}
	v.crl = rl
	v.modTime = info.ModTime()

	return rl, nil
}

func (v *CRLVerifier) policyError(err error) error {
	if v.policy == CRLPolicyStrict {
		return err
	}
	return nil
}

func (*CRLVerifier) verifyCRLSig(rl *x509.RevocationList, verifiedChains [][]*x509.Certificate) error {
	for _, chain := range verifiedChains {
		for i := len(chain) - 1; i >= 0; i-- {
			if err := rl.CheckSignatureFrom(chain[i]); err == nil {
				return nil
			}
		}
	}
	return errors.New("failed to verify crl signature")
}

func (*CRLVerifier) leafFromPeer(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) (*x509.Certificate, error) {
	if len(verifiedChains) > 0 && len(verifiedChains[0]) > 0 {
		return verifiedChains[0][0], nil
	}
	if len(rawCerts) == 0 {
		return nil, nil
	}
	return x509.ParseCertificate(rawCerts[0])
}

func (*CRLVerifier) isSerialRevoked(rl *x509.RevocationList, serial *big.Int) bool {
	for _, entry := range rl.RevokedCertificateEntries {
		if entry.SerialNumber != nil && entry.SerialNumber.Cmp(serial) == 0 {
			return true
		}
	}
	return false
}

func parseCRL(data []byte) (*x509.RevocationList, error) {
	if block, _ := pem.Decode(data); block != nil {
		if block == nil {
			return nil, errors.New("failed to decode data as PEM")
		}
		data = block.Bytes
	}
	return x509.ParseRevocationList(data)
}

func nextCRLNumber(rl *x509.RevocationList) *big.Int {
	if rl == nil || rl.Number == nil {
		return big.NewInt(1)
	}
	return new(big.Int).Add(rl.Number, big.NewInt(1))
}
