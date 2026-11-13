package capture

import (
	"crypto/tls"
	"errors"
	"net/http"
	"sync"

	"github.com/elazarl/goproxy"
)

// TransparentTLSConfig terminates redirected TLS using the requested SNI and
// the same per-device CA as the explicit CONNECT proxy.
func TransparentTLSConfig(ca *tls.Certificate, proxy *goproxy.ProxyHttpServer) *tls.Config {
	var cache sync.Map
	return &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"h2", "http/1.1"}, GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		host := hello.ServerName
		if host == "" {
			return nil, errors.New("transparent TLS connection has no SNI")
		}
		if cached, ok := cache.Load(host); ok {
			return cached.(*tls.Config), nil
		}
		config, err := goproxy.TLSConfigFromCA(ca)(host, &goproxy.ProxyCtx{Proxy: proxy})
		if err != nil {
			return nil, err
		}
		config.MinVersion = tls.VersionTLS12
		config.NextProtos = []string{"h2", "http/1.1"}
		cached, _ := cache.LoadOrStore(host, config)
		return cached.(*tls.Config), nil
	}}
}

// OriginProxy normalizes origin-form requests from transparent HTTP/TLS
// listeners before passing them to the ordinary capture pipeline.
func OriginProxy(proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect && r.URL.Host == "" {
			if r.Host == "" {
				http.Error(w, "missing request host", http.StatusBadRequest)
				return
			}
			r.URL.Host = r.Host
			r.URL.Scheme = "http"
			if r.TLS != nil {
				r.URL.Scheme = "https"
			}
		}
		proxy.ServeHTTP(w, r)
	})
}
