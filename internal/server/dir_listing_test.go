package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ben/warpbox/internal/metadata"
)

func TestServeDirListingRoot(t *testing.T) {
	// Open an in-memory store with some test data.
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	// Seed some files.
	files := []metadata.FileRecord{
		{TorrentID: 1, FileID: 10, Name: "file1.mkv", Path: "Movie.A/file1.mkv", Size: 1000, MimeType: "video/x-matroska"},
		{TorrentID: 1, FileID: 11, Name: "file2.mkv", Path: "Movie.A/file2.mkv", Size: 2000, MimeType: "video/x-matroska"},
		{TorrentID: 2, FileID: 20, Name: "ep1.mkv",  Path: "Show.B/ep1.mkv", Size: 500, MimeType: "video/x-matroska"},
	}
	for _, f := range files {
		if err := store.UpsertFile(f); err != nil {
			t.Fatalf("failed to upsert file: %v", err)
		}
	}

	// Create a server pointing to the in-memory store.
	srv := New(Config{WebDAVRoot: "/webdav", Version: "test"}, store, nil, nil, nil)

	// Simulate GET /webdav/
	req := httptest.NewRequest(http.MethodGet, "/webdav/", nil)
	w := httptest.NewRecorder()
	srv.handleGet(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Verify status is 207 Multi-Status.
	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("expected 207 Multi-Status, got %d %s", resp.StatusCode, resp.Status)
	}

	// Verify Content-Type is XML.
	ct := resp.Header.Get("Content-Type")
	if ct != "application/xml; charset=utf-8" {
		t.Errorf("expected XML content type, got %q", ct)
	}

	// Verify the DAV header is present.
	if resp.Header.Get("DAV") != "1" {
		t.Errorf("expected DAV: 1 header")
	}

	// Verify the XML is well-formed and contains expected elements.
	body := readAllStr(resp.Body)
	if !strings.Contains(body, "<D:multistatus") {
		t.Error("expected <D:multistatus> element")
	}
	if !strings.Contains(body, "<D:href>/webdav/</D:href>") {
		t.Error("expected root href /webdav/")
	}
	if !strings.Contains(body, "<D:collection>") {
		t.Error("expected collection element for directory")
	}
	if !strings.Contains(body, "Movie.A") || !strings.Contains(body, "Show.B") {
		t.Error("expected Movie.A and Show.B in response")
	}
}

func TestServeDirListingSubdir(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	files := []metadata.FileRecord{
		{TorrentID: 1, FileID: 10, Name: "file1.mkv", Path: "Movie.A/file1.mkv", Size: 1000, MimeType: "video/x-matroska"},
	}
	for _, f := range files {
		if err := store.UpsertFile(f); err != nil {
			t.Fatalf("failed to upsert file: %v", err)
		}
	}

	srv := New(Config{WebDAVRoot: "/webdav", Version: "test"}, store, nil, nil, nil)

	// Simulate GET /webdav/Movie.A/ — a subdirectory.
	req := httptest.NewRequest(http.MethodGet, "/webdav/Movie.A/", nil)
	w := httptest.NewRecorder()
	srv.handleGet(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("expected 207 Multi-Status, got %d %s", resp.StatusCode, resp.Status)
	}

	body := readAllStr(resp.Body)
	if !strings.Contains(body, "<D:multistatus") {
		t.Error("expected <D:multistatus> element")
	}
	if !strings.Contains(body, "<D:href>/webdav/Movie.A/</D:href>") {
		t.Error("expected dir href /webdav/Movie.A/")
	}
	if !strings.Contains(body, "<D:collection>") {
		t.Error("expected collection element for directory")
	}
	if !strings.Contains(body, "file1.mkv") {
		t.Error("expected file1.mkv in response")
	}
}

func TestServeDirListingMissingPath(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	srv := New(Config{WebDAVRoot: "/webdav", Version: "test"}, store, nil, nil, nil)

	// GET on a path that doesn't exist and has no children.
	req := httptest.NewRequest(http.MethodGet, "/webdav/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.handleGet(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent path, got %d", resp.StatusCode)
	}
}

func readAllStr(r io.ReadCloser) string {
	b, _ := io.ReadAll(r)
	r.Close()
	return string(b)
}

func TestServeDirListingNestedPaths(t *testing.T) {
	// Test that PROPFIND with depth=1 on a directory containing files
	// with nested paths returns only immediate children, not deeply nested entries.
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	// Simulate the real-world scenario from issue #37:
	// Torrent "The.Studio.2025.S01.MULTi.1080p.WEB.H265-FW" contains files with
	// paths like "The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW/The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW.mkv"
	// This creates a nested structure: TorrentName/SubDir/File.ext
	files := []metadata.FileRecord{
		// A torrent with nested subdirectories (the common case from the bug report)
		{TorrentID: 1, FileID: 10, Name: "The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW.mkv",
			Path: "The.Studio.2025.S01.MULTi.1080p.WEB.H265-FW/The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW/The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW.mkv",
			Size: 1000, MimeType: "video/x-matroska"},
		{TorrentID: 1, FileID: 11, Name: "The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW.nfo",
			Path: "The.Studio.2025.S01.MULTi.1080p.WEB.H265-FW/The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW/The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW.nfo",
			Size: 500, MimeType: "text/plain"},
		// A torrent where files are directly at the root (normal case)
		{TorrentID: 2, FileID: 20, Name: "movie.mkv",
			Path: "Simple.Movie/movie.mkv",
			Size: 2000, MimeType: "video/x-matroska"},
	}
	for _, f := range files {
		if err := store.UpsertFile(f); err != nil {
			t.Fatalf("failed to upsert file: %v", err)
		}
	}

	srv := New(Config{WebDAVRoot: "/webdav", Version: "test"}, store, nil, nil, nil)

	// --- Test 1: Listing the root should show only the torrent directories ---
	req := httptest.NewRequest(http.MethodGet, "/webdav/", nil)
	w := httptest.NewRecorder()
	srv.handleGet(w, req)

	resp := w.Result()
	body := readAllStr(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("expected 207 Multi-Status, got %d", resp.StatusCode)
	}

	// Should NOT contain deeply nested paths as direct children
	if strings.Contains(body, "The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW.mkv") {
		t.Error("root listing should NOT contain deeply nested file; only the torrent directory")
	}
	if strings.Contains(body, "Simple.Movie/movie.mkv") {
		t.Error("root listing should NOT contain child file paths with slash")
	}

	// Should only contain the torrent names as directory entries
	if !strings.Contains(body, "<D:href>/webdav/The.Studio.2025.S01.MULTi.1080p.WEB.H265-FW/</D:href>") {
		t.Error("root listing should contain the nested torrent as a directory entry")
	}
	if !strings.Contains(body, "<D:href>/webdav/Simple.Movie/</D:href>") {
		t.Error("root listing should contain Simple.Movie as a directory entry")
	}

	// --- Test 2: Listing the Simple.Movie directory should show the file directly ---
	req2 := httptest.NewRequest(http.MethodGet, "/webdav/Simple.Movie/", nil)
	w2 := httptest.NewRecorder()
	srv.handleGet(w2, req2)

	resp2 := w2.Result()
	body2 := readAllStr(resp2.Body)
	resp2.Body.Close()

	if !strings.Contains(body2, "<D:href>/webdav/Simple.Movie/movie.mkv</D:href>") {
		t.Error("Simple.Movie listing should contain movie.mkv directly")
	}

	// --- Test 3: Listing the nested torrent directory should show the subdirectory, not the file ---
	req3 := httptest.NewRequest(http.MethodGet, "/webdav/The.Studio.2025.S01.MULTi.1080p.WEB.H265-FW/", nil)
	w3 := httptest.NewRecorder()
	srv.handleGet(w3, req3)

	resp3 := w3.Result()
	body3 := readAllStr(resp3.Body)
	resp3.Body.Close()

	// Should contain the subdirectory entry (with trailing slash)
	if !strings.Contains(body3, "<D:href>/webdav/The.Studio.2025.S01.MULTi.1080p.WEB.H265-FW/The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW/</D:href>") {
		t.Error("nested torrent listing should contain subdirectory, not deeply nested file")
	}
	// Should NOT contain the deeply nested file directly in this listing
	if strings.Contains(body3, "The.Studio.2025.S01E02.MULTi.1080p.WEB.H265-FW.mkv") {
		t.Error("nested torrent listing should NOT contain the file directly; only the subdirectory")
	}

	// Should have a <D:collection> for the subdirectory
	if !strings.Contains(body3, "<D:collection>") {
		t.Error("subdirectory should have a collection resource type")
	}
}

func TestServeDirListingGETRootNoSlash(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	defer store.Close()

	files := []metadata.FileRecord{
		{TorrentID: 1, FileID: 10, Name: "file.mkv", Path: "Torrent/file.mkv", Size: 1000, MimeType: "video/x-matroska"},
	}
	for _, f := range files {
		if err := store.UpsertFile(f); err != nil {
			t.Fatalf("failed to upsert file: %v", err)
		}
	}

	srv := New(Config{WebDAVRoot: "/webdav", Version: "test"}, store, nil, nil, nil)

	// GET /webdav (without trailing slash) — this is the case the user reported.
	req := httptest.NewRequest(http.MethodGet, "/webdav", nil)
	w := httptest.NewRecorder()
	srv.handleGet(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("expected 207 Multi-Status, got %d", resp.StatusCode)
	}

	body := readAllStr(resp.Body)
	if !strings.Contains(body, "<D:multistatus") {
		t.Error("expected valid multi-status XML")
	}
}
