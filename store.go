package main

import (
	"sync"
)

/*
	store.go

	In-memory ring buffer holding the most recent captured requests so the admin
	dashboard can review them. Captures are intentionally not persisted to disk —
	they may contain sensitive headers/bodies and are only meant for live
	debugging sessions.
*/

// Capture is a single recorded request.
type Capture struct {
	ID         string              `json:"id"`
	Time       int64               `json:"time"` // unix milliseconds
	RuleID     string              `json:"rule_id"`
	RuleName   string              `json:"rule_name"`
	Mode       CaptureMode         `json:"mode"`
	Method     string              `json:"method"`
	Host       string              `json:"host"`
	Path       string              `json:"path"`
	RequestURI string              `json:"request_uri"`
	Proto      string              `json:"proto"`
	RemoteAddr string              `json:"remote_addr"`
	Headers    map[string][]string `json:"headers"`
	Query      map[string][]string `json:"query"`
	Cookies    map[string]string   `json:"cookies"`

	ContentLength int64  `json:"content_length"`
	BodyPreview   string `json:"body_preview"`
	BodySize      int    `json:"body_size"`
	BodyTruncated bool   `json:"body_truncated"`
	BodyCaptured  bool   `json:"body_captured"` // false in tap mode (metadata only)
}

// CaptureStore is a fixed-capacity, newest-first ring of captures.
type CaptureStore struct {
	mu    sync.RWMutex
	items []*Capture // newest at index 0
	limit int
}

// NewCaptureStore creates a store retaining up to limit captures.
func NewCaptureStore(limit int) *CaptureStore {
	if limit <= 0 {
		limit = DefaultLogLimit
	}
	return &CaptureStore{
		items: make([]*Capture, 0, limit),
		limit: limit,
	}
}

// SetLimit updates the retention limit, trimming immediately if needed.
func (s *CaptureStore) SetLimit(limit int) {
	if limit <= 0 {
		limit = DefaultLogLimit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limit = limit
	if len(s.items) > limit {
		s.items = s.items[:limit]
	}
}

// Add records a capture at the front of the ring.
func (s *CaptureStore) Add(c *Capture) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append([]*Capture{c}, s.items...)
	if len(s.items) > s.limit {
		s.items = s.items[:s.limit]
	}
}

// List returns a shallow copy of all captures (newest first).
func (s *CaptureStore) List() []*Capture {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Capture, len(s.items))
	copy(out, s.items)
	return out
}

// Get returns a single capture by ID, or nil.
func (s *CaptureStore) Get(id string) *Capture {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.items {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// Clear removes all captures.
func (s *CaptureStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = s.items[:0]
}
