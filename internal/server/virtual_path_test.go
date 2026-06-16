package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mainlink0435/warpbox/internal/config"
	"github.com/mainlink0435/warpbox/internal/metadata"
	"github.com/mainlink0435/warpbox/internal/throttle"
)

func seedLibraryFiles(t *testing.T, store *metadata.Store) {
	t.Helper()
	files := []metadata.FileRecord{
		{ItemID: 1, FileID: 10, Source: metadata.SourceTorrent, Name: "movie.mkv", Path: "The.Matrix.1999/movie.mkv", Size: 5000, MimeType: "video/x-matroska"},
		{ItemID: 1, FileID: 11, Source: metadata.SourceTorrent, Name: "featurette.mkv", Path: "The.Matrix.1999/featurette.mkv", Size: 200, MimeType: "video/x-matroska"},
		{ItemID: 1, FileID: 12, Source: metadata.SourceTorrent, Name: "sample.mkv", Path: "The.Matrix.1999/sample.mkv", Size: 100, MimeType: "video/x-matroska"},
		{ItemID: 2, FileID: 20, Source: metadata.SourceTorrent, Name: "ep1.mkv", Path: "Breaking.Bad.S01/ep1.mkv", Size: 1000, MimeType: "video/x-matroska"},
		{ItemID: 2, FileID: 21, Source: metadata.SourceTorrent, Name: "ep2.mkv", Path: "Breaking.Bad.S01/ep2.mkv", Size: 1100, MimeType: "video/x-matroska"},
		{ItemID: 3, FileID: 30, Source: metadata.SourceTorrent, Name: "release.rar", Path: "Archive.Release/release.rar", Size: 50000, MimeType: "application/x-rar"},
		{ItemID: 4, FileID: 40, Source: metadata.SourceTorrent, Name: "random.mp4", Path: "Random.Video/random.mp4", Size: 500, MimeType: "video/mp4"},
	}
	for _, f := range files {
		if err := store.UpsertFile(f); err != nil {
			t.Fatalf("upsert failed: %v", err)
		}
	}
}

func TestVirtualPathRoot_ShowsSyntheticDirs(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version: "test",
		VirtualPaths: []config.VirtualPathConfig{
			{Name: "movies", DirectoryInclude: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`},
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

	body := req("/webdav/")
	if !strings.Contains(body, "__all__") {
		t.Error("/webdav/ root should contain __all__ synthetic dir")
	}
	if !strings.Contains(body, "movies") {
		t.Error("/webdav/ root should contain movies synthetic dir")
	}
	// Should NOT contain real torrent names at root — they're inside __all__
	for _, dir := range []string{"The.Matrix.1999", "Breaking.Bad.S01", "Archive.Release", "Random.Video"} {
		if strings.Contains(body, dir) {
			t.Errorf("/webdav/ root should NOT contain real torrent %q at root", dir)
		}
	}
}

func TestVirtualPath_All_ShowsEverything(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version: "test",
		VirtualPaths: []config.VirtualPathConfig{
			{Name: "movies", DirectoryInclude: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`},
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

	body := req("/webdav/__all__/")
	for _, dir := range []string{"The.Matrix.1999", "Breaking.Bad.S01", "Archive.Release", "Random.Video"} {
		if !strings.Contains(body, dir) {
			t.Errorf("/webdav/__all__/ should contain %q", dir)
		}
	}
}

func TestVirtualPath_MoviesFilter(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version: "test",
		VirtualPaths: []config.VirtualPathConfig{
			{Name: "movies", DirectoryInclude: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`},
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

	body := req("/webdav/movies/")
	if !strings.Contains(body, "The.Matrix.1999") {
		t.Error("/webdav/movies/ should contain The.Matrix.1999")
	}
	for _, dir := range []string{"Breaking.Bad.S01", "Archive.Release", "Random.Video"} {
		if strings.Contains(body, dir) {
			t.Errorf("/webdav/movies/ should NOT contain %q", dir)
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
		Version: "test",
		VirtualPaths: []config.VirtualPathConfig{
			{Name: "movies", DirectoryInclude: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	r := httptest.NewRequest("PROPFIND", "/webdav/movies/The.Matrix.1999/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)
	body := w.Body.String()

	if !strings.Contains(body, "movie.mkv") {
		t.Error("should contain movie.mkv")
	}
	if !strings.Contains(body, "featurette.mkv") {
		t.Error("should contain featurette.mkv")
	}
	if !strings.Contains(body, "sample.mkv") {
		t.Error("should contain sample.mkv (no sample filter)")
	}
}

func TestVirtualPath_TVFilter(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version: "test",
		VirtualPaths: []config.VirtualPathConfig{
			{Name: "tv", DirectoryInclude: "(?i)(season|episode)s?\\.?\\d?|[se]\\d\\d|\\b(tv|complete)", FileRegex: `.*\.(mkv|mp4)$`},
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

	body := req("/webdav/tv/")
	if !strings.Contains(body, "Breaking.Bad.S01") {
		t.Error("/webdav/tv/ should contain Breaking.Bad.S01")
	}
	for _, dir := range []string{"The.Matrix.1999", "Archive.Release", "Random.Video"} {
		if strings.Contains(body, dir) {
			t.Errorf("/webdav/tv/ should NOT contain %q", dir)
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
		Version: "test",
		VirtualPaths: []config.VirtualPathConfig{
			{Name: "movies", DirectoryInclude: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`, LargestFileOnly: true},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	r := httptest.NewRequest("PROPFIND", "/webdav/movies/The.Matrix.1999/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)
	body := w.Body.String()

	if !strings.Contains(body, "movie.mkv") {
		t.Error("should contain movie.mkv (largest)")
	}
	if strings.Contains(body, "featurette.mkv") {
		t.Error("should NOT contain featurette.mkv")
	}
	if strings.Contains(body, "sample.mkv") {
		t.Error("should NOT contain sample.mkv")
	}
}

func TestVirtualPath_NoLibraryConfig(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{Version: "test"}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	// Without virtual paths, /webdav/ shows only __all__ synthetic dir.
	r := httptest.NewRequest("PROPFIND", "/webdav/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)
	body := w.Body.String()

	if !strings.Contains(body, "__all__") {
		t.Error("without virtual paths, /webdav/ should show __all__ synthetic dir")
	}
	for _, dir := range []string{"The.Matrix.1999", "Breaking.Bad.S01"} {
		if strings.Contains(body, dir) {
			t.Errorf("without virtual paths, /webdav/ root should NOT show %q directly", dir)
		}
	}

	// __all__ sub-path shows all real torrent dirs.
	r2 := httptest.NewRequest("PROPFIND", "/webdav/__all__/", nil)
	w2 := httptest.NewRecorder()
	srv.mux.ServeHTTP(w2, r2)
	body2 := w2.Body.String()
	for _, dir := range []string{"The.Matrix.1999", "Breaking.Bad.S01"} {
		if !strings.Contains(body2, dir) {
			t.Errorf("/webdav/__all__/ should show %q", dir)
		}
	}

	// Without virtual paths, virtual names are treated as real dir lookups.
	// They return empty collections (not 404) since PROPFIND shows the dir itself.
	for _, name := range []string{"/webdav/movies", "/webdav/tv"} {
		r3 := httptest.NewRequest("PROPFIND", name+"/", nil)
		w3 := httptest.NewRecorder()
		srv.mux.ServeHTTP(w3, r3)
		resp3 := w3.Result()
		resp3.Body.Close()
		if resp3.StatusCode != http.StatusMultiStatus {
			t.Errorf("without virtual paths, %s/ should return MultiStatus (empty dir), got %d", name, resp3.StatusCode)
		}
		body3 := w3.Body.String()
		// Should only contain the directory itself, no real content.
		if strings.Contains(body3, "The.Matrix.1999") || strings.Contains(body3, "Breaking.Bad.S01") {
			t.Errorf("without virtual paths, %s/ should not contain real content", name)
		}
	}
}

func TestVirtualPath_GETDirListing(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version: "test",
		VirtualPaths: []config.VirtualPathConfig{
			{Name: "movies", DirectoryInclude: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	r := httptest.NewRequest(http.MethodGet, "/webdav/movies/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)
	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("GET /webdav/movies/: got %d, want 207", resp.StatusCode)
	}
	body := w.Body.String()
	if !strings.Contains(body, "The.Matrix.1999") {
		t.Error("should contain The.Matrix.1999")
	}
	if strings.Contains(body, "Breaking.Bad.S01") {
		t.Error("should NOT contain Breaking.Bad.S01")
	}
}

func TestVirtualPath_WithinDirectoryFilter(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version: "test",
		VirtualPaths: []config.VirtualPathConfig{
			{Name: "movies", DirectoryInclude: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`, LargestFileOnly: true},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	r := httptest.NewRequest("PROPFIND", "/webdav/movies/The.Matrix.1999/", nil)
	r.Header.Set("Depth", "1")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)
	body := w.Body.String()

	if !strings.Contains(body, "The.Matrix.1999") {
		t.Error("should contain directory href")
	}
	if !strings.Contains(body, "movie.mkv") {
		t.Error("should contain movie.mkv (largest file)")
	}
	if strings.Contains(body, "featurette.mkv") {
		t.Error("should NOT contain featurette.mkv (not largest)")
	}
	if strings.Contains(body, "sample.mkv") {
		t.Error("should NOT contain sample.mkv (not largest)")
	}
}

func TestVirtualPath_ArchiveHiddenFromMovie(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version: "test",
		VirtualPaths: []config.VirtualPathConfig{
			{Name: "movies", DirectoryInclude: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	r := httptest.NewRequest("PROPFIND", "/webdav/__all__/Archive.Release/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)
	body := w.Body.String()

	if !strings.Contains(body, "release.rar") {
		t.Error("__all__ should show archive files")
	}
}

func TestInfuseVirtualPath(t *testing.T) {
	store, err := metadata.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	seedLibraryFiles(t, store)

	cfg := Config{
		Version: "test",
		VirtualPaths: []config.VirtualPathConfig{
			{Name: "movies", DirectoryInclude: "(?i)(19|20)([0-9]{2})", FileRegex: `.*\.(mkv|mp4|avi)$`},
		},
	}
	queue := throttle.NewQueue(600)
	srv := New(cfg, store, nil, queue)

	r := httptest.NewRequest("PROPFIND", "/infuse/movies/", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, r)
	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("PROPFIND /infuse/movies/: got %d, want 207", resp.StatusCode)
	}
	body := w.Body.String()
	if !strings.Contains(body, "The.Matrix.1999") {
		t.Error("infuse should filter movies")
	}
	if strings.Contains(body, "Breaking.Bad.S01") {
		t.Error("infuse should NOT show TV dirs in movies")
	}
}
