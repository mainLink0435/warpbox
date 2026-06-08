// Package server implements the WebDAV HTTP handler for Warpbox.
//
// This file contains the branded HTML landing page served at the root
// URL (/). The Warpbox logo is compiled into the binary via Go's embed
// package so there are no external file dependencies at runtime.

package server

import (
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"runtime"
	"time"
)

//go:embed landing.html warpbox.svg
var landingFS embed.FS

// landingTmpl is the parsed landing page template.
var landingTmpl = template.Must(template.New("landing").Parse(
	mustReadString(landingFS, "landing.html"),
))

// mustReadString reads an embedded file and returns its contents as a string.
// Panics on failure (called during init via template.Must).
func mustReadString(fs embed.FS, name string) string {
	b, err := fs.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("embedded file %s: %v", name, err))
	}
	return string(b)
}

// LandingData holds the dynamic values rendered into the landing page template.
type LandingData struct {
	Version              string
	Uptime               string
	FileCount            int
	WebDAVURL            string
	HTTPURL              string
	InfuseURL            string
	LogsURL              string
	AllocMB              uint64
	TotalAllocMB         uint64
	SysMB                uint64
	NumGC                uint64
	ListenAddr           string
	WebDAVRoot           string
	MaxRAMMB             int
	ChunkSizeMB          int
	TTLSeconds           int
	EvictionStrategy     string
	CDNURLTTLMinutes     int
	RequestsPerMinute    int
	SyncIntervalMinutes  int
	LogFormat            string
	LogLevel             string
	APICallsTotal        int64
	APICallsLastMinute   int
}

// handleLanding serves the Warpbox branded landing page with runtime stats.
func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Gather runtime data.
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptime := time.Since(s.startTime)
	uptimeStr := formatDuration(uptime)

	// Count files in the store.
	fileCount, err := s.store.CountFiles()
	if err != nil {
		slog.Error("landing: CountFiles failed", "error", err)
		fileCount = -1
	}

	// Throttle stats.
	throttleStats := s.queue.Stats()

	data := LandingData{
		Version:             s.cfg.Version,
		Uptime:              uptimeStr,
		FileCount:           fileCount,
		WebDAVURL:           s.root + "/",
		HTTPURL:             "/http/",
		InfuseURL:           "/infuse/",
		LogsURL:             "/logs/",
		AllocMB:             mem.Alloc / 1024 / 1024,
		TotalAllocMB:        mem.TotalAlloc / 1024 / 1024,
		SysMB:               mem.Sys / 1024 / 1024,
		NumGC:               uint64(mem.NumGC),
		ListenAddr:          s.cfg.ListenAddr,
		WebDAVRoot:          s.cfg.WebDAVRoot,
		MaxRAMMB:            s.cfg.MaxRAMMB,
		ChunkSizeMB:         s.cfg.ChunkSizeMB,
		TTLSeconds:          s.cfg.TTLSeconds,
		EvictionStrategy:    s.cfg.EvictionStrategy,
		CDNURLTTLMinutes:    s.cfg.CDNTtlMinutes,
		RequestsPerMinute:   s.cfg.RequestsPerMinute,
		SyncIntervalMinutes: s.cfg.SyncIntervalMinute,
		LogFormat:           s.cfg.LogFormat,
		LogLevel:            s.cfg.LogLevel,
		APICallsTotal:       throttleStats.TotalCalls,
		APICallsLastMinute:  throttleStats.CallsLastMinute,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := landingTmpl.Execute(w, data); err != nil {
		slog.Error("landing: template execute failed", "error", err)
	}
}

// formatDuration returns a human-readable duration string like "2h34m12s".
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// handleLogo serves the embedded warpbox.svg at /warpbox.svg and also at
// /favicon.ico, giving the landing page a branded browser tab icon.
func (s *Server) handleLogo(w http.ResponseWriter, r *http.Request) {
	svg, err := landingFS.ReadFile("warpbox.svg")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.WriteHeader(http.StatusOK)
	w.Write(svg)
}
