package auth

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertTypeRegistry(t *testing.T) {
	var (
		registry = NewCertTypeRegistry()
		server   = CertType{KeyFile: "server-key.pem", CertFile: "server-cert.pem"}
	)
	require.NoError(t, registry.Register("server", server))
	assert.Error(t, registry.Register("server", CertType{KeyFile: "other-key.pem", CertFile: "other-cert.pem"}))
	assert.Error(t, registry.Register("client", CertType{KeyFile: "client-key.pem"}))

	certType, err := registry.Lookup("server")
	require.NoError(t, err)
	assert.Equal(t, server, certType)

	_, err = registry.Lookup("client")
	assert.Error(t, err)
}

func TestCertTypeRegistryRegistersANameOnce(t *testing.T) {
	var (
		registry = NewCertTypeRegistry()
		wg       sync.WaitGroup
		accepted atomic.Int32
	)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := registry.Register("server", CertType{KeyFile: "server-key.pem", CertFile: "server-cert.pem"})
			if err == nil {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), accepted.Load())
}
