package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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
	SerialFile = "serial"
)

type CertType struct {
	KeyFile  string
	CertFile string
}

type CertTypeRegistry struct {
	types map[string]CertType
	mu    sync.RWMutex
}

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

type CertToolOptions struct {
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

type CertTool struct {
	*CertTypeRegistry
}

func NewCertTool(registry *CertTypeRegistry) *CertTool {
	if registry == nil {
		registry = NewCertTypeRegistry()
	}
	return &CertTool{registry}
}

// Generate creates certificates based on options. Caller is responsible for synchronization.
func (ct *CertTool) Generate(opts CertToolOptions) error {
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

	if err := ct.generateCerts(opts, certType); err != nil {
		return errors.Errorf("error generating certificates: %w", err)
	}

	return nil
}

func (ct *CertTool) namespace(opts CertToolOptions, fileName string) string {
	if opts.NamePrefix != "" {
		return opts.NamePrefix + "." + fileName
	}
	return fileName
}

func (ct *CertTool) certFileName(opts CertToolOptions, fileName string) string {
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

func (ct *CertTool) caKeyPath(opts CertToolOptions) string {
	if opts.CAKeyPath != "" {
		return opts.CAKeyPath
	}
	return ct.namespace(opts, CAKeyFile)
}

func (ct *CertTool) caCertPath(opts CertToolOptions) string {
	if opts.CACertPath != "" {
		return opts.CACertPath
	}
	return ct.namespace(opts, CACertFile)
}

func (ct *CertTool) loadSerial(opts CertToolOptions) (*big.Int, error) {
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

func (ct *CertTool) saveSerial(opts CertToolOptions, serial *big.Int) error {
	return os.WriteFile(ct.namespace(opts, SerialFile), []byte(serial.String()), 0o660)
}

func (ct *CertTool) generateCerts(opts CertToolOptions, certType CertType) error {
	if !ct.fileExists(ct.caKeyPath(opts)) {
		if err := ct.generateCA(opts); err != nil {
			return errors.Errorf("generating CA: %w", err)
		}
	}

	serial, err := ct.loadSerial(opts)
	if err != nil {
		return errors.Errorf("error loading serial: %w", err)
	}
	defer func() {
		if err := ct.saveSerial(opts, serial); err != nil {
			fmt.Printf("error saving serial: %v\n", err)
		}
	}()

	caCert, caKey, err := ct.readCA(opts)
	if err != nil {
		return errors.Errorf("reading CA: %w", err)
	}

	return ct.generateCert(opts, certType, serial, caCert, caKey)
}

func (ct *CertTool) generateCA(opts CertToolOptions) error {
	serial, err := ct.loadSerial(opts)
	if err != nil {
		return errors.Errorf("error loading serial: %w", err)
	}
	defer func() {
		if err := ct.saveSerial(opts, serial); err != nil {
			fmt.Printf("error saving serial: %v\n", err)
		}
	}()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
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
	}
	ct.applyCountry(template, opts.Country)

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}

	if err := ct.writePEMFile(ct.caCertPath(opts), "CERTIFICATE", certBytes); err != nil {
		return err
	}

	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}

	return ct.writePEMFile(ct.caKeyPath(opts), "EC PRIVATE KEY", keyBytes)
}

func (ct *CertTool) generateCert(opts CertToolOptions, certType CertType, serial *big.Int, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) error {
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
	if err := ct.applyCapabilities(template, opts.Capabilities); err != nil {
		return err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return err
	}

	if err := ct.writePEMFile(ct.certFileName(opts, certType.CertFile), "CERTIFICATE", certBytes); err != nil {
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

func (ct *CertTool) readCA(opts CertToolOptions) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	caCertPEM, err := os.ReadFile(ct.caCertPath(opts))
	if err != nil {
		return nil, nil, err
	}
	caKeyPEM, err := os.ReadFile(ct.caKeyPath(opts))
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
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer errors.LogCallErr(file.Close, "failed to close file %q", path)

	return pem.Encode(file, &pem.Block{
		Type:  pemType,
		Bytes: data,
	})
}

func (ct *CertTool) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
