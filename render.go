package main

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"
)

/*
	render.go

	Renders a Capture into the debug page returned to the client in intercept
	mode — either a styled HTML report or plain text that works well with curl.
*/

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fmtTime(unixMs int64) string {
	return time.UnixMilli(unixMs).UTC().Format("2006-01-02 15:04:05 UTC")
}

// ── Plain-text renderer ───────────────────────────────────────────────────────

func renderCapturePlain(c *Capture) string {
	var sb strings.Builder
	kw := 26

	line := func(k, v string) {
		sb.WriteString(fmt.Sprintf("  %-*s %s\n", kw, k+":", v))
	}
	section := func(title string) {
		sb.WriteString("\n" + title + "\n" + strings.Repeat("-", len(title)) + "\n")
	}

	sb.WriteString("Zoraxy Route Debugger\n")
	sb.WriteString(fmt.Sprintf("Captured: %s\n", fmtTime(c.Time)))
	sb.WriteString(fmt.Sprintf("Matched rule: %s\n", c.RuleName))

	section("REQUEST")
	line("Method", c.Method)
	line("Host", c.Host)
	line("Request-URI", c.RequestURI)
	line("Path", c.Path)
	line("Protocol", c.Proto)
	line("Remote Address", c.RemoteAddr)
	line("Content-Length", fmt.Sprintf("%d", c.ContentLength))

	section("HEADERS")
	if len(c.Headers) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, k := range sortedKeys(c.Headers) {
			for _, v := range c.Headers[k] {
				line(k, v)
			}
		}
	}

	section("COOKIES")
	if len(c.Cookies) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, k := range sortedStringKeys(c.Cookies) {
			line(k, c.Cookies[k])
		}
	}

	section("QUERY PARAMETERS")
	if len(c.Query) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, k := range sortedKeys(c.Query) {
			for _, v := range c.Query[k] {
				line(k, v)
			}
		}
	}

	bodyLabel := fmt.Sprintf("BODY (%d bytes)", c.BodySize)
	if c.BodyTruncated {
		bodyLabel = "BODY (truncated preview)"
	}
	section(bodyLabel)
	if c.BodyPreview == "" {
		sb.WriteString("  (empty)\n")
	} else {
		for _, ln := range strings.Split(c.BodyPreview, "\n") {
			sb.WriteString("  " + ln + "\n")
		}
	}

	return sb.String()
}

// ── HTML renderer  ─────────────────────────────────────────

const htmlHead = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Route Debugger — Capture</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;font-size:13px;background:#1b1d21;color:#e7e9ec;padding:32px 40px;line-height:1.55;max-width:1000px;margin:0 auto}
h1{font-size:17px;font-weight:700;color:#fff;margin-bottom:2px}
.meta{font-size:11px;color:#9aa0a8;margin-bottom:26px}
.meta b{color:#cfd3d9;font-weight:600}
.section{margin-bottom:26px}
.section-title{font-size:11px;font-weight:700;color:#9aa0a8;text-transform:uppercase;letter-spacing:.07em;margin-bottom:8px;padding-bottom:6px;border-bottom:1px solid #33373d}
.reqline{font-family:'SF Mono',Menlo,Consolas,monospace;font-size:13px;color:#fff;background:#25282d;border:1px solid #33373d;border-radius:7px;padding:11px 14px;word-break:break-all}
.method{font-weight:700;margin-right:10px;color:#fff}
.proto{color:#9aa0a8;margin-left:10px}
table{border-collapse:collapse;width:100%}
tr:last-child td{border-bottom:none}
td{padding:6px 0;border-bottom:1px solid #2a2e33;vertical-align:top;word-break:break-all}
td.k{color:#cfd3d9;font-weight:600;white-space:nowrap;width:34%;padding-right:16px}
td.v{color:#aeb4bc;font-family:'SF Mono',Menlo,Consolas,monospace;font-size:12px}
td.empty{color:#6b7078;font-style:italic}
pre{font-family:'SF Mono',Menlo,Consolas,monospace;font-size:12px;background:#25282d;border:1px solid #33373d;border-radius:7px;padding:12px;overflow:auto;white-space:pre-wrap;word-break:break-all;color:#e7e9ec}
.note{font-size:11px;color:#6b7078;margin-bottom:6px}
</style>
</head>
<body>`

func renderCaptureHTML(c *Capture) string {
	e := html.EscapeString
	var b strings.Builder
	b.WriteString(htmlHead)

	b.WriteString(`<h1>Zoraxy Route Debugger</h1>`)
	b.WriteString(fmt.Sprintf(`<div class="meta">Captured <b>%s</b> &middot; matched rule <b>%s</b> &middot; mode <b>intercept</b></div>`,
		e(fmtTime(c.Time)), e(c.RuleName)))

	// Request line
	b.WriteString(`<div class="section"><div class="section-title">Request Line</div>`)
	b.WriteString(fmt.Sprintf(`<div class="reqline"><span class="method">%s</span>%s<span class="proto">%s</span></div></div>`,
		e(c.Method), e(c.RequestURI), e(c.Proto)))

	// Request details
	b.WriteString(`<div class="section"><div class="section-title">Request Details</div><table>`)
	b.WriteString(row("Host", c.Host))
	b.WriteString(row("Path", c.Path))
	b.WriteString(row("Remote Address", c.RemoteAddr))
	b.WriteString(row("Content-Length", fmt.Sprintf("%d", c.ContentLength)))
	b.WriteString(`</table></div>`)

	// Headers
	b.WriteString(`<div class="section"><div class="section-title">Headers</div><table>`)
	b.WriteString(multiRows(c.Headers, "No headers"))
	b.WriteString(`</table></div>`)

	// Cookies
	b.WriteString(`<div class="section"><div class="section-title">Cookies</div><table>`)
	b.WriteString(singleRows(c.Cookies, "No cookies"))
	b.WriteString(`</table></div>`)

	// Query
	b.WriteString(`<div class="section"><div class="section-title">Query Parameters</div><table>`)
	b.WriteString(multiRows(c.Query, "No query parameters"))
	b.WriteString(`</table></div>`)

	// Body
	b.WriteString(`<div class="section"><div class="section-title">Request Body</div>`)
	bodyNote := fmt.Sprintf("%d bytes", c.BodySize)
	if c.BodyTruncated {
		bodyNote = "truncated preview"
	}
	b.WriteString(fmt.Sprintf(`<div class="note">%s</div>`, e(bodyNote)))
	if c.BodyPreview == "" {
		b.WriteString(`<div class="note">Empty body</div>`)
	} else {
		b.WriteString(fmt.Sprintf(`<pre>%s</pre>`, e(c.BodyPreview)))
	}
	b.WriteString(`</div>`)

	b.WriteString(`</body></html>`)
	return b.String()
}

func row(k, v string) string {
	return fmt.Sprintf(`<tr><td class="k">%s</td><td class="v">%s</td></tr>`,
		html.EscapeString(k), html.EscapeString(v))
}

func multiRows(m map[string][]string, empty string) string {
	if len(m) == 0 {
		return fmt.Sprintf(`<tr><td colspan="2" class="empty">%s</td></tr>`, html.EscapeString(empty))
	}
	var b strings.Builder
	for _, k := range sortedKeys(m) {
		for _, v := range m[k] {
			b.WriteString(row(k, v))
		}
	}
	return b.String()
}

func singleRows(m map[string]string, empty string) string {
	if len(m) == 0 {
		return fmt.Sprintf(`<tr><td colspan="2" class="empty">%s</td></tr>`, html.EscapeString(empty))
	}
	var b strings.Builder
	for _, k := range sortedStringKeys(m) {
		b.WriteString(row(k, m[k]))
	}
	return b.String()
}
