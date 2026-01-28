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
		panic(err)
	}
	defer os.RemoveAll(dir)

	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}

	if err := tool.Generate(CertToolGenerateOptions{
		GenerateCA: true,
		CommonName: "atlas-ca",
	}); err != nil {
		panic(err)
	}

	if err := tool.Generate(CertToolGenerateOptions{
		Type:        "server",
		CommonName:  "localhost",
		IPAddresses: "127.0.0.1",
		DNSNames:    "localhost",
	}); err != nil {
		panic(err)
	}

	if err := tool.Revoke(CertToolRevokeOptions{
		CertPath: "server-cert.pem",
	}); err != nil {
		panic(err)
	}
}
