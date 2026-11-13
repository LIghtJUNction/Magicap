package capture

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"github.com/LIghtJUNction/Magicap/internal/buildinfo"
	"github.com/elazarl/goproxy"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const BodyLimit = 1 << 20
const SessionLimit = 500

type Rule struct {
	Match           string            `json:"match"`
	Block           bool              `json:"block"`
	RequestHeaders  map[string]string `json:"request_headers,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	MockStatus      int               `json:"mock_status,omitempty"`
	MockBody        string            `json:"mock_body,omitempty"`
}
type Flow struct {
	ID                int64       `json:"id"`
	Started           time.Time   `json:"started"`
	DurationMS        int64       `json:"duration_ms"`
	Method            string      `json:"method"`
	URL               string      `json:"url"`
	Status            int         `json:"status"`
	RequestHeaders    http.Header `json:"request_headers"`
	ResponseHeaders   http.Header `json:"response_headers"`
	RequestBody       []byte      `json:"request_body"`
	ResponseBody      []byte      `json:"response_body"`
	RequestTruncated  bool        `json:"request_truncated"`
	ResponseTruncated bool        `json:"response_truncated"`
	Error             string      `json:"error,omitempty"`
}
type Store struct {
	mu    sync.RWMutex
	flows []Flow
	rules []Rule
	next  int64
	bytes int
}

func (s *Store) List(q string) []Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Flow{}
	for _, f := range s.flows {
		if strings.Contains(strings.ToLower(f.URL), strings.ToLower(q)) {
			out = append(out, f)
		}
	}
	return out
}
func (s *Store) Clear() { s.mu.Lock(); defer s.mu.Unlock(); s.flows = nil; s.bytes = 0 }
func (s *Store) Rules() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Rule{}, s.rules...)
}
func (s *Store) SetRules(r []Rule) { s.mu.Lock(); defer s.mu.Unlock(); s.rules = r }
func (s *Store) add(f Flow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	f.ID = s.next
	s.flows = append(s.flows, f)
	s.bytes += len(f.RequestBody) + len(f.ResponseBody)
	for len(s.flows) > SessionLimit || s.bytes > 32<<20 {
		s.bytes -= len(s.flows[0].RequestBody) + len(s.flows[0].ResponseBody)
		s.flows = s.flows[1:]
	}
}

type joinedBody struct {
	io.Reader
	io.Closer
}

func sample(body io.ReadCloser) ([]byte, io.ReadCloser, bool, error) {
	if body == nil {
		return nil, nil, false, nil
	}
	b, e := io.ReadAll(io.LimitReader(body, BodyLimit+1))
	if e != nil {
		return nil, body, false, e
	}
	n := len(b)
	if n > BodyLimit {
		n = BodyLimit
	}
	return b[:n], &joinedBody{io.MultiReader(bytes.NewReader(b), body), body}, len(b) > BodyLimit, nil
}

// responseRecorder samples as the client reads: streaming responses are not buffered
// before their headers are sent. A closed partial body is explicitly marked.
type responseRecorder struct {
	io.ReadCloser
	data           []byte
	truncated, eof bool
	once           sync.Once
	finish         func([]byte, bool)
}

func (b *responseRecorder) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	left := BodyLimit - len(b.data)
	take := n
	if take > left {
		take = left
		b.truncated = true
	}
	b.data = append(b.data, p[:take]...)
	if e == io.EOF {
		b.eof = true
		b.done()
	}
	return n, e
}
func (b *responseRecorder) done()        { b.once.Do(func() { b.finish(b.data, b.truncated || !b.eof) }) }
func (b *responseRecorder) Close() error { e := b.ReadCloser.Close(); b.done(); return e }

func NewProxy(ca *tls.Certificate, s *Store) *goproxy.ProxyHttpServer {
	p := goproxy.NewProxyHttpServer()
	p.Tr = &http.Transport{Proxy: nil, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
	p.OnRequest().HandleConnect(goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		return &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(ca)}, host
	}))
	p.OnRequest().DoFunc(func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		f := &Flow{Started: time.Now(), Method: r.Method, URL: r.URL.String(), RequestHeaders: r.Header.Clone()}
		ctx.UserData = f
		var err error
		f.RequestBody, r.Body, f.RequestTruncated, err = sample(r.Body)
		if err != nil {
			f.Error = err.Error()
		}
		for _, rule := range s.Rules() {
			if rule.Match == "" || !strings.Contains(r.URL.String(), rule.Match) {
				continue
			}
			for k, v := range rule.RequestHeaders {
				r.Header.Set(k, v)
			}
			if rule.Block {
				return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusForbidden, "Blocked by Magicap")
			}
			if rule.MockStatus >= 200 && rule.MockStatus <= 599 {
				return r, goproxy.NewResponse(r, goproxy.ContentTypeText, rule.MockStatus, rule.MockBody)
			}
		}
		return r, nil
	})
	p.OnResponse().DoFunc(func(r *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		f, ok := ctx.UserData.(*Flow)
		if !ok {
			return r
		}
		f.DurationMS = time.Since(f.Started).Milliseconds()
		if r != nil {
			for _, rule := range s.Rules() {
				if rule.Match != "" && strings.Contains(f.URL, rule.Match) {
					for k, v := range rule.ResponseHeaders {
						r.Header.Set(k, v)
					}
				}
			}
			f.Status = r.StatusCode
			f.ResponseHeaders = r.Header.Clone()

			if r.Body != nil {
				r.Body = &responseRecorder{ReadCloser: r.Body, finish: func(data []byte, truncated bool) {
					f.ResponseBody = data
					f.ResponseTruncated = truncated
					f.DurationMS = time.Since(f.Started).Milliseconds()
					s.add(*f)
				}}
				return r
			}
		} else if ctx.Error != nil {
			f.Error = ctx.Error.Error()
		}
		s.add(*f)
		return r
	})
	return p
}
func (s *Store) HAR() any {
	entries := []any{}
	for _, f := range s.List("") {
		headers := func(h http.Header) []any {
			a := []any{}
			for k, vs := range h {
				for _, v := range vs {
					a = append(a, map[string]any{"name": k, "value": v})
				}
			}
			return a
		}
		entries = append(entries, map[string]any{"startedDateTime": f.Started.Format(time.RFC3339Nano), "time": f.DurationMS, "request": map[string]any{"method": f.Method, "url": f.URL, "httpVersion": "HTTP/1.1", "headers": headers(f.RequestHeaders), "queryString": []any{}, "cookies": []any{}, "headersSize": -1, "bodySize": len(f.RequestBody), "postData": map[string]any{"mimeType": f.RequestHeaders.Get("Content-Type"), "text": string(f.RequestBody)}}, "response": map[string]any{"status": f.Status, "statusText": http.StatusText(f.Status), "httpVersion": "HTTP/1.1", "headers": headers(f.ResponseHeaders), "cookies": []any{}, "content": map[string]any{"size": len(f.ResponseBody), "mimeType": f.ResponseHeaders.Get("Content-Type"), "text": base64.StdEncoding.EncodeToString(f.ResponseBody), "encoding": "base64"}, "redirectURL": "", "headersSize": -1, "bodySize": len(f.ResponseBody)}, "cache": map[string]any{}, "timings": map[string]any{"send": 0, "wait": f.DurationMS, "receive": 0}, "_magicap": map[string]any{"requestTruncated": f.RequestTruncated, "responseTruncated": f.ResponseTruncated, "error": f.Error}})
	}
	return map[string]any{"log": map[string]any{"version": "1.2", "creator": map[string]string{"name": "Magicap", "version": buildinfo.Version}, "entries": entries}}
}
func JSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
