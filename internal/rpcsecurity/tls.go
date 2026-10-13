package rpcsecurity

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"os"
)

func config(getenv func(string) string) (*tls.Config, error) {
	cert, key, ca := getenv("RPC_TLS_CERT_FILE"), getenv("RPC_TLS_KEY_FILE"), getenv("RPC_TLS_CA_FILE")
	if cert == "" && key == "" && ca == "" {
		return nil, nil
	}
	if cert == "" || key == "" || ca == "" {
		return nil, fmt.Errorf("RPC TLS requires certificate, key and CA together")
	}
	identity, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(ca)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("invalid RPC CA")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{identity}, RootCAs: roots, ClientCAs: roots, ServerName: getenv("RPC_TLS_SERVER_NAME")}, nil
}
func ClientOptions(getenv func(string) string) ([]grpc.DialOption, error) {
	cfg, err := config(getenv)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	return []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(cfg))}, nil
}
func ServerOptions(getenv func(string) string) ([]grpc.ServerOption, error) {
	cfg, err := config(getenv)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	cfg.ClientAuth = tls.RequireAndVerifyClientCert
	return []grpc.ServerOption{grpc.Creds(credentials.NewTLS(cfg))}, nil
}
