package main

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"strconv"

	plugin "aroz.org/zoraxy/route-debugger/mod/zoraxy_plugin"
)

/*
	Route Debugger — a dynamic-capture plugin for Zoraxy.

	Define capture rules (host + path patterns). When a proxied request matches:
	  - intercept mode : the request is answered with a full debug dump of what
	                     Zoraxy forwarded (headers, cookies, query, body) instead
	                     of being proxied upstream.
	  - tap mode       : the request metadata is logged to the dashboard and the
	                     request still proxies upstream normally (non-intrusive).

	Every capture is recorded in an in-memory log reviewable from the admin UI.
*/

const (
	PLUGIN_ID = "org.aroz.zoraxy.route-debugger"
	UI_PATH   = "/"
	WEB_ROOT  = "/www"

	DYNAMIC_CAPTURE_SNIFF   = "/d_sniff"
	DYNAMIC_CAPTURE_INGRESS = "/d_capture"
)

//go:embed www/*
var adminFS embed.FS

// App holds shared singletons used across handlers.
type App struct {
	cfg      *ConfigManager
	store    *CaptureStore
	capturer *Capturer
}

// global app (single plugin instance)
var app *App

func main() {
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            PLUGIN_ID,
		Name:          "Route Debugger",
		Author:        "aroz.org",
		AuthorContact: "noreply@aroz.org",
		Description:   "Capture and inspect proxied requests on matching host/path patterns. Intercept requests to return a debug dump, or passively tap them while they proxy upstream.",
		URL:           "https://github.com/aroz-online/route-debugger",
		Type:          plugin.PluginType_Router,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,

		DynamicCaptureSniff:   DYNAMIC_CAPTURE_SNIFF,
		DynamicCaptureIngress: DYNAMIC_CAPTURE_INGRESS,

		UIPath: UI_PATH,
	})
	if err != nil {
		panic(err)
	}

	// --- Core wiring ---
	cfg := LoadConfig()
	var limit int
	cfg.Read(func(c *Config) { limit = c.LogLimit })
	store := NewCaptureStore(limit)
	capturer := NewCapturer(cfg, store)
	app = &App{cfg: cfg, store: store, capturer: capturer}

	mux := http.NewServeMux()

	// --- Dynamic capture endpoints ---
	pathRouter := plugin.NewPathRouter()
	pathRouter.RegisterDynamicSniffHandler(DYNAMIC_CAPTURE_SNIFF, mux, capturer.HandleSniff)
	pathRouter.RegisterDynamicCaptureHandle(DYNAMIC_CAPTURE_INGRESS, mux, capturer.HandleCapture)

	// --- Admin UI (proxied via Zoraxy) ---
	embedWebRouter := plugin.NewPluginEmbedUIRouter(PLUGIN_ID, &adminFS, WEB_ROOT, UI_PATH)
	// Point at the on-disk ./www folder only when it exists (source-tree dev run);
	// otherwise fall back to the embedded FS so a standalone binary still serves UI.
	if info, err := os.Stat("./www"); err == nil && info.IsDir() {
		embedWebRouter.SetDevWebRoot("./www")
	}
	embedWebRouter.RegisterTerminateHandler(func() {
		fmt.Println("[route-debugger] terminating...")
	}, mux)

	registerAdminAPI(embedWebRouter, mux)

	// Catch-all: serve the admin UI
	mux.Handle(UI_PATH, embedWebRouter.Handler())

	addr := "127.0.0.1:" + strconv.Itoa(runtimeCfg.Port)
	fmt.Println("[route-debugger] admin UI on http://" + addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

// registerAdminAPI mounts all configuration endpoints on the admin mux.
func registerAdminAPI(router *plugin.PluginUiRouter, mux *http.ServeMux) {
	h := func(path string, fn http.HandlerFunc) {
		router.HandleFunc(path, fn, mux)
	}
	// rules
	h("/api/rules", handleRuleList)
	h("/api/rule/save", handleRuleSave)
	h("/api/rule/delete", handleRuleDelete)
	h("/api/rule/toggle", handleRuleToggle)
	// captures
	h("/api/captures", handleCaptureList)
	h("/api/capture/detail", handleCaptureDetail)
	h("/api/captures/clear", handleCaptureClear)
	// settings
	h("/api/settings/get", handleSettingsGet)
	h("/api/settings/save", handleSettingsSave)
}
