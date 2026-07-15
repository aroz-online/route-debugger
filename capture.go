package main

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	plugin "aroz.org/zoraxy/route-debugger/mod/zoraxy_plugin"
)

/*
	capture.go

	Pattern matching + the dynamic sniff / capture handlers that turn a matching
	proxied request into a logged Capture (and, in intercept mode, a debug page
	returned to the client).
*/

// Capturer wires the config, the capture log and the pending-request map used to
// correlate the sniff phase (metadata) with the capture phase (body).
type Capturer struct {
	cfg   *ConfigManager
	store *CaptureStore

	mu      sync.Mutex
	pending map[string]*Capture // keyed by Zoraxy request UUID
}

func NewCapturer(cfg *ConfigManager, store *CaptureStore) *Capturer {
	return &Capturer{
		cfg:     cfg,
		store:   store,
		pending: make(map[string]*Capture),
	}
}

// HandleSniff is the dynamic capture sniff endpoint. Zoraxy asks, for every
// proxied request, whether this plugin wants to capture it.
func (c *Capturer) HandleSniff(dsfr *plugin.DynamicSniffForwardRequest) plugin.SniffResult {
	path, rawQuery := splitPathQuery(dsfr.RequestURI)
	target := dsfr.Host + path

	rule := c.cfg.MatchRule(target)
	if rule == nil {
		return plugin.SniffResultSkip
	}

	capture := &Capture{
		ID:            newID(),
		Time:          time.Now().UnixMilli(),
		RuleID:        rule.ID,
		RuleName:      rule.Name,
		Mode:          rule.Mode,
		Method:        dsfr.Method,
		Host:          dsfr.Host,
		Path:          path,
		RequestURI:    dsfr.RequestURI,
		Proto:         dsfr.Proto,
		RemoteAddr:    dsfr.RemoteAddr,
		Headers:       dsfr.Header,
		Query:         parseQuery(rawQuery),
		Cookies:       parseCookies(dsfr.Header),
		ContentLength: headerContentLength(dsfr.Header),
	}

	if rule.Mode == ModeTap {
		// Passive tap: log the metadata now and let the request continue upstream.
		capture.BodyCaptured = false
		c.store.Add(capture)
		return plugin.SniffResultSkip
	}

	// Intercept: stash the metadata until the body arrives at the capture ingress.
	uuid := dsfr.GetRequestUUID()
	c.mu.Lock()
	c.pending[uuid] = capture
	c.mu.Unlock()
	return plugin.SniffResultAccept
}

// HandleCapture is the dynamic capture ingress. Only intercepted (accepted)
// requests reach here; the full request body is available. We finalize the
// capture, store it, and render the debug dump back to the client.
func (c *Capturer) HandleCapture(w http.ResponseWriter, r *http.Request) {
	uuid := r.Header.Get("X-Zoraxy-RequestID")

	c.mu.Lock()
	capture := c.pending[uuid]
	delete(c.pending, uuid)
	c.mu.Unlock()

	// Fallback: build from the raw request if the sniff record went missing.
	if capture == nil {
		path, rawQuery := splitPathQuery(r.RequestURI)
		capture = &Capture{
			ID:            newID(),
			Time:          time.Now().UnixMilli(),
			RuleName:      "(unmatched)",
			Mode:          ModeIntercept,
			Method:        r.Method,
			Host:          r.Host,
			Path:          path,
			RequestURI:    r.RequestURI,
			Proto:         r.Proto,
			RemoteAddr:    r.RemoteAddr,
			Headers:       r.Header,
			Query:         parseQuery(rawQuery),
			Cookies:       parseCookies(r.Header),
			ContentLength: r.ContentLength,
		}
	}

	// Read the body preview.
	var previewSize int
	var pretty bool
	c.cfg.Read(func(cfg *Config) { previewSize = cfg.BodyPreviewSize })
	if rule := c.cfg.matchRuleByID(capture.RuleID); rule != nil {
		pretty = rule.PrettyPrint
	}
	if r.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(r.Body, int64(previewSize)+1))
		if err == nil {
			if len(raw) > previewSize {
				raw = raw[:previewSize]
				capture.BodyTruncated = true
				capture.BodySize = -1
			} else {
				capture.BodySize = len(raw)
			}
			capture.BodyPreview = string(raw)
		}
	}
	capture.BodyCaptured = true

	c.store.Add(capture)

	// Respond to the client with the debug dump.
	if pretty {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(renderCaptureHTML(capture)))
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(renderCapturePlain(capture)))
	}
}

// matchRuleByID looks up a rule by ID (read-locked).
func (cm *ConfigManager) matchRuleByID(id string) *CaptureRule {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	for _, r := range cm.cfg.Rules {
		if r.ID == id {
			return r
		}
	}
	return nil
}

/*
	Pattern matching
*/

// matchPattern reports whether target matches a glob pattern anchored at the
// start. * matches any run of characters, ? matches a single character.
func matchPattern(pattern, target string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	re, err := compileGlob(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(target)
}

var globCache sync.Map // pattern string -> *regexp.Regexp

func compileGlob(pattern string) (*regexp.Regexp, error) {
	if v, ok := globCache.Load(pattern); ok {
		return v.(*regexp.Regexp), nil
	}
	var sb strings.Builder
	sb.WriteString("^")
	for _, ch := range pattern {
		switch ch {
		case '*':
			sb.WriteString(".*")
		case '?':
			sb.WriteString(".")
		default:
			sb.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	re, err := regexp.Compile(sb.String())
	if err != nil {
		return nil, err
	}
	globCache.Store(pattern, re)
	return re, nil
}

/*
	Small parsing helpers
*/

// splitPathQuery splits a raw request URI into its path and raw query parts.
func splitPathQuery(requestURI string) (path string, rawQuery string) {
	if i := strings.IndexByte(requestURI, '?'); i >= 0 {
		return requestURI[:i], requestURI[i+1:]
	}
	return requestURI, ""
}

func parseQuery(rawQuery string) map[string][]string {
	if rawQuery == "" {
		return map[string][]string{}
	}
	v, err := url.ParseQuery(rawQuery)
	if err != nil {
		return map[string][]string{}
	}
	return v
}

// parseCookies extracts cookie name/value pairs from the Cookie header.
func parseCookies(header map[string][]string) map[string]string {
	out := map[string]string{}
	var raw []string
	for k, v := range header {
		if strings.EqualFold(k, "Cookie") {
			raw = v
			break
		}
	}
	for _, line := range raw {
		for _, part := range strings.Split(line, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if eq := strings.IndexByte(part, '='); eq >= 0 {
				out[part[:eq]] = part[eq+1:]
			} else {
				out[part] = ""
			}
		}
	}
	return out
}

func headerContentLength(header map[string][]string) int64 {
	for k, v := range header {
		if strings.EqualFold(k, "Content-Length") && len(v) > 0 {
			if n, err := strconv.ParseInt(v[0], 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}
