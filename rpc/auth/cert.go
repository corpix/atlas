package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"git.tatikoma.dev/corpix/atlas/errors"
)

const (
	CAKeyFile  = "ca-key.pem"
	CACertFile = "ca-cert.pem"
	CRLFile    = "ca-crl.pem"
	SerialFile = "serial"

	DefaultCRLValidity = 24 * 7 * time.Hour // week
)

type (
	CertTool struct {
		*CertTypeRegistry
	}
	CertType struct {
		KeyFile  string
		CertFile string
	}
	CertTypeRegistry struct {
		types map[string]CertType
		mu    sync.RWMutex
	}

	CertToolGenerateOptions struct {
		Country      string
		NameSuffix   string
		Type         string
		CAKeyPath    string
		CACertPath   string
		IPAddresses  string
		DNSNames     string
		CommonName   string
		NamePrefix   string
		Capabilities []string
		ExtKeyUsage  []x509.ExtKeyUsage
		KeyUsage     x509.KeyUsage
		GenerateCA   bool
	}

	CertToolRevokeOptions struct {
		NamePrefix     string
		CACertPath     string
		CAKeyPath      string
		CRLPath        string
		CertPath       string
		SerialNumber   string
		ReasonCode     int
		RevocationTime time.Time
		CRLValidity    time.Duration
	}

	CertToolCRLInitOptions struct {
		NamePrefix  string
		CACertPath  string
		CAKeyPath   string
		CRLPath     string
		CRLValidity time.Duration
	}
)

func NewCertTypeRegistry() *CertTypeRegistry {
	return &CertTypeRegistry{types: map[string]CertType{}}
}

// Register registers a new certificate type for generation.
func (r *CertTypeRegistry) Register(name string, certType CertType) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("certificate type name is required")
	}
	if strings.TrimSpace(certType.KeyFile) == "" || strings.TrimSpace(certType.CertFile) == "" {
		return errors.New("certificate key and cert file names are required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.types[name]; exists {
		return errors.Errorf("certificate type %q already registered", name)
	}

	r.types[name] = certType
	return nil
}

func (r *CertTypeRegistry) Lookup(name string) (CertType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	certType, ok := r.types[name]
	if !ok {
		return CertType{}, errors.Errorf("unknown certificate type: %s", name)
	}

	return certType, nil
}

// Generate creates certificates based on options. Caller is responsible for synchronization.
func (ct *CertTool) Generate(opts CertToolGenerateOptions) error {
	if opts.GenerateCA {
		return ct.generateCA(opts)
	}
	if strings.TrimSpace(opts.Type) == "" {
		return errors.New("certificate type is required")
	}

	certType, err := ct.Lookup(opts.Type)
	if err != nil {
		return err
	}

	err = ct.generateCerts(opts, certType)
	if err != nil {
		return errors.Errorf("error generating certificates: %w", err)
	}

	return nil
}

// Revoke updates or creates a CRL entry for a certificate or serial number.
func (ct *CertTool) Revoke(opts CertToolRevokeOptions) error {
	serial, err := ct.resolveRevocationSerial(opts)
	if err != nil {
		return err
	}

	caCertPath := ct.caCertPathWithPrefix(opts.NamePrefix, opts.CACertPath)
	caKeyPath := ct.caKeyPathWithPrefix(opts.NamePrefix, opts.CAKeyPath)
	caCert, caKey, err := ct.readCAFiles(caCertPath, caKeyPath)
	if err != nil {
		return err
	}

	if len(caCert.SubjectKeyId) == 0 {
		subjectKeyID, err := ct.subjectKeyID(caCert.PublicKey)
		if err != nil {
			return err
		}
		caCert.SubjectKeyId = subjectKeyID
	}

	crlPath := strings.TrimSpace(opts.CRLPath)
	if crlPath == "" {
		return nil
	}

	crlPath = ct.crlPathWithPrefix(opts.NamePrefix, crlPath)
	rl, err := ct.readCRL(crlPath, caCert)
	if err != nil {
		return err
	}

	entries := revokedEntriesFromList(rl)
	if !revocationListHasSerial(entries, serial) {
		revocationTime := opts.RevocationTime
		if revocationTime.IsZero() {
			revocationTime = time.Now()
		}
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber:   serial,
			RevocationTime: revocationTime,
			ReasonCode:     opts.ReasonCode,
		})
	}

	validity := opts.CRLValidity
	if validity == 0 {
		validity = DefaultCRLValidity
	}
	if validity < 0 {
		return errors.New("crl validity must be positive")
	}

	now := time.Now()
	number := nextCRLNumber(rl)
	crl := &x509.RevocationList{
		RevokedCertificateEntries: entries,
		Number:                    number,
		ThisUpdate:                now,
		NextUpdate:                now.Add(validity),
	}
	crlBytes, err := x509.CreateRevocationList(rand.Reader, crl, caCert, caKey)
	if err != nil {
		return err
	}

	return ct.writePEMFile(crlPath, "X509 CRL", crlBytes)
}

// InitCRL creates a new empty CRL.
func (ct *CertTool) InitCRL(opts CertToolCRLInitOptions) error {
	crlPath := ct.crlPathWithPrefix(opts.NamePrefix, strings.TrimSpace(opts.CRLPath))
	if crlPath == "" {
		return errors.New("crl path is required")
	}

	caCertPath := ct.caCertPathWithPrefix(opts.NamePrefix, opts.CACertPath)
	caKeyPath := ct.caKeyPathWithPrefix(opts.NamePrefix, opts.CAKeyPath)
	caCert, caKey, err := ct.readCAFiles(caCertPath, caKeyPath)
	if err != nil {
		return err
	}

	if len(caCert.SubjectKeyId) == 0 {
		subjectKeyID, err := ct.subjectKeyID(caCert.PublicKey)
		if err != nil {
			return err
		}
		caCert.SubjectKeyId = subjectKeyID
	}

	validity := opts.CRLValidity
	if validity == 0 {
		validity = DefaultCRLValidity
	}
	if validity < 0 {
		return errors.New("crl validity must be positive")
	}

	now := time.Now()
	crl := &x509.RevocationList{
		RevokedCertificateEntries: nil,
		Number:                    big.NewInt(1),
		ThisUpdate:                now,
		NextUpdate:                now.Add(validity),
	}
	crlBytes, err := x509.CreateRevocationList(rand.Reader, crl, caCert, caKey)
	if err != nil {
		return err
	}

	return ct.writePEMFile(crlPath, "X509 CRL", crlBytes)
}

func (ct *CertTool) namespace(opts CertToolGenerateOptions, fileName string) string {
	return ct.namespacePrefix(opts.NamePrefix, fileName)
}

func (ct *CertTool) namespacePrefix(namePrefix, fileName string) string {
	if namePrefix != "" {
		return namePrefix + "." + fileName
	}
	return fileName
}

func (ct *CertTool) certFileName(opts CertToolGenerateOptions, fileName string) string {
	if opts.NameSuffix != "" {
		ext := filepath.Ext(fileName)
		base := strings.TrimSuffix(fileName, ext)
		if ext == "" {
			fileName = base + "." + opts.NameSuffix
		} else {
			fileName = base + "." + opts.NameSuffix + ext
		}
	}
	return ct.namespace(opts, fileName)
}

func (ct *CertTool) caKeyPath(opts CertToolGenerateOptions) string {
	return ct.caKeyPathWithPrefix(opts.NamePrefix, opts.CAKeyPath)
}

func (ct *CertTool) caCertPath(opts CertToolGenerateOptions) string {
	return ct.caCertPathWithPrefix(opts.NamePrefix, opts.CACertPath)
}

func (ct *CertTool) caKeyPathWithPrefix(namePrefix, path string) string {
	if path != "" {
		return path
	}
	return ct.namespacePrefix(namePrefix, CAKeyFile)
}

func (ct *CertTool) caCertPathWithPrefix(namePrefix, path string) string {
	if path != "" {
		return path
	}
	return ct.namespacePrefix(namePrefix, CACertFile)
}

func (ct *CertTool) crlPathWithPrefix(namePrefix, path string) string {
	if path != "" {
		return path
	}
	return ct.namespacePrefix(namePrefix, CRLFile)
}

func (ct *CertTool) loadSerial(opts CertToolGenerateOptions) (*big.Int, error) {
	serialFilePath := ct.namespace(opts, SerialFile)
	if !ct.fileExists(serialFilePath) {
		err := os.WriteFile(serialFilePath, []byte("1"), 0o660)
		if err != nil {
			return nil, errors.Errorf("error initializing cert serial number cache: %v", err)
		}
	}
	buf, err := os.ReadFile(serialFilePath)
	if err != nil {
		return nil, errors.Errorf("error reading cert serial number cache: %v", err)
	}

	serial := big.NewInt(0)
	var ok bool
	serial, ok = serial.SetString(strings.TrimSpace(string(buf)), 10)
	if !ok {
		return nil, errors.Errorf("error setting serial from cache: %v", string(buf))
	}

	return serial, nil
}

func (ct *CertTool) saveSerial(opts CertToolGenerateOptions, serial *big.Int) error {
	return os.WriteFile(ct.namespace(opts, SerialFile), []byte(serial.String()), 0o660)
}

func (ct *CertTool) generateCerts(opts CertToolGenerateOptions, certType CertType) error {
	if !ct.fileExists(ct.caKeyPath(opts)) {
		err := ct.generateCA(opts)
		if err != nil {
			return errors.Errorf("generating CA: %w", err)
		}
	}

	serial, err := ct.loadSerial(opts)
	if err != nil {
		return errors.Errorf("error loading serial: %w", err)
	}
	defer func() {
		err := ct.saveSerial(opts, serial)
		if err != nil {
			fmt.Printf("error saving serial: %v\n", err)
		}
	}()

	caCert, caKey, err := ct.readCA(opts)
	if err != nil {
		return errors.Errorf("reading CA: %w", err)
	}

	return ct.generateCert(opts, certType, serial, caCert, caKey)
}

func (ct *CertTool) generateCA(opts CertToolGenerateOptions) error {
	serial, err := ct.loadSerial(opts)
	if err != nil {
		return errors.Errorf("error loading serial: %w", err)
	}
	defer func() {
		err := ct.saveSerial(opts, serial)
		if err != nil {
			fmt.Printf("error saving serial: %v\n", err)
		}
	}()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	subjectKeyID, err := ct.subjectKeyID(&key.PublicKey)
	if err != nil {
		return err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: opts.CommonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		SubjectKeyId:          subjectKeyID,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	ct.applyCountry(template, opts.Country)

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}

	err = ct.writePEMFile(ct.caCertPath(opts), "CERTIFICATE", certBytes)
	if err != nil {
		return err
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}

	return ct.writePEMFile(ct.caKeyPath(opts), "EC PRIVATE KEY", keyBytes)
}

func (ct *CertTool) generateCert(opts CertToolGenerateOptions, certType CertType, serial *big.Int, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serial.Set(serial.Add(serial, big.NewInt(1)))

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: opts.CommonName,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(10, 0, 0),
	}
	ct.applyCountry(template, opts.Country)
	ct.applyAltNames(template, opts.IPAddresses, opts.DNSNames)
	ct.applyKeyUsage(template, opts.KeyUsage, opts.ExtKeyUsage)

	err = ct.applyCapabilities(template, opts.Capabilities)
	if err != nil {
		return err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return err
	}

	err = ct.writePEMFile(ct.certFileName(opts, certType.CertFile), "CERTIFICATE", certBytes)
	if err != nil {
		return err
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}

	return ct.writePEMFile(ct.certFileName(opts, certType.KeyFile), "EC PRIVATE KEY", keyBytes)
}

func (ct *CertTool) applyCountry(template *x509.Certificate, country string) {
	country = strings.TrimSpace(country)
	if country == "" {
		return
	}
	template.Subject.Country = []string{strings.ToUpper(country)}
}

func (ct *CertTool) applyAltNames(template *x509.Certificate, ipAddresses, dnsNames string) {
	for _, ip := range strings.Split(ipAddresses, ",") {
		v := strings.TrimSpace(ip)
		if v == "" {
			continue
		}
		template.IPAddresses = append(template.IPAddresses, net.ParseIP(v))
	}
	for _, hostname := range strings.Split(dnsNames, ",") {
		v := strings.TrimSpace(hostname)
		if v == "" {
			continue
		}
		template.DNSNames = append(template.DNSNames, v)
	}
}

func (ct *CertTool) applyKeyUsage(template *x509.Certificate, keyUsage x509.KeyUsage, extKeyUsage []x509.ExtKeyUsage) {
	if keyUsage != 0 {
		template.KeyUsage = keyUsage
	}
	if len(extKeyUsage) > 0 {
		template.ExtKeyUsage = extKeyUsage
	}
}

func (ct *CertTool) applyCapabilities(template *x509.Certificate, capabilities []string) error {
	if len(capabilities) == 0 {
		return nil
	}

	capJSONBytes, err := json.Marshal(capabilities)
	if err != nil {
		return err
	}
	capBytes, err := asn1.Marshal(string(capJSONBytes))
	if err != nil {
		return err
	}

	template.ExtraExtensions = append(template.ExtraExtensions, pkix.Extension{
		Id:       CapabilitiesCertificateOID,
		Critical: false,
		Value:    capBytes,
	})
	return nil
}

func (ct *CertTool) readCA(opts CertToolGenerateOptions) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	return ct.readCAFiles(ct.caCertPath(opts), ct.caKeyPath(opts))
}

func (ct *CertTool) readCAFiles(certPath, keyPath string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	caCertPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, err
	}
	caKeyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}

	caCert, err := ct.parseCert(caCertPEM)
	if err != nil {
		return nil, nil, err
	}
	caKey, err := ct.parsePrivateKey(caKeyPEM)
	if err != nil {
		return nil, nil, err
	}

	return caCert, caKey, nil
}

func (ct *CertTool) parseCert(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("failed to decode certificate PEM")
	}
	if block.Type != "CERTIFICATE" {
		return nil, errors.Errorf("unexpected PEM type %q", block.Type)
	}

	return x509.ParseCertificate(block.Bytes)
}

func (ct *CertTool) parsePrivateKey(keyPEM []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("failed to decode private key PEM")
	}
	if block.Type != "EC PRIVATE KEY" {
		return nil, errors.Errorf("unexpected PEM type %q", block.Type)
	}

	return x509.ParseECPrivateKey(block.Bytes)
}

func (ct *CertTool) writePEMFile(path, pemType string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".crl-*")
	if err != nil {
		return err
	}
	defer func() {
		tmpname := tmpFile.Name()
		err := os.Remove(tmpname)
		if err != nil && !os.IsNotExist(err) {
			errors.Log(err, "failed to remove tmp file %q", tmpname)
		}
	}()

	err = tmpFile.Chmod(0o660)
	if err != nil {
		return err
	}
	err = pem.Encode(tmpFile, &pem.Block{
		Type:  pemType,
		Bytes: data,
	})
	if err != nil {
		return err
	}
	err = tmpFile.Close()
	if err != nil {
		return err
	}

	return os.Rename(tmpFile.Name(), path)

}

func (ct *CertTool) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (ct *CertTool) subjectKeyID(pub any) ([]byte, error) {
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(pubKeyBytes)
	// see: https://datatracker.ietf.org/doc/html/rfc7093#section-2
	return sum[sha1.Size:], nil
}

func (ct *CertTool) resolveRevocationSerial(opts CertToolRevokeOptions) (*big.Int, error) {
	if strings.TrimSpace(opts.CertPath) != "" {
		certPEM, err := os.ReadFile(opts.CertPath)
		if err != nil {
			return nil, err
		}
		cert, err := ct.parseCert(certPEM)
		if err != nil {
			return nil, err
		}
		return cert.SerialNumber, nil
	}

	serialText := strings.TrimSpace(opts.SerialNumber)
	if serialText == "" {
		return nil, errors.New("certificate path or serial number is required")
	}
	serial := new(big.Int)
	if _, ok := serial.SetString(serialText, 0); !ok {
		return nil, errors.Errorf("invalid serial number %q", serialText)
	}

	return serial, nil
}

func (ct *CertTool) readCRL(path string, caCert *x509.Certificate) (*x509.RevocationList, error) {
	crlPEM, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	rl, err := parseCRL(crlPEM)
	if err != nil {
		return nil, err
	}
	err = rl.CheckSignatureFrom(caCert)
	if err != nil {
		return nil, err
	}

	return rl, nil
}

func revokedEntriesFromList(rl *x509.RevocationList) []x509.RevocationListEntry {
	if len(rl.RevokedCertificateEntries) > 0 {
		return append([]x509.RevocationListEntry{}, rl.RevokedCertificateEntries...)
	}
	if len(rl.RevokedCertificateEntries) == 0 {
		return nil
	}
	entries := make([]x509.RevocationListEntry, 0, len(rl.RevokedCertificateEntries))
	for _, entry := range rl.RevokedCertificateEntries {
		entries = append(entries, x509.RevocationListEntry{
			SerialNumber:   entry.SerialNumber,
			RevocationTime: entry.RevocationTime,
			Extensions:     entry.Extensions,
		})
	}
	return entries
}

func revocationListHasSerial(entries []x509.RevocationListEntry, serial *big.Int) bool {
	for _, entry := range entries {
		if entry.SerialNumber != nil && entry.SerialNumber.Cmp(serial) == 0 {
			return true
		}
	}
	return false
}

func NewCertTool(registry *CertTypeRegistry) *CertTool {
	if registry == nil {
		registry = NewCertTypeRegistry()
	}
	return &CertTool{registry}
}
