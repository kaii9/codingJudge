package rpcsecurity_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type testAuthority struct {
	cert   *x509.Certificate
	key    *ecdsa.PrivateKey
	file   string
	roots  *x509.CertPool
	dir    string
	serial int64
}

type testIdentity struct {
	certFile string
	keyFile  string
}

func newTestAuthority(t *testing.T) *testAuthority {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-only-ca"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	file := filepath.Join(dir, "ca.pem")
	writeFixture(t, file, data)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return &testAuthority{cert: cert, key: key, file: file, roots: roots, dir: dir, serial: 1}
}

func (a *testAuthority) issue(t *testing.T, name string, usage x509.ExtKeyUsage, expired bool) testIdentity {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a.serial++
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(a.serial), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
	}
	if expired {
		template.NotBefore, template.NotAfter = now.Add(-2*time.Hour), now.Add(-time.Hour)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, a.cert, &key.PublicKey, a.key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	identity := testIdentity{certFile: filepath.Join(a.dir, name+".pem"), keyFile: filepath.Join(a.dir, name+".key")}
	writeFixture(t, identity.certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	writeFixture(t, identity.keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return identity
}

func writeFixture(t *testing.T, file string, data []byte) {
	t.Helper()
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (i testIdentity) env(ca, serverName string) func(string) string {
	values := map[string]string{
		"RPC_TLS_CERT_FILE": i.certFile, "RPC_TLS_KEY_FILE": i.keyFile,
		"RPC_TLS_CA_FILE": ca, "RPC_TLS_SERVER_NAME": serverName,
	}
	return func(key string) string { return values[key] }
}

func (i testIdentity) certificate(t *testing.T) tls.Certificate {
	t.Helper()
	cert, err := tls.LoadX509KeyPair(i.certFile, i.keyFile)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
