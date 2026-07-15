package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

/*
	api.go

	REST handlers for the configuration UI (served through Zoraxy's authenticated
	admin panel). All endpoints are mounted under the plugin UI path.
*/

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeOK(w http.ResponseWriter) {
	writeJSON(w, map[string]bool{"ok": true})
}

// --- Capture rules ---

func handleRuleList(w http.ResponseWriter, r *http.Request) {
	var rules []*CaptureRule
	app.cfg.Read(func(c *Config) {
		rules = append(rules, c.Rules...)
	})
	if rules == nil {
		rules = []*CaptureRule{}
	}
	writeJSON(w, rules)
}

// handleRuleSave adds a new rule (empty id) or updates an existing one.
func handleRuleSave(w http.ResponseWriter, r *http.Request) {
	var in CaptureRule
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, "invalid payload: "+err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Pattern = strings.TrimSpace(in.Pattern)
	if in.Pattern == "" {
		writeErr(w, "pattern is required")
		return
	}
	if in.Name == "" {
		in.Name = in.Pattern
	}
	if in.Mode != ModeIntercept && in.Mode != ModeTap {
		in.Mode = ModeIntercept
	}

	err := app.cfg.Update(func(c *Config) {
		if in.ID == "" {
			in.ID = newID()
			c.Rules = append(c.Rules, &in)
			return
		}
		for i, existing := range c.Rules {
			if existing.ID == in.ID {
				c.Rules[i] = &in
				return
			}
		}
		// ID supplied but not found: treat as new
		c.Rules = append(c.Rules, &in)
	})
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	writeJSON(w, in)
}

func handleRuleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	if id == "" {
		writeErr(w, "missing id")
		return
	}
	err := app.cfg.Update(func(c *Config) {
		out := c.Rules[:0]
		for _, rule := range c.Rules {
			if rule.ID != id {
				out = append(out, rule)
			}
		}
		c.Rules = out
	})
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	writeOK(w)
}

func handleRuleToggle(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	if id == "" {
		writeErr(w, "missing id")
		return
	}
	err := app.cfg.Update(func(c *Config) {
		for _, rule := range c.Rules {
			if rule.ID == id {
				rule.Enabled = !rule.Enabled
				return
			}
		}
	})
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	writeOK(w)
}

// --- Capture log ---

func handleCaptureList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, app.store.List())
}

func handleCaptureDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	c := app.store.Get(id)
	if c == nil {
		writeErr(w, "capture not found")
		return
	}
	writeJSON(w, c)
}

func handleCaptureClear(w http.ResponseWriter, r *http.Request) {
	app.store.Clear()
	writeOK(w)
}

// --- Settings ---

func handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	var out struct {
		LogLimit        int `json:"log_limit"`
		BodyPreviewSize int `json:"body_preview_size"`
	}
	app.cfg.Read(func(c *Config) {
		out.LogLimit = c.LogLimit
		out.BodyPreviewSize = c.BodyPreviewSize
	})
	writeJSON(w, out)
}

func handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	logLimit, _ := strconv.Atoi(r.FormValue("log_limit"))
	bodyPreview, _ := strconv.Atoi(r.FormValue("body_preview_size"))
	if logLimit <= 0 {
		logLimit = DefaultLogLimit
	}
	if bodyPreview <= 0 {
		bodyPreview = DefaultBodyPreviewSize
	}
	err := app.cfg.Update(func(c *Config) {
		c.LogLimit = logLimit
		c.BodyPreviewSize = bodyPreview
	})
	if err != nil {
		writeErr(w, err.Error())
		return
	}
	app.store.SetLimit(logLimit)
	writeOK(w)
}
