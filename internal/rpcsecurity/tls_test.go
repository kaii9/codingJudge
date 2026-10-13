package rpcsecurity

import (
	"testing"
)

func TestPartialTLSConfigurationFailsClosed(t *testing.T) {
	values := map[string]string{"RPC_TLS_CERT_FILE": "/missing/cert.pem"}
	if _, err := ServerOptions(func(k string) string { return values[k] }); err == nil {
		t.Fatal("partial TLS config allowed plaintext fallback")
	}
	if _, err := ClientOptions(func(k string) string { return values[k] }); err == nil {
		t.Fatal("partial client TLS config allowed plaintext fallback")
	}
}
func TestLocalExplicitNoTLSConfiguration(t *testing.T) {
	options, err := ServerOptions(func(string) string { return "" })
	if err != nil || len(options) != 0 {
		t.Fatalf("local server options=%v err=%v", options, err)
	}
}
