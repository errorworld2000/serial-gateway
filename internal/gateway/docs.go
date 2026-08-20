package gateway

import (
	"fmt"
	"html/template"
	"net/http"
)

type apiDocsData struct {
	BaseURL string
}

var apiDocsTemplate = template.Must(template.New("api-docs").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Serial Gateway API</title>
  <style>
    :root{color-scheme:dark;--bg:#0b0b0b;--panel:#141414;--line:#343434;--text:#e8e8e8;--muted:#aaa;--accent:#e02932;--code:#090909}
    *{box-sizing:border-box} body{margin:0;background:var(--bg);color:var(--text);font:15px/1.55 "Segoe UI",Arial,sans-serif}
    main{max-width:1040px;margin:auto;padding:36px 24px 72px} h1{font-size:34px;margin:0 0 8px} h2{margin-top:36px;border-bottom:1px solid var(--line);padding-bottom:8px}
    h3{margin:24px 0 8px} p,li{color:var(--muted)} a{color:#ff7379} code,pre{font-family:"Cascadia Mono",Consolas,monospace}
    code{color:#fff} pre{background:var(--code);border:1px solid var(--line);padding:14px;overflow:auto;white-space:pre-wrap}
    .hero,.card{background:var(--panel);border:1px solid var(--line);padding:20px}.links{display:flex;gap:10px;flex-wrap:wrap;margin-top:18px}
    .links a{border:1px solid var(--line);padding:8px 12px;text-decoration:none}.method{display:inline-block;min-width:54px;color:#fff;font-weight:700}.get{color:#70d98b}.post{color:#ffb45d}.put{color:#75b8ff}
    table{width:100%;border-collapse:collapse} th,td{text-align:left;padding:9px;border-bottom:1px solid var(--line);vertical-align:top} th{color:#fff}
    .warning{border-left:3px solid var(--accent);padding-left:14px}
  </style>
</head>
<body><main>
  <section class="hero">
    <h1>Serial Gateway API</h1>
    <p>Local HTTP API for AI agents, scripts, and serial-console automation. Base URL: <code>{{.BaseURL}}</code></p>
    <div class="links"><a href="/api/v1/guide">AI guide (Markdown)</a><a href="/api/v1/openapi.json">OpenAPI 3.1 JSON</a><a href="/api/v1/ports">Live ports</a><a href="/">Web terminal</a></div>
  </section>

  <h2>Recommended AI workflow</h2>
  <ol>
    <li>Call <code>GET /api/v1/ports</code> and choose a connected port.</li>
    <li>Remember its <code>latest_sequence</code> as your cursor.</li>
    <li>Send a command with <code>POST /api/v1/write?port=COM13</code>. Serial consoles normally need a trailing carriage return (<code>\r</code>).</li>
    <li>Read with <code>GET /api/v1/read?port=COM13&amp;after=CURSOR&amp;encoding=text&amp;wait_ms=30000</code>.</li>
    <li>Advance the cursor to <code>next_after</code>; repeat immediately while <code>has_more</code> is true.</li>
  </ol>
  <pre>curl.exe {{.BaseURL}}/api/v1/ports

curl.exe -X POST "{{.BaseURL}}/api/v1/write?port=COM13" ^
  -H "Content-Type: application/json" ^
  -d "{\"encoding\":\"text\",\"data\":\"uname -a\\r\"}"

curl.exe "{{.BaseURL}}/api/v1/read?port=COM13&amp;after=0&amp;encoding=text&amp;wait_ms=30000"</pre>

  <h2>Endpoints</h2>
  <table><thead><tr><th>Method</th><th>Path</th><th>Purpose</th></tr></thead><tbody>
    <tr><td class="method get">GET</td><td><code>/healthz</code></td><td>Service health and connected-port count.</td></tr>
    <tr><td class="method get">GET</td><td><code>/api/v1/info</code></td><td>Build and default serial settings.</td></tr>
    <tr><td class="method get">GET</td><td><code>/api/v1/ports</code></td><td>Port status, cursors, Raw TCP and Telnet addresses.</td></tr>
    <tr><td class="method get">GET</td><td><code>/api/v1/read</code></td><td>Cursor-based RX/TX history with optional long polling.</td></tr>
    <tr><td class="method post">POST</td><td><code>/api/v1/write</code></td><td>Write text, hex, base64, or a raw body to a serial port.</td></tr>
    <tr><td class="method get">GET</td><td><code>/api/v1/config</code></td><td>Read runtime configuration and detected ports.</td></tr>
    <tr><td class="method put">PUT</td><td><code>/api/v1/config</code></td><td>Validate and persist the complete configuration.</td></tr>
  </tbody></table>

  <h2>Read semantics</h2>
  <p>Every RX and TX record has a monotonically increasing <code>sequence</code>. The gateway retains roughly 1 MiB of history per port. Use <code>history_truncated</code> to detect an expired cursor. A read can return both your transmitted command (<code>tx</code>) and device output (<code>rx</code>).</p>
  <p>For command completion, print unique markers such as <code>__AI_BEGIN__</code> and <code>__AI_END__</code>. Serial streams have no inherent request/response boundary.</p>

  <h2>Transport selection</h2>
  <table><thead><tr><th>Consumer</th><th>Transport</th></tr></thead><tbody>
    <tr><td>AI and automation</td><td>HTTP API on this page</td></tr>
    <tr><td>SecureCRT interactive terminal</td><td>Telnet address reported by <code>/api/v1/ports</code></td></tr>
    <tr><td>Binary/socket tooling</td><td>Raw TCP address reported by <code>/api/v1/ports</code></td></tr>
  </tbody></table>
  <p class="warning">The service is intended for localhost use and currently has no authentication or TLS. Multiple clients share the same serial input; coordinate writers to avoid interleaved commands.</p>
</main></body></html>`))

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (a *App) HandleAPIDocs(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = apiDocsTemplate.Execute(w, apiDocsData{BaseURL: requestBaseURL(r)})
}

func (a *App) HandleAIGuide(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	base := requestBaseURL(r)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = fmt.Fprintf(w, `# Serial Gateway AI Guide

Base URL: %s
OpenAPI: %s/api/v1/openapi.json

## Workflow

1. GET /api/v1/ports and select a port whose connected field is true.
2. Save that port's latest_sequence as the cursor.
3. POST /api/v1/write?port=COM13 with JSON {"encoding":"text","data":"command\r"}.
4. GET /api/v1/read?port=COM13&after=CURSOR&encoding=text&wait_ms=30000.
5. Set the cursor to next_after. If has_more is true, read again immediately.

Records have direction rx or tx. The per-port history is about 1 MiB. history_truncated means the requested cursor expired. Supported encodings are text, hex, and base64. A non-JSON write body is sent as raw bytes.

For shell automation, use unique output markers because a serial stream has no request boundary:

    echo __AI_BEGIN__; uname -a; echo __AI_END__\r

Do not assume COM13: always discover ports first. Multiple Web, Telnet, Raw, and API clients share the serial port. Avoid simultaneous writers. This localhost service has no authentication or TLS.
`, base, base)
}

const openAPISpec = `{
  "openapi": "3.1.0",
  "info": {"title": "Serial Gateway API", "version": "1.0.0", "description": "Cursor-based local serial debugging API for AI agents and scripts."},
  "servers": [{"url": "/"}],
  "paths": {
    "/healthz": {"get": {"summary": "Health check", "responses": {"200": {"description": "Healthy"}}}},
    "/api/v1/info": {"get": {"summary": "Build and serial defaults", "responses": {"200": {"description": "Service information"}}}},
    "/api/v1/ports": {"get": {"summary": "Discover serial ports and transport addresses", "responses": {"200": {"description": "Port status list"}}}},
    "/api/v1/read": {"get": {"summary": "Read cursor-based serial history", "parameters": [
      {"name":"port","in":"query","required":true,"schema":{"type":"string"}},
      {"name":"after","in":"query","schema":{"type":"integer","minimum":0,"default":0}},
      {"name":"limit","in":"query","schema":{"type":"integer","minimum":1,"maximum":1000,"default":256}},
      {"name":"wait_ms","in":"query","schema":{"type":"integer","minimum":0,"maximum":30000,"default":0}},
      {"name":"encoding","in":"query","schema":{"type":"string","enum":["text","hex","base64"],"default":"base64"}}
    ], "responses": {"200":{"description":"History records"},"404":{"description":"Unknown port"}}}},
    "/api/v1/write": {"post": {"summary": "Write bytes to a serial port", "parameters": [
      {"name":"port","in":"query","required":true,"schema":{"type":"string"}}
    ], "requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/WriteRequest"}},"application/octet-stream":{"schema":{"type":"string","format":"binary"}}}}, "responses":{"200":{"description":"Write accepted"},"404":{"description":"Unknown port"},"503":{"description":"Port disconnected"}}}},
    "/api/v1/config": {
      "get":{"summary":"Read configuration and detected ports","responses":{"200":{"description":"Configuration"}}},
      "put":{"summary":"Validate and persist complete configuration","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object"}}}},"responses":{"200":{"description":"Saved"},"400":{"description":"Invalid configuration"}}}
    },
    "/api/v1/guide": {"get":{"summary":"AI-readable Markdown guide","responses":{"200":{"description":"Markdown documentation"}}}},
    "/api/v1/openapi.json": {"get":{"summary":"This OpenAPI document","responses":{"200":{"description":"OpenAPI 3.1 schema"}}}}
  },
  "components": {"schemas": {
    "WriteRequest": {"type":"object","required":["data"],"properties":{"encoding":{"type":"string","enum":["text","hex","base64"],"default":"text"},"data":{"type":"string"}}}
  }}
}`

func (a *App) HandleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	w.Header().Set("Content-Type", "application/vnd.oai.openapi+json;version=3.1; charset=utf-8")
	_, _ = w.Write([]byte(openAPISpec))
}
