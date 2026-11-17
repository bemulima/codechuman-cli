package main

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteCARejectsInvalidBundle(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ca.pem")
	if e := os.WriteFile(p, []byte("invalid"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("CODECHUMAN_CA_FILE", p)
	if _, e := configuredRemoteClient(); e == nil {
		t.Fatal("invalid CA accepted")
	}
}

func TestRemoteCAValidatesServerAndPreservesDefaults(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "ca.pem")
	if e := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("CODECHUMAN_CA_FILE", path)
	client, e := configuredRemoteClient()
	if e != nil {
		t.Fatal(e)
	}
	response, e := client.Get(server.URL)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatal(response.StatusCode)
	}
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer other.Close()
	// httptest shares a fixed certificate; an unrelated hostname must still fail.
	transport := client.Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = "unrelated.invalid"
	if _, e = (&http.Client{Transport: transport}).Get(server.URL); e == nil {
		t.Fatal("hostname verification bypassed")
	}
	t.Setenv("CODECHUMAN_CA_FILE", "")
	client, e = configuredRemoteClient()
	if e != nil || client != nil {
		t.Fatal("default trust changed")
	}
}
