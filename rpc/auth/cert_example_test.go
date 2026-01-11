package auth

import "os"

func ExampleCertTool() {
	registry := NewCertTypeRegistry()
	_ = registry.Register("server", CertType{
		KeyFile:  "server-key.pem",
		CertFile: "server-cert.pem",
	})

	tool := NewCertTool(registry)

	dir, err := os.MkdirTemp("", "auth-cert-example-*")
	if err != nil {
		return
	}
	defer os.RemoveAll(dir)

	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	if err := os.Chdir(dir); err != nil {
		return
	}

	if err := tool.Generate(Options{
		Type:        "server",
		CommonName:  "localhost",
		IPAddresses: "127.0.0.1",
		DNSNames:    "localhost",
	}); err != nil {
		return
	}
}
