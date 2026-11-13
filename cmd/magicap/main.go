package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/LIghtJUNction/Magicap/internal/buildinfo"
	"github.com/LIghtJUNction/Magicap/internal/capture"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	Capture         bool   `json:"capture"`
	MCP             bool   `json:"mcp"`
	ProxyPort       int    `json:"proxy_port"`
	WebPort         int    `json:"web_port"`
	MCPPort         int    `json:"mcp_port"`
	Secret          string `json:"secret,omitempty"`
	TransparentPort int    `json:"transparent_port"`
	CaptureUIDs     []int  `json:"capture_uids,omitempty"`
}

var root, state string

func die(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func config() (Config, error) {
	c := Config{ProxyPort: 8080, WebPort: 8081, MCPPort: 8765, TransparentPort: 8443}
	b, e := os.ReadFile(filepath.Join(state, "config.json"))
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
func save(c Config) error {
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	p := filepath.Join(state, "config.json")
	if e = os.WriteFile(p+".tmp", b, 0600); e != nil {
		return e
	}
	return os.Rename(p+".tmp", p)
}
func endpoint(c Config) string { return fmt.Sprintf("http://127.0.0.1:%d", c.WebPort) }
func running(c Config) bool {
	client := http.Client{Timeout: time.Second}
	r, e := client.Get(endpoint(c) + "/health")
	if e != nil {
		return false
	}
	defer r.Body.Close()
	return r.StatusCode == 200 && r.Header.Get("X-Magicap") == buildinfo.Version
}
func stop(c Config) error {
	if len(c.CaptureUIDs) > 0 {
		if err := routes(c, false); err != nil {
			return err
		}
	}
	b, e := os.ReadFile(filepath.Join(state, "pid"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	pid, e := strconv.Atoi(strings.TrimSpace(string(b)))
	if e != nil {
		return e
	}
	if !running(c) {
		return errors.New("daemon health unavailable; refusing to signal unverified PID")
	}
	client := http.Client{Timeout: 3 * time.Second}
	req, _ := http.NewRequest("POST", endpoint(c)+"/api/shutdown", nil)
	authorize(req, c)
	r, e := client.Do(req)
	if e != nil {
		return e
	}
	r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("shutdown: %s", r.Status)
	}
	for i := 0; i < 50; i++ {
		if _, e = os.Stat(filepath.Join(state, "pid")); os.IsNotExist(e) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("process %d did not stop", pid)
}
func start(c Config) error {
	if running(c) {
		return routes(c, true)
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	log, e := os.OpenFile(filepath.Join(state, "daemon.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if e != nil {
		return e
	}
	defer log.Close()
	cmd := exec.Command(exe, "daemon")
	cmd.Env = os.Environ()
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = cmd.Start(); e != nil {
		return e
	}
	_ = cmd.Process.Release()
	for i := 0; i < 100; i++ {
		if running(c) {
			return routes(c, true)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("daemon failed to start; inspect " + filepath.Join(state, "daemon.log"))
}
func authorize(r *http.Request, c Config) {
	if c.Secret != "" {
		r.Header.Set("Authorization", "Bearer "+c.Secret)
	}
}
func api(c Config, method, path string, body io.Reader) error {
	req, e := http.NewRequest(method, endpoint(c)+path, body)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	authorize(req, c)
	client := http.Client{Timeout: 45 * time.Second}
	r, e := client.Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 4096))
		return fmt.Errorf("%s: %s", r.Status, b)
	}
	_, e = io.Copy(os.Stdout, r.Body)
	return e
}
func main() {
	exe, e := os.Executable()
	die(e)
	root = filepath.Dir(filepath.Dir(exe))
	if v := os.Getenv("MAGICAP_ROOT"); v != "" {
		root = v
	}
	state = os.Getenv("MAGICAP_STATE")
	if state == "" {
		state = "/data/adb/magicap"
	}
	die(os.MkdirAll(state, 0700))
	c, e := config()
	die(e)
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("Magicap: status | start | stop | capture enable|disable | mcp enable|disable|status [--json] [--port N] | config [--proxy-port N] [--web-port N] [--secret KEY] | cert init|export|overlay | sessions [filter] | clear | har | replay JSON | rules [JSON] | app enable|disable PACKAGE | app status | cert trust|untrust | pcap [--interface any] [--seconds 60] [--filter EXPR] | boot")
		return
	}
	switch args[0] {
	case "app":
		die(appCommand(c, args[1:]))
	case "pcap":
		die(pcap(args[1:]))
	case "daemon":
		die(serve(c))
	case "start":
		die(start(c))
	case "stop":
		die(stop(c))
	case "boot":
		if c.Capture || c.MCP {
			die(start(c))
		}
	case "status":
		b, _ := json.Marshal(map[string]any{"running": running(c), "capture": c.Capture, "mcp": c.MCP, "proxy_port": c.ProxyPort, "web_port": c.WebPort, "mcp_port": c.MCPPort, "authenticated": c.Secret != "", "transparent_port": c.TransparentPort, "capture_uids": c.CaptureUIDs})
		fmt.Println(string(b))
	case "cert":
		_, hash, e := capture.EnsureCA(filepath.Join(state, "certs"))
		die(e)
		sub := "init"
		if len(args) > 1 {
			sub = args[1]
		}
		switch sub {
		case "trust", "untrust":
			if sub == "untrust" && len(c.CaptureUIDs) > 0 {
				if running(c) {
					die(stop(c))
				}
				c.Capture = false
				die(save(c))
				if c.MCP {
					die(start(c))
				}
			}
			action := "enable"
			if sub == "untrust" {
				action = "disable"
			}
			die(helper("trust-ca.sh", action))
		case "init":
			fmt.Println(filepath.Join(state, "certs", "ca.pem"))
		case "export":
			b, e := os.ReadFile(filepath.Join(state, "certs", "ca.pem"))
			die(e)
			_, e = os.Stdout.Write(b)
			die(e)
		case "overlay":
			d := filepath.Join(root, "system/etc/security/cacerts")
			die(os.MkdirAll(d, 0755))
			b, e := os.ReadFile(filepath.Join(state, "certs", "ca.pem"))
			die(e)
			die(os.WriteFile(filepath.Join(d, hash), b, 0644))
			fmt.Println(hash)
		default:
			die(errors.New("unknown cert command"))
		}
	case "capture", "mcp":
		if len(args) < 2 {
			die(errors.New("expected enable, disable or status"))
		}
		isMCP := args[0] == "mcp"
		if args[1] == "status" {
			out := map[string]any{"enabled": c.MCP, "running": c.MCP && running(c), "pid": 0, "host": "127.0.0.1", "port": c.MCPPort, "endpoint": "/mcp", "transport": "streamable-http"}
			if b, e := os.ReadFile(filepath.Join(state, "pid")); e == nil && running(c) {
				out["pid"], _ = strconv.Atoi(strings.TrimSpace(string(b)))
			}
			if !isMCP {
				out["enabled"] = c.Capture
				out["running"] = c.Capture && running(c)
				out["port"] = c.ProxyPort
				out["endpoint"] = ""
				out["transport"] = "http-proxy"
			}
			if len(args) < 3 || args[2] != "--json" {
				fmt.Printf("enabled=%v running=%v pid=%v host=%v port=%v endpoint=%v transport=%v\n", out["enabled"], out["running"], out["pid"], out["host"], out["port"], out["endpoint"], out["transport"])
				return
			}
			b, _ := json.Marshal(out)
			fmt.Println(string(b))
			return
		}
		old := c
		switch args[1] {
		case "enable":
			if isMCP {
				c.MCP = true
			} else {
				c.Capture = true
			}
		case "disable":
			if isMCP {
				c.MCP = false
			} else {
				c.Capture = false
			}
		default:
			die(errors.New("unknown action"))
		}
		for i := 2; i < len(args); i++ {
			if args[i] != "--port" || i+1 >= len(args) {
				die(errors.New("expected --port N"))
			}
			i++
			p, e := port(args[i])
			die(e)
			if isMCP {
				c.MCPPort = p
			} else {
				c.ProxyPort = p
			}
		}
		die(valid(c))
		if args[1] == "enable" && c.Capture == old.Capture && c.MCP == old.MCP && c.ProxyPort == old.ProxyPort && c.MCPPort == old.MCPPort {
			die(start(c))
			fmt.Println("updated")
			return
		}
		if running(old) {
			die(stop(old))
		}
		die(save(c))
		if c.Capture || c.MCP {
			die(start(c))
		}
		fmt.Println("updated")
	case "config":
		old := c
		for i := 1; i < len(args); i += 2 {
			if i+1 >= len(args) {
				die(errors.New("missing value"))
			}
			if args[i] == "--secret" {
				c.Secret = args[i+1]
				continue
			}
			p, e := port(args[i+1])
			die(e)
			switch args[i] {
			case "--proxy-port":
				c.ProxyPort = p
			case "--web-port":
				c.WebPort = p
			case "--mcp-port":
				c.MCPPort = p
			case "--transparent-port":
				c.TransparentPort = p
			default:
				die(errors.New("unknown option"))
			}
		}
		die(valid(c))
		if running(old) {
			die(stop(old))
		}
		die(save(c))
		if c.Capture || c.MCP {
			die(start(c))
		}
	case "sessions":
		q := ""
		if len(args) > 1 {
			q = args[1]
		}
		die(api(c, "GET", "/api/sessions?q="+url.QueryEscape(q), nil))
	case "clear":
		die(api(c, "DELETE", "/api/sessions", nil))
	case "har":
		die(api(c, "GET", "/api/har", nil))
	case "rules":
		method := "GET"
		var body io.Reader
		if len(args) > 1 {
			method = "PUT"
			body = strings.NewReader(args[1])
		}
		die(api(c, method, "/api/rules", body))
	case "replay":
		if len(args) != 2 {
			die(errors.New("replay requires JSON {method,url,headers,body}"))
		}
		die(api(c, "POST", "/api/replay", strings.NewReader(args[1])))
	default:
		die(errors.New("unknown command"))
	}
}
func port(s string) (int, error) {
	n, e := strconv.Atoi(s)
	if e != nil || n < 1024 || n > 65535 {
		return 0, errors.New("port must be 1024..65535")
	}
	return n, nil
}
func valid(c Config) error {
	seen := map[int]bool{}
	for _, p := range []int{c.ProxyPort, c.WebPort, c.MCPPort, c.TransparentPort} {
		if p < 1024 || p > 65535 {
			return errors.New("port must be 1024..65535")
		}
		if seen[p] {
			return errors.New("ports must differ")
		}
		seen[p] = true
	}
	return nil
}

type Replay struct {
	Method  string            `json:"method,omitempty"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

func replay(ctx context.Context, c Config, in Replay) (any, error) {
	u, e := url.Parse(in.URL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("expected absolute http(s) URL")
	}
	if len(in.Body) > capture.BodyLimit {
		return nil, errors.New("body too large")
	}
	method := in.Method
	if method == "" {
		method = "GET"
	}
	r, e := http.NewRequestWithContext(ctx, method, in.URL, strings.NewReader(in.Body))
	if e != nil {
		return nil, e
	}
	for k, v := range in.Headers {
		r.Header.Set(k, v)
	}
	ca, _, e := capture.EnsureCA(filepath.Join(state, "certs"))
	if e != nil {
		return nil, e
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.Leaf)
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", c.ProxyPort))
	tr := &http.Transport{Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{RootCAs: pool}}
	defer tr.CloseIdleConnections()
	client := http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(r)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, capture.BodyLimit+1))
	if e != nil {
		return nil, e
	}
	truncated := len(b) > capture.BodyLimit
	if truncated {
		b = b[:capture.BodyLimit]
	}
	return map[string]any{"status": resp.StatusCode, "headers": resp.Header, "body": b, "truncated": truncated}, nil
}
func serve(c Config) error {
	lock, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", c.WebPort))
	if e != nil {
		return e
	}
	defer lock.Close()
	ca, _, e := capture.EnsureCA(filepath.Join(state, "certs"))
	if e != nil {
		return e
	}
	store := &capture.Store{}
	rulesPath := filepath.Join(state, "rules.json")
	if b, e := os.ReadFile(rulesPath); e == nil {
		var rules []capture.Rule
		if e = json.Unmarshal(b, &rules); e != nil {
			return e
		}
		store.SetRules(rules)
	} else if !os.IsNotExist(e) {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if c.Capture && len(c.CaptureUIDs) > 0 {
		go watchAppTrust(ctx, c.CaptureUIDs)
	}
	servers := []*http.Server{}
	errCh := make(chan error, 3)
	launch := func(listener net.Listener, h http.Handler) {
		protocols := &http.Protocols{}
		protocols.SetHTTP1(true)
		protocols.SetHTTP2(true)
		srv := &http.Server{Protocols: protocols, Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
		servers = append(servers, srv)
		go func() {
			if e := srv.Serve(listener); e != nil && !errors.Is(e, http.ErrServerClosed) {
				errCh <- e
			}
		}()
	}
	listen := func(p int) (net.Listener, error) { return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)) }
	ownsPID := false
	defer func() {
		for _, server := range servers {
			_ = server.Close()
		}
		if len(c.CaptureUIDs) > 0 {
			if err := routes(c, false); err != nil {
				fmt.Fprintln(os.Stderr, "route cleanup:", err)
			}
		}
		if ownsPID {
			_ = os.Remove(filepath.Join(state, "pid"))
		}
	}()
	if c.Capture {
		l, e := listen(c.ProxyPort)
		if e != nil {
			return e
		}
		proxy := capture.NewProxy(&ca, store)
		handler := capture.OriginProxy(proxy)
		launch(l, handler)
		if len(c.CaptureUIDs) > 0 {
			for _, host := range []string{"127.0.0.1", "::1"} {
				raw, e := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(c.TransparentPort)))
				if e != nil {
					return e
				}
				launch(tls.NewListener(raw, capture.TransparentTLSConfig(&ca, proxy)), handler)
			}
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Magicap", buildinfo.Version)
		capture.JSON(w, map[string]any{"capture": c.Capture, "mcp": c.MCP, "proxy_port": c.ProxyPort, "web_port": c.WebPort, "mcp_port": c.MCPPort})
	})
	mux.HandleFunc("POST /api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		capture.JSON(w, map[string]bool{"ok": true})
		go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	})
	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) { capture.JSON(w, store.List(r.URL.Query().Get("q"))) })
	mux.HandleFunc("DELETE /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		store.Clear()
		capture.JSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/har", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=magicap.har")
		capture.JSON(w, store.HAR())
	})
	mux.HandleFunc("GET /api/rules", func(w http.ResponseWriter, r *http.Request) { capture.JSON(w, store.Rules()) })
	mux.HandleFunc("PUT /api/rules", func(w http.ResponseWriter, r *http.Request) {
		var rules []capture.Rule
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&rules); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		if e := updateRules(store, rulesPath, rules); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		capture.JSON(w, rules)
	})
	mux.HandleFunc("POST /api/replay", func(w http.ResponseWriter, r *http.Request) {
		var in Replay
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&in); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		out, e := replay(r.Context(), c, in)
		if e != nil {
			http.Error(w, e.Error(), 502)
			return
		}
		capture.JSON(w, out)
	})
	mux.HandleFunc("GET /api/ca", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename=magicap-ca.pem")
		http.ServeFile(w, r, filepath.Join(state, "certs", "ca.pem"))
	})
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(root, "webroot"))))
	if c.MCP {
		server := mcp.NewServer(&mcp.Implementation{Name: "magicap", Version: buildinfo.Version}, nil)
		type Query struct {
			Filter string `json:"filter,omitempty"`
		}
		mcp.AddTool(server, &mcp.Tool{Name: "sessions_list", Description: "List captured HTTP(S) sessions; body fields are base64, limited to 1 MiB"}, func(ctx context.Context, r *mcp.CallToolRequest, in Query) (*mcp.CallToolResult, any, error) {
			return nil, store.List(in.Filter), nil
		})
		mcp.AddTool(server, &mcp.Tool{Name: "sessions_clear", Description: "Clear in-memory captured sessions"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			store.Clear()
			return nil, map[string]bool{"ok": true}, nil
		})
		mcp.AddTool(server, &mcp.Tool{Name: "har_export", Description: "Export captured sessions as HAR 1.2"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			return nil, store.HAR(), nil
		})
		mcp.AddTool(server, &mcp.Tool{Name: "request_replay", Description: "Send an HTTP(S) request through the enabled capture proxy"}, func(ctx context.Context, r *mcp.CallToolRequest, in Replay) (*mcp.CallToolResult, any, error) {
			out, e := replay(ctx, c, in)
			return nil, out, e
		})
		mcp.AddTool(server, &mcp.Tool{Name: "rules_list", Description: "Read capture rewrite, block and mock rules"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			return nil, store.Rules(), nil
		})

		type RulesInput struct {
			Rules []capture.Rule `json:"rules"`
		}
		mcp.AddTool(server, &mcp.Tool{Name: "rules_set", Description: "Replace persistent block, rewrite and mock rules (maximum 100)"}, func(ctx context.Context, r *mcp.CallToolRequest, in RulesInput) (*mcp.CallToolResult, any, error) {
			e := updateRules(store, rulesPath, in.Rules)
			return nil, map[string]bool{"ok": e == nil}, e
		})
		mcp.AddTool(server, &mcp.Tool{Name: "capture_status", Description: "Read capture and MCP runtime settings (no secrets)"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			return nil, map[string]any{"capture": c.Capture, "mcp": c.MCP, "proxy_port": c.ProxyPort, "mcp_port": c.MCPPort, "web_port": c.WebPort}, nil
		})
		l, e := listen(c.MCPPort)
		if e != nil {
			return e
		}
		m := http.NewServeMux()
		m.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true}))
		launch(l, guard(c, m))
	}
	dieWrite := os.WriteFile(filepath.Join(state, "pid"), []byte(strconv.Itoa(os.Getpid())), 0600)
	if dieWrite != nil {
		return dieWrite
	}
	ownsPID = true
	launch(lock, guard(c, mux))
	select {
	case <-ctx.Done():
		return nil
	case e := <-errCh:
		return e
	}
}
func guard(c Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, e := net.SplitHostPort(r.Host)
		if e != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "invalid Host", 403)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
				http.Error(w, "invalid Origin", 403)
				return
			}
		}
		if (strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/mcp") && c.Secret != "" && r.Header.Get("Authorization") != "Bearer "+c.Secret {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// pcap delegates packet decoding/filtering to tcpdump, without shell evaluation.
func pcap(args []string) error {
	iface, filter, seconds := "any", "", 60
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) {
			return errors.New("pcap expects --interface NAME, --filter EXPR, --seconds N")
		}
		switch args[i] {
		case "--interface":
			iface = args[i+1]
		case "--filter":
			filter = args[i+1]
		case "--seconds":
			n, e := strconv.Atoi(args[i+1])
			if e != nil || n < 1 || n > 3600 {
				return errors.New("seconds must be 1..3600")
			}
			seconds = n
		default:
			return errors.New("unknown pcap option")
		}
	}
	binary, e := exec.LookPath("tcpdump")
	if e != nil {
		return errors.New("PCAP requires tcpdump in PATH (not bundled)")
	}
	dir := filepath.Join(state, "captures")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	path := filepath.Join(dir, fmt.Sprintf("capture-%d.pcap", time.Now().UnixNano()))
	argv := []string{"-i", iface, "-U", "-s", "0", "-c", "100000", "-w", path}
	if filter != "" {
		argv = append(argv, "--", filter)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer timeout()
	cmd := exec.CommandContext(ctx, binary, argv...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGINT) }
	cmd.WaitDelay = 3 * time.Second
	if e = cmd.Run(); e != nil && ctx.Err() == nil {
		return e
	}
	fmt.Println(path)
	return nil
}

var rulesMu sync.Mutex

func updateRules(store *capture.Store, path string, rules []capture.Rule) error {
	rulesMu.Lock()
	defer rulesMu.Unlock()
	if len(rules) > 100 {
		return errors.New("maximum 100 rules")
	}
	for _, rule := range rules {
		if rule.Match == "" || rule.MockStatus != 0 && (rule.MockStatus < 200 || rule.MockStatus > 599) {
			return errors.New("invalid match/status")
		}
	}
	b, e := json.Marshal(rules)
	if e != nil {
		return e
	}
	if len(b) > 1<<20 {
		return errors.New("rules exceed 1 MiB")
	}
	if e = os.WriteFile(path+".tmp", b, 0600); e != nil {
		return e
	}
	if e = os.Rename(path+".tmp", path); e != nil {
		return e
	}
	store.SetRules(rules)
	return nil
}

func helper(name string, args ...string) error {
	command := exec.Command("/system/bin/sh", append([]string{filepath.Join(root, name)}, args...)...)
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	return command.Run()
}
func routes(c Config, enable bool) error {
	if len(c.CaptureUIDs) == 0 {
		return nil
	}
	if !enable || !c.Capture {
		return helper("app-routes.sh", "clear")
	}
	args := []string{"apply", strconv.Itoa(c.TransparentPort), strconv.Itoa(c.ProxyPort)}
	for _, uid := range c.CaptureUIDs {
		args = append(args, strconv.Itoa(uid))
	}
	return helper("app-routes.sh", args...)
}
func appCommand(c Config, args []string) error {
	if len(args) == 1 && args[0] == "status" {
		b, _ := json.Marshal(map[string]any{"uids": c.CaptureUIDs, "port": c.TransparentPort, "capture": c.Capture})
		fmt.Println(string(b))
		return nil
	}
	if len(args) != 2 || (args[0] != "enable" && args[0] != "disable") {
		return errors.New("usage: cli app enable|disable PACKAGE | status")
	}
	pkg := args[1]
	out, e := exec.Command("/system/bin/cmd", "package", "list", "packages", "-U", pkg).Output()
	if e != nil {
		return e
	}
	uid := 0
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "package:"+pkg {
			uid, _ = strconv.Atoi(strings.TrimPrefix(fields[1], "uid:"))
		}
	}
	if uid < 10000 {
		return errors.New("application UID was not found")
	}
	old := c
	exists := false
	for _, existing := range c.CaptureUIDs {
		if existing == uid {
			exists = true
		}
	}
	if args[0] == "enable" && exists && c.Capture {
		if e = helper("trust-ca.sh", "enable"); e != nil {
			return e
		}
		return start(c)
	}
	if args[0] == "disable" && !exists {
		return nil
	}
	next := []int{}
	for _, existing := range c.CaptureUIDs {
		if existing != uid {
			next = append(next, existing)
		}
	}
	if args[0] == "enable" {
		next = append(next, uid)
		c.Capture = true
		if e = helper("trust-ca.sh", "enable"); e != nil {
			return e
		}
	}
	c.CaptureUIDs = next
	if running(old) {
		if e = stop(old); e != nil {
			return e
		}
	}
	if e = save(c); e != nil {
		return e
	}
	if c.Capture || c.MCP {
		return start(c)
	}
	return nil
}
