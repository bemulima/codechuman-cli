package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"os"
	"time"
)

// CODECHUMAN_CA_FILE is explicit process-local trust, including on macOS where
// SSL_CERT_FILE does not configure Go's native certificate verifier.
func configuredRemoteClient() (*http.Client, error) {
	path := os.Getenv("CODECHUMAN_CA_FILE")
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot read configured CA bundle")
	}
	defer file.Close()
	pem, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(pem) > 1<<20 {
		return nil, errors.New("invalid configured CA bundle")
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("invalid configured CA bundle")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	return &http.Client{Transport: transport, Timeout: 12 * time.Second}, nil
}
