package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ben/warpbox/internal/config"
	"github.com/ben/warpbox/internal/metadata"
	"github.com/ben/warpbox/internal/throttle"
)

// seedLibraryFiles adds a realistic set of test files across multiple torrents.
func seedLibraryFiles(t *testing.T, store *metadata.Store) {
	t.Helper()
	files := []metadata.FileRecord{
		// Movie: single file, year in name
		{ItemID: 1, FileID: 10, Source: metadata.SourceTorrent, Name: "movie.mkv", Path: "The.Matrix.1999/movie.mkv", Size: 5000, MimeType: "video/x-matroska"},
		// Movie: featurette (same torrent, smaller)
		{ItemID: 1, FileID: 11, Source: metadata.SourceTorrent, Name: "featurette.mkv", Path: "The.Matrix.1999/featurette.mkv", Size: 200, MimeType: "video/x-matroska"},
		// Movie: sample file
		{ItemID: 1, FileID: 12, Source: metadata.SourceTorrent, Name: "sample.mkv", Path: "The.Matrix.1999/sample.mkv", Size: 100, MimeType: "video/x-matroska"},
		// TV: season pattern
		{ItemID: 2, FileID: 20, Source: metadata.SourceTorrent, Name: "ep1.mkv", Path: "Breaking.Bad.S01/ep1.mkv", Size: 1000, MimeType: "video/x-matroska"},
		{ItemID: 2, FileID: 21, Source: metadata.SourceTorrent, Name: "ep2.mkv", Path: "Breaking.Bad.S01/ep2.mkv", Size: 1100, MimeType: "video/x-matroska"},
		// Archive: only archive files, no video
		{ItemID: 3, FileID: 30, Source: metadata.SourceTorrent, Name: "release.rar", Path: "Archive.Release/release.rar", Size: 50000, MimeType: "application/x-rar"},
		// Unmatched: no year or season in name
		{ItemID: 4, FileID: 40, Source: metadata.SourceTorrent, Name: "random.mp4", Path: "Random.Video/random.mp4", Size: 500, MimeType: "video/mp4"},
	}
	for _, f := range files {
		if err := store.UpsertFile(f); err != nil {
			t.Fatalf("upsert failed: %v", err)
		}
	}
}

func TestVirtualPathRouting_DefaultWebDAV(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version:    "test",
		WebDAVRoot: "/webdav",
		VirtualPaths: []config.VirtualPathConfig{
			{Mount: "/movies", DirectoryRegex: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`, LargestFileOnly: false},
			{Mount: "/tv", DirectoryRegex: "(?i)(season|episode)s?\\.?\\d?|[se]\\d\\d|\\b(tv|complete)", FileRegex: `.*\.(mkv|mp4)$`, LargestFileOnly: false},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	req := func(method, path string) *http.Response {
		r := httptest.NewRequest(method, path, nil)
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, r)
		return w.Result()
	}

	// All 3 mount points should respond to PROPFIND.
	tests := []struct {
		name string
		path string
		code int
	}{
		{"__all__ root", "/webdav/", http.StatusMultiStatus},
		{"movies root", "/movies/", http.StatusMultiStatus},
		{"tv root", "/tv/", http.StatusMultiStatus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := req("PROPFIND", tt.path)
			if resp.StatusCode != tt.code {
				t.Errorf("PROPFIND %s: got %d, want %d", tt.path, resp.StatusCode, tt.code)
			}
		})
	}
}

func TestVirtualPath_AllShowsEverything(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version:    "test",
		WebDAVRoot: "/webdav",
		VirtualPaths: []config.VirtualPathConfig{
			{Mount: "/movies", DirectoryRegex: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	req := func(path string) string {
		r := httptest.NewRequest("PROPFIND", path, nil)
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, r)
		return w.Body.String()
	}

	allBody := req("/webdav/")
	// __all__ should contain all torrent directories
	for _, dir := range []string{"The.Matrix.1999", "Breaking.Bad.S01", "Archive.Release", "Random.Video"} {
		if !strings.Contains(allBody, dir) {
			t.Errorf("__all__ should contain %q", dir)
		}
	}
}

func TestVirtualPath_MoviesFilterOnly(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version:    "test",
		WebDAVRoot: "/webdav",
		VirtualPaths: []config.VirtualPathConfig{
			{Mount: "/movies", DirectoryRegex: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	req := func(path string) string {
		r := httptest.NewRequest("PROPFIND", path, nil)
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, r)
		return w.Body.String()
	}

	moviesBody := req("/movies/")
	// /movies/ should contain the Matrix (year in name)
	if !strings.Contains(moviesBody, "The.Matrix.1999") {
		t.Error("/movies/ should contain The.Matrix.1999")
	}
	// /movies/ should NOT contain TV shows, archives, or random videos
	for _, dir := range []string{"Breaking.Bad.S01", "Archive.Release", "Random.Video"} {
		if strings.Contains(moviesBody, dir) {
			t.Errorf("/movies/ should NOT contain %q", dir)
		}
	}
}

func TestVirtualPath_MoviesDirectoryContents(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version:    "test",
		WebDAVRoot: "/webdav",
		VirtualPaths: []config.VirtualPathConfig{
			{Mount: "/movies", DirectoryRegex: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`, LargestFileOnly: false},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	// Browse INTO the Matrix directory — should show all video files.
	r := httptest.NewRequest("PROPFIND", "/movies/The.Matrix.1999/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "movie.mkv") {
		t.Error("/movies/The.Matrix.1999/ should contain movie.mkv")
	}
	if !strings.Contains(body, "featurette.mkv") {
		t.Error("/movies/The.Matrix.1999/ should contain featurette.mkv")
	}
	if !strings.Contains(body, "sample.mkv") {
		t.Error("/movies/The.Matrix.1999/ should contain sample.mkv (no sample filter)")
	}
}

func TestVirtualPath_TVFilterOnly(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version:    "test",
		WebDAVRoot: "/webdav",
		VirtualPaths: []config.VirtualPathConfig{
			{Mount: "/tv", DirectoryRegex: "(?i)(season|episode)s?\\.?\\d?|[se]\\d\\d|\\b(tv|complete)", FileRegex: `.*\.(mkv|mp4)$`},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	req := func(path string) string {
		r := httptest.NewRequest("PROPFIND", path, nil)
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, r)
		return w.Body.String()
	}

	tvBody := req("/tv/")
	if !strings.Contains(tvBody, "Breaking.Bad.S01") {
		t.Error("/tv/ should contain Breaking.Bad.S01")
	}
	for _, dir := range []string{"The.Matrix.1999", "Archive.Release", "Random.Video"} {
		if strings.Contains(tvBody, dir) {
			t.Errorf("/tv/ should NOT contain %q", dir)
		}
	}
}

func TestVirtualPath_LargestFileOnly(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version:    "test",
		WebDAVRoot: "/webdav",
		VirtualPaths: []config.VirtualPathConfig{
			{Mount: "/movies", DirectoryRegex: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`, LargestFileOnly: true},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	req := func(path string) string {
		r := httptest.NewRequest("PROPFIND", path, nil)
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, r)
		return w.Body.String()
	}

	// Browse into the Matrix directory - should show only the largest file.
	dirBody := req("/movies/The.Matrix.1999/")
	if !strings.Contains(dirBody, "movie.mkv") {
		t.Error("should contain movie.mkv (largest)")
	}
	if strings.Contains(dirBody, "featurette.mkv") {
		t.Error("should NOT contain featurette.mkv (smaller)")
	}
	if strings.Contains(dirBody, "sample.mkv") {
		t.Error("should NOT contain sample.mkv (smaller)")
	}
}

func TestVirtualPath_FileDoesntMatchExtension(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version:    "test",
		WebDAVRoot: "/webdav",
		VirtualPaths: []config.VirtualPathConfig{
			{Mount: "/archives", DirectoryRegex: ".*", FileRegex: `.*\.(rar|zip|7z)$`},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	req := func(path string) string {
		r := httptest.NewRequest("PROPFIND", path, nil)
		w := httptest.NewRecorder()
		srv.mux.ServeHTTP(w, r)
		return w.Body.String()
	}

	body := req("/archives/")
	// Should show Archive.Release directory (matches all dirs)
	if !strings.Contains(body, "Archive.Release") {
		t.Error("/archives/ should contain Archive.Release")
	}
	// Inside Archive.Release — should show archive files
	dirBody := req("/archives/Archive.Release/")
	if !strings.Contains(dirBody, "release.rar") {
		t.Error("should contain release.rar")
	}
	// TV/Movie directories should also appear but their files won't match
	// .rar extension — but at root level they appear as dirs
}

func TestVirtualPath_GETDirListing(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version:    "test",
		WebDAVRoot: "/webdav",
		VirtualPaths: []config.VirtualPathConfig{
			{Mount: "/movies", DirectoryRegex: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	r := httptest.NewRequest(http.MethodGet, "/movies/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("GET /movies/: got %d, want 207", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "The.Matrix.1999") {
		t.Error("GET /movies/ should contain The.Matrix.1999")
	}
	if strings.Contains(body, "Breaking.Bad.S01") {
		t.Error("GET /movies/ should NOT contain Breaking.Bad.S01")
	}
}

func TestVirtualPath_NoLibraryConfig(t *testing.T) {
	// When no library config is provided, /webdav should still work as before,
	// and virtual path mounts should not exist.
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version:    "test",
		WebDAVRoot: "/webdav",
		// No VirtualPaths = no virtual mount points
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	r := httptest.NewRequest("PROPFIND", "/webdav/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)
	resp := w.Result()
	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("PROPFIND /webdav/ without library: got %d, want 207", resp.StatusCode)
	}

	// Virtual path mounts should 404
	for _, mount := range []string{"/movies", "/tv"} {
		r2 := httptest.NewRequest("PROPFIND", mount+"/", nil)
		w2 := httptest.NewRecorder()
		srv.mux.ServeHTTP(w2, r2)
		if w2.Result().StatusCode != http.StatusNotFound {
			t.Errorf("without library config, %s/ should 404", mount)
		}
	}
}

func TestVirtualPath_WithinDirectory(t *testing.T) {
	// When browsing inside a torrent directory under a virtual path,
	// the filter should still apply file_regex and largest_file_only.
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version:    "test",
		WebDAVRoot: "/webdav",
		VirtualPaths: []config.VirtualPathConfig{
			{Mount: "/movies", DirectoryRegex: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`, LargestFileOnly: true},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	// PROPFIND with depth=1 on /movies/The.Matrix.1999/
	r := httptest.NewRequest("PROPFIND", "/movies/The.Matrix.1999/", nil)
	r.Header.Set("Depth", "1")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)

	body := w.Body.String()
	// Should show the directory itself
	if !strings.Contains(body, "The.Matrix.1999") {
		t.Error("should contain directory href")
	}
	// Should show only the largest file (movie.mkv = 5000)
	if !strings.Contains(body, "movie.mkv") {
		t.Error("should contain movie.mkv (largest file)")
	}
	// Should NOT show the smaller files
	if strings.Contains(body, "featurette.mkv") {
		t.Error("should NOT contain featurette.mkv (not largest)")
	}
	if strings.Contains(body, "sample.mkv") {
		t.Error("should NOT contain sample.mkv (not largest)")
	}
}
