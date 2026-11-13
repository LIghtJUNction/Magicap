package capture

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestProxyHTTPSAndRules(t *testing.T) {
	ca, hash, e := EnsureCA(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if len(hash) != 10 {
		t.Fatal(hash)
	}
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Seen", r.Header.Get("X-Test"))
		_, _ = io.Copy(w, r.Body)
	}))
	defer upstream.Close()
	store := &Store{}
	store.SetRules([]Rule{{Match: upstream.URL, RequestHeaders: map[string]string{"X-Test": "rewritten"}, ResponseHeaders: map[string]string{"X-Response": "modified"}}})
	p := NewProxy(&ca, store)
	p.Tr = upstream.Client().Transport.(*http.Transport)
	proxy := httptest.NewServer(p)
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	pool := x509.NewCertPool()
	pool.AddCert(ca.Leaf)
	transport := &http.Transport{Proxy: http.ProxyURL(u), TLSClientConfig: &tls.Config{RootCAs: pool}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	resp, e := client.Post(upstream.URL+"/hello", "text/plain", bytes.NewBufferString("secret body"))
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(resp.Body)
	resp.Body.Close()
	if e != nil || string(b) != "secret body" {
		t.Fatalf("body %q err %v", b, e)
	}
	if resp.Header.Get("X-Seen") != "rewritten" || resp.Header.Get("X-Response") != "modified" {
		t.Fatal(resp.Header)
	}
	flows := store.List("hello")
	if len(flows) != 1 || flows[0].Status != 200 || string(flows[0].ResponseBody) != "secret body" {
		t.Fatalf("%+v", flows)
	}
	store.SetRules([]Rule{{Match: "/blocked", Block: true}, {Match: "/mock", MockStatus: 201, MockBody: "mocked"}})
	for path, status := range map[string]int{"/blocked": 403, "/mock": 201} {
		r, e := client.Get(upstream.URL + path)
		if e != nil {
			t.Fatal(e)
		}
		r.Body.Close()
		if r.StatusCode != status {
			t.Fatalf("%s: %d", path, r.StatusCode)
		}
	}
}
func TestSamplePreservesStreamAndCloses(t *testing.T) {
	data := bytes.Repeat([]byte("x"), BodyLimit+123)
	b, body, tr, e := sample(io.NopCloser(bytes.NewReader(data)))
	if e != nil || !tr || len(b) != BodyLimit {
		t.Fatal(len(b), tr, e)
	}
	all, e := io.ReadAll(body)
	body.Close()
	if e != nil || !bytes.Equal(all, data) {
		t.Fatal("capture altered upstream body")
	}
}
func TestCAStableAndIncompleteFails(t *testing.T) {
	d := t.TempDir()
	a, h, e := EnsureCA(d)
	if e != nil {
		t.Fatal(e)
	}
	b, h2, e := EnsureCA(d)
	if e != nil || h != h2 || !bytes.Equal(a.Certificate[0], b.Certificate[0]) {
		t.Fatal("identity changed", e)
	}
	if e = os.Remove(filepath.Join(d, "ca.pem")); e != nil {
		t.Fatal(e)
	}
	if _, _, e = EnsureCA(d); e == nil {
		t.Fatal("incomplete identity was silently replaced")
	}
}
func TestRetention(t *testing.T) {
	s := &Store{}
	for i := 0; i < SessionLimit+1; i++ {
		s.add(Flow{})
	}
	f := s.List("")
	if len(f) != SessionLimit || f[0].ID != 2 {
		t.Fatal(len(f), f[0].ID)
	}
	s.Clear()
	if len(s.List("")) != 0 {
		t.Fatal("clear failed")
	}
}
