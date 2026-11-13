package capture

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestTransparentHTTPS(t *testing.T) {
	ca, _, err := EnsureCA(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Forwarded-Test", r.Header.Get("X-Client-Test"))
		_, _ = io.WriteString(w, "transparent plaintext")
	}))
	defer upstream.Close()
	host := upstream.Certificate().DNSNames[0]
	for _, http2 := range []bool{false, true} {
		name := "http1"
		if http2 {
			name = "http2"
		}
		t.Run(name, func(t *testing.T) {
			store := &Store{}
			proxy := NewProxy(&ca, store)
			upstreamTransport := upstream.Client().Transport.(*http.Transport).Clone()
			defer upstreamTransport.CloseIdleConnections()
			upstreamTransport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
			}
			proxy.Tr = upstreamTransport
			server := httptest.NewUnstartedServer(OriginProxy(proxy))
			server.EnableHTTP2 = true
			server.TLS = TransparentTLSConfig(&ca, proxy)
			server.StartTLS()
			defer server.Close()
			pool := x509.NewCertPool()
			pool.AddCert(ca.Leaf)
			transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}, ForceAttemptHTTP2: http2, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport}
			req, _ := http.NewRequest("GET", "https://"+host+"/transparent", nil)
			req.Header.Set("X-Client-Test", "visible")
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || string(body) != "transparent plaintext" || response.Header.Get("X-Forwarded-Test") != "visible" {
				t.Fatalf("status=%d body=%q err=%v", response.StatusCode, body, err)
			}
			if http2 && response.ProtoMajor != 2 {
				t.Fatal("HTTP/2 not negotiated", response.Proto)
			}
			flows := store.List("/transparent")
			if len(flows) != 1 || flows[0].URL != "https://"+host+"/transparent" {
				t.Fatalf("%+v", flows)
			}
		})
	}
}

func TestTransparentHTTP(t *testing.T) {
	ca, _, err := EnsureCA(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "plain http") }))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	store := &Store{}
	proxy := httptest.NewServer(OriginProxy(NewProxy(&ca, store)))
	defer proxy.Close()
	req, _ := http.NewRequest("GET", proxy.URL+"/direct", nil)
	req.Host = target.Host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "plain http" {
		t.Fatal(string(body))
	}
	if len(store.List("/direct")) != 1 {
		t.Fatal("missing captured HTTP request")
	}
}
