package library

import (
	"testing"

	"github.com/ben/warpbox/internal/metadata"
)

func TestNewFilter(t *testing.T) {
	f, err := NewFilter("/tv", "(?i)season", `.*\.(mkv|mp4)$`, true)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}
	if f.Mount != "/tv" {
		t.Errorf("Mount = %q, want /tv", f.Mount)
	}
	if !f.LargestFileOnly {
		t.Error("LargestFileOnly should be true")
	}
}

func TestNewFilter_EmptyRegex(t *testing.T) {
	f, err := NewFilter("/all", "", "", false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}
	if f.DirectoryRegex != nil {
		t.Error("DirectoryRegex should be nil for empty string")
	}
	if f.FileRegex != nil {
		t.Error("FileRegex should be nil for empty string")
	}
}

func TestNewFilter_InvalidRegex(t *testing.T) {
	_, err := NewFilter("/bad", "[invalid", "", false)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestExtractDirectory(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"Movie.Name.1999/file.mkv", "Movie.Name.1999"},
		{"TV.Show.S01/Season 1/ep1.mkv", "TV.Show.S01"},
		{"singlefile.mkv", "singlefile.mkv"},
		{"", ""},
		{"a/b/c/d.mkv", "a"},
	}
	for _, tt := range tests {
		got := ExtractDirectory(tt.path)
		if got != tt.want {
			t.Errorf("ExtractDirectory(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestExtractRelativePath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"Movie.Name.1999/file.mkv", "file.mkv"},
		{"TV.Show.S01/Season 1/ep1.mkv", "Season 1/ep1.mkv"},
		{"singlefile.mkv", "singlefile.mkv"},
		{"", ""},
	}
	for _, tt := range tests {
		got := ExtractRelativePath(tt.path)
		if got != tt.want {
			t.Errorf("ExtractRelativePath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// Gold TV regex: the user's main classification regex
var tvRegex = "(?i)(season|episode)s?\\.?\\d?|[se]\\d\\d|\\b(tv|complete)|\\b(saison|stage)\\.?\\d|[a-z]\\s?-\\s?\\d{2,4}\\b|\\d{2,4}\\s?-\\s?\\d{2,4}\\b"

func TestMatchDirectory_TV(t *testing.T) {
	f, err := NewFilter("/tv", tvRegex, "", false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	tvDirs := []string{
		"Breaking.Bad.S01.1080p",
		"The.Office.Season.3.Complete",
		"Game.of.Thrones.S08E01",
		"TV.Show.Complete.Series",
		"Show.Name.01-10",
		"Show.2001-2005",
		"Some.S01E01.Complete",
		"Saison.1.Show",
		"Stage.2.Show",
		"a-2024",
		"Show.tv.Complete",
		"Episode.1.Show",
	}
	for _, dir := range tvDirs {
		if !f.MatchDirectory(dir) {
			t.Errorf("TV regex should match %q", dir)
		}
	}
}

func TestMatchDirectory_NotTV(t *testing.T) {
	f, err := NewFilter("/tv", tvRegex, "", false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	nonTVDirs := []string{
		"The.Matrix.1999.1080p",
		"Random.Video.File",
		"Inception.2010.4K",
		"Documentary.2023",
	}
	for _, dir := range nonTVDirs {
		if f.MatchDirectory(dir) {
			t.Errorf("TV regex should NOT match %q", dir)
		}
	}
}

// Movie regex: simple 4-digit year
var movieRegex = "(?i)(19|20)([0-9]{2})"

func TestMatchDirectory_Movie(t *testing.T) {
	f, err := NewFilter("/movies", movieRegex, "", false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	movieDirs := []string{
		"The.Matrix.1999.1080p",
		"Inception.2010.4K",
		"Documentary.2023",
		"Interstellar.2014.2160p",
		"Movie.Title.1984",
	}
	for _, dir := range movieDirs {
		if !f.MatchDirectory(dir) {
			t.Errorf("Movie regex should match %q", dir)
		}
	}
}

func TestMatchDirectory_NotMovie(t *testing.T) {
	f, err := NewFilter("/movies", movieRegex, "", false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	nonMovieDirs := []string{
		"Breaking.Bad.S01.1080p",
		"Random.File",
		"Music.Album.FLAC",
	}
	for _, dir := range nonMovieDirs {
		if f.MatchDirectory(dir) {
			t.Errorf("Movie regex should NOT match %q", dir)
		}
	}
}

func TestMatchDirectory_NoFilter(t *testing.T) {
	f, err := NewFilter("/all", "", "", false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}
	if !f.MatchDirectory("anything") {
		t.Error("no filter should match everything")
	}
}

func TestMatchFile(t *testing.T) {
	f, err := NewFilter("/movies", "", `.*\.(mkv|mp4|avi)$`, false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	if !f.MatchFile("movie.mkv") {
		t.Error("should match .mkv")
	}
	if !f.MatchFile("show.mp4") {
		t.Error("should match .mp4")
	}
	if !f.MatchFile("clip.avi") {
		t.Error("should match .avi")
	}
	if f.MatchFile("archive.rar") {
		t.Error("should NOT match .rar")
	}
	if f.MatchFile("sample.txt") {
		t.Error("should NOT match .txt")
	}
}

func TestMatchFile_RelativePath(t *testing.T) {
	f, err := NewFilter("/tv", "", `.*\.(mkv|mp4)$`, false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	if !f.MatchFile("Season 1/episode.mkv") {
		t.Error("should match path with subdirectories")
	}
	if f.MatchFile("Season 1/sample.txt") {
		t.Error("should NOT match non-video in subdirectory")
	}
}

func TestKeepLargest(t *testing.T) {
	records := []metadata.FileRecord{
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "Movie.A/file1.mkv", Size: 500},
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "Movie.A/file2.mkv", Size: 1000},
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "Movie.A/featurette.mkv", Size: 200},
	}
	got := KeepLargest(records)
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	if got[0].Size != 1000 {
		t.Errorf("expected largest file (1000), got %d", got[0].Size)
	}
}

func TestKeepLargest_MultipleItems(t *testing.T) {
	records := []metadata.FileRecord{
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "Movie.A/main.mkv", Size: 1000},
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "Movie.A/featurette.mkv", Size: 200},
		{ItemID: 2, Source: metadata.SourceTorrent, Path: "Show.B/ep1.mkv", Size: 500},
		{ItemID: 2, Source: metadata.SourceTorrent, Path: "Show.B/ep2.mkv", Size: 600},
	}
	got := KeepLargest(records)
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	if got[0].Size != 1000 || got[0].ItemID != 1 {
		t.Errorf("expected item 1 largest (1000), got item %d size %d", got[0].ItemID, got[0].Size)
	}
	if got[1].Size != 600 || got[1].ItemID != 2 {
		t.Errorf("expected item 2 largest (600), got item %d size %d", got[1].ItemID, got[1].Size)
	}
}

func TestKeepLargest_SingleFile(t *testing.T) {
	records := []metadata.FileRecord{
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "Movie.A/file.mkv", Size: 500},
	}
	got := KeepLargest(records)
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	if got[0].Size != 500 {
		t.Errorf("expected size 500, got %d", got[0].Size)
	}
}

func TestKeepLargest_Empty(t *testing.T) {
	got := KeepLargest(nil)
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestKeepLargest_SourceDisambiguation(t *testing.T) {
	records := []metadata.FileRecord{
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "item/file1.mkv", Size: 500},
		{ItemID: 1, Source: metadata.SourceUsenet, Path: "item/file2.mkv", Size: 600},
	}
	got := KeepLargest(records)
	if len(got) != 2 {
		t.Fatalf("expected 2 records (different sources), got %d", len(got))
	}
}

func TestApplyFilter_FullFlow(t *testing.T) {
	f, err := NewFilter("/movies", movieRegex, `.*\.(mkv|mp4)$`, false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	records := []metadata.FileRecord{
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "The.Matrix.1999/movie.mkv", Size: 5000, MimeType: "video/x-matroska"},
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "The.Matrix.1999/featurette.mkv", Size: 200, MimeType: "video/x-matroska"},
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "The.Matrix.1999/sample.txt", Size: 50, MimeType: "text/plain"},
		{ItemID: 2, Source: metadata.SourceTorrent, Path: "Breaking.Bad.S01/ep1.mkv", Size: 1000, MimeType: "video/x-matroska"},
		{ItemID: 3, Source: metadata.SourceTorrent, Path: "Archive.Release/archive.rar", Size: 50000, MimeType: "application/x-rar"},
		{ItemID: 4, Source: metadata.SourceTorrent, Path: "Unmatched.Video/file.mp4", Size: 500, MimeType: "video/mp4"},
	}

	got := f.Apply(records)
	if len(got) != 2 {
		t.Fatalf("expected 2 records (movie.mkv + featurette.mkv), got %d", len(got))
	}
	for _, r := range got {
		if r.ItemID != 1 {
			t.Errorf("expected only item 1 (Matrix), got item %d", r.ItemID)
		}
		if r.Size == 50 {
			t.Error("sample.txt should have been filtered out")
		}
		if r.Size == 50000 {
			t.Error("archive.rar should have been filtered out")
		}
	}
}

func TestApplyFilter_LargestFileOnly(t *testing.T) {
	f, err := NewFilter("/movies", movieRegex, `.*\.(mkv|mp4)$`, true)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	records := []metadata.FileRecord{
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "The.Matrix.1999/movie.mkv", Size: 5000},
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "The.Matrix.1999/featurette.mkv", Size: 200},
	}

	got := f.Apply(records)
	if len(got) != 1 {
		t.Fatalf("expected 1 record (largest only), got %d", len(got))
	}
	if got[0].Size != 5000 {
		t.Errorf("expected largest (5000), got %d", got[0].Size)
	}
}

func TestApplyFilter_NoDirectoryMatch(t *testing.T) {
	f, err := NewFilter("/movies", movieRegex, `.*\.(mkv|mp4)$`, false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	records := []metadata.FileRecord{
		{ItemID: 2, Source: metadata.SourceTorrent, Path: "Breaking.Bad.S01/ep1.mkv", Size: 1000},
	}

	got := f.Apply(records)
	if len(got) != 0 {
		t.Errorf("expected 0 records (no movie match), got %d", len(got))
	}
}

func TestApplyFilter_NoFileMatch(t *testing.T) {
	f, err := NewFilter("/movies", movieRegex, `.*\.(mkv|mp4)$`, false)
	if err != nil {
		t.Fatalf("NewFilter failed: %v", err)
	}

	records := []metadata.FileRecord{
		{ItemID: 1, Source: metadata.SourceTorrent, Path: "The.Matrix.1999/sample.txt", Size: 100},
	}

	got := f.Apply(records)
	if len(got) != 0 {
		t.Errorf("expected 0 records (no video file), got %d", len(got))
	}
}
