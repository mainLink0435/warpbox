// WebDAV GET handler — serves file content via throttle → cache → CDN pipeline.
//
// Handles byte-range requests for partial content delivery (used by rclone
// for metadata scanning and media server streaming). CDN URLs are cached in
// the SQLite store with configurable TTL to minimise TorBox API calls.
package server

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ben/warpbox/internal/throttle"
)

// ---------------------------------------------------------------------------
// GET handler
// ---------------------------------------------------------------------------

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	// Resolve virtual path.
	virtualPath := strings.TrimPrefix(r.URL.Path, s.root)
	virtualPath = strings.TrimPrefix(virtualPath, "/")

	if virtualPath == "" {
		s.serveDirListing(w, r.URL.Path, "1")
		return
	}

	slog.Debug("GET", "path", virtualPath, "range", r.Header.Get("Range"))

	// Look up the file in the SQLite store.
	file, err := s.store.GetFileByPath(virtualPath)
	if err != nil {
		slog.Error("GET: store lookup failed", "path", virtualPath, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if file == nil {
		// Not a file — check if it's a virtual directory with children.
		records, listErr := s.store.ListDir(virtualPath)
		if listErr != nil {
			slog.Error("GET: ListDir failed", "prefix", virtualPath, "error", listErr)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		if len(records) > 0 {
			s.serveDirListing(w, r.URL.Path, "1")
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Get or refresh the CDN URL.
	cdnURL, err := s.store.GetCDNURL(file.ID)
	if err != nil {
		slog.Error("GET: CDN URL lookup failed", "id", file.ID, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if cdnURL == "" {
		// No cached CDN URL — fetch one via the throttle queue.
		type result struct {
			url string
			err error
		}
		resCh := make(chan result, 1)

		s.queue.Enqueue(throttle.Request{
			Label: fmt.Sprintf("GET CDN URL for file %d", file.FileID),
			Execute: func(ctx context.Context) error {
				url, err := s.torBox.GetDownloadURL(ctx, file.TorrentID, file.FileID, false)
				resCh <- result{url, err}
				return err
			},
		})

		res := <-resCh
		if res.err != nil {
			slog.Error("GET: failed to get CDN URL", "torrent_id", file.TorrentID, "file_id", file.FileID, "error", res.err)
			http.Error(w, "Failed to get download URL", http.StatusBadGateway)
			return
		}
		cdnURL = res.url

		// Cache the CDN URL if TTL > 0.
		if s.cfg.CDNTtlMinutes > 0 {
			expiry := time.Now().Add(time.Duration(s.cfg.CDNTtlMinutes) * time.Minute)
			if err := s.store.SetCDNURL(file.ID, cdnURL, expiry); err != nil {
				slog.Error("GET: failed to cache CDN URL", "path", file.Path, "error", err)
			}
		}
	}

	// Determine if the client requested a byte range.
	rangeHeader := r.Header.Get("Range")
	if rangeHeader == "" {
		// No range — redirect directly to the CDN URL.
		slog.Debug("GET: redirecting to CDN", "url", cdnURL)
		http.Redirect(w, r, cdnURL, http.StatusFound)
		return
	}

	// Parse the byte range.
	srvRange, err := parseRange(rangeHeader, file.Size)
	if err != nil {
		slog.Error("GET: invalid range", "range", rangeHeader, "error", err)
		http.Error(w, "Invalid range", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// Check the RAM cache first.
	cachedData := s.cache.Get(int(file.ID), srvRange.Start)
	if cachedData != nil {
		slog.Debug("GET: cache hit", "id", file.ID, "offset", srvRange.Start)
		mime := file.MimeType
		if mime == "" {
			mime = "application/octet-stream"
		}
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Length", strconv.FormatInt(srvRange.Length, 10))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", srvRange.Start, srvRange.End, file.Size))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(cachedData)
		return
	}

	// Cache miss — fetch the data through a proxied request to the CDN URL.
	slog.Debug("GET: cache miss, proxying from CDN", "id", file.ID, "offset", srvRange.Start)

	client := &http.Client{Timeout: 30 * time.Second}
	proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, cdnURL, http.NoBody)
	if err != nil {
		slog.Error("GET: failed to create CDN request", "error", err)
		http.Error(w, "Failed to create upstream request", http.StatusInternalServerError)
		return
	}
	proxyReq.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", srvRange.Start, srvRange.End))

	proxyResp, err := client.Do(proxyReq)
	if err != nil {
		slog.Error("GET: CDN proxy request failed", "error", err)
		http.Error(w, "CDN proxy error", http.StatusBadGateway)
		return
	}
	defer proxyResp.Body.Close()

	data, err := io.ReadAll(proxyResp.Body)
	if err != nil {
		slog.Error("GET: failed to read CDN response", "error", err)
		http.Error(w, "CDN read error", http.StatusBadGateway)
		return
	}

	// Cache the chunk in RAM.
	s.cache.Put(int(file.ID), srvRange.Start, data)

	// Serve the partial content.
	mime := file.MimeType
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", srvRange.Start, srvRange.End, file.Size))
	w.WriteHeader(http.StatusPartialContent)
	w.Write(data)
}

// ---------------------------------------------------------------------------
// Byte range parsing
// ---------------------------------------------------------------------------

type httpRange struct {
	Start  int64
	End    int64
	Length int64
}

// parseRange parses a "bytes=start-end" Range header and returns the computed
// range bounds. Only a single range is supported (rclone uses single ranges).
func parseRange(rang string, fileSize int64) (*httpRange, error) {
	if rang == "" {
		return nil, fmt.Errorf("empty range")
	}

	if !strings.HasPrefix(rang, "bytes=") {
		return nil, fmt.Errorf("invalid range prefix")
	}

	rangeVal := strings.TrimPrefix(rang, "bytes=")
	parts := strings.SplitN(rangeVal, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid range format")
	}

	startStr := strings.TrimSpace(parts[0])
	endStr := strings.TrimSpace(parts[1])

	var start, end int64

	if startStr == "" {
		// Suffix range: "bytes=-N" means last N bytes.
		suffixSize, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid suffix range: %w", err)
		}
		if suffixSize >= fileSize {
			start = 0
			end = fileSize - 1
		} else {
			start = fileSize - suffixSize
			end = fileSize - 1
		}
	} else {
		var parseErr error
		start, parseErr = strconv.ParseInt(startStr, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid start in range: %w", parseErr)
		}

		if endStr == "" {
			end = fileSize - 1
		} else {
			end, parseErr = strconv.ParseInt(endStr, 10, 64)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid end in range: %w", parseErr)
			}
		}

		if start > end || start < 0 || end >= fileSize {
			return nil, fmt.Errorf("range out of bounds: start=%d end=%d fileSize=%d", start, end, fileSize)
		}
	}

	return &httpRange{
		Start:  start,
		End:    end,
		Length: end - start + 1,
	}, nil
}

// ---------------------------------------------------------------------------
// HEAD handler (same as GET but no body)
// ---------------------------------------------------------------------------

func (s *Server) handleHead(w http.ResponseWriter, r *http.Request) {
	// Resolve virtual path.
	virtualPath := strings.TrimPrefix(r.URL.Path, s.root)
	virtualPath = strings.TrimPrefix(virtualPath, "/")

	if virtualPath == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	slog.Debug("HEAD", "path", virtualPath)

	// Look up the file to get metadata.
	file, err := s.store.GetFileByPath(virtualPath)
	if err != nil {
		slog.Error("HEAD: store lookup failed", "path", virtualPath, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if file == nil {
		// Not a file — head is for files only; return not found.
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	mime := file.MimeType
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	w.Header().Set("Accept-Ranges", "bytes")
	w.WriteHeader(http.StatusOK)
}

// ---------------------------------------------------------------------------
// Directory listing (WebDAV-style Multi-Status for GET on directory paths)
// ---------------------------------------------------------------------------

// serveDirListing responds to a GET request on a virtual directory path with
// a WebDAV Multi-Status XML document listing the directory contents.
// This matches the behaviour of zurg and other standards-compliant WebDAV servers
// so that Chrome and other browsers render a browsable directory listing.
func (s *Server) serveDirListing(w http.ResponseWriter, reqPath string, depth string) {
	slog.Debug("directory listing", "path", reqPath, "depth", depth)

	// Normalise the path.
	normalised := strings.TrimRight(reqPath, "/")
	if normalised == "" {
		normalised = "/"
	}

	// Build the virtual prefix: strip the WebDAV root from the path.
	prefix := strings.TrimPrefix(normalised, s.root)
	prefix = strings.TrimPrefix(prefix, "/")

	// List files from SQLite matching this prefix.
	records, err := s.store.ListDir(prefix)
	if err != nil {
		slog.Error("directory listing: ListDir failed", "prefix", prefix, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Build a set of virtual paths for the response.
	seen := map[string]bool{}
	var responses []response

	// Always include the requested directory itself.
	dirHref := normalised
	if !strings.HasSuffix(dirHref, "/") {
		dirHref += "/"
	}
	responses = appendResponse(responses, dirHref, true, 0, "", "", &seen)

	// Add immediate children based on depth.
	if depth == "1" || depth == "infinity" {
		// Track immediate children of the requested directory.
		type childInfo struct {
			isDir bool
			size  int64
			name  string
			mime  string
		}
		immediate := map[string]childInfo{}

		for _, rec := range records {
			relPath := strings.TrimPrefix(rec.Path, prefix)
			relPath = strings.TrimPrefix(relPath, "/")

			parts := strings.SplitN(relPath, "/", 2)
			immediateName := parts[0]

			if _, exists := immediate[immediateName]; exists {
				continue
			}

			if len(parts) > 1 {
				// The file is nested deeper — the immediate child is a directory.
				immediate[immediateName] = childInfo{isDir: true}
			} else {
				// Direct file in the requested directory.
				mime := rec.MimeType
				if mime == "" {
					mime = "application/octet-stream"
				}
				immediate[immediateName] = childInfo{
					isDir: false,
					size:  rec.Size,
					name:  rec.Name,
					mime:  mime,
				}
			}
		}

		// Build response entries from the immediate children map.
		baseHref := strings.TrimRight(normalised, "/") + "/"
		for name, info := range immediate {
			childHref := baseHref + name
			if info.isDir {
				childHref += "/"
				responses = appendResponse(responses, childHref, true, 0, "", "", &seen)
			} else {
				responses = appendResponse(responses, childHref, false, info.size, info.name, info.mime, &seen)
			}
		}
	}

	// Build the XML response.
	ms := multiStatus{
		XmlnsD:    davNamespace,
		Responses: responses,
	}

	output, err := xml.MarshalIndent(ms, "", "  ")
	if err != nil {
		slog.Error("directory listing: XML marshal failed", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Prepend XML declaration.
	body := append([]byte(xml.Header), output...)

	w.Header().Set("DAV", "1")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	w.Write(body)
}
