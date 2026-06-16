// WebDAV HTTP browser — serves HTML directory listings at /http/.
//
// This provides a human-browsable HTML view of the virtual filesystem,
// similar to zurg's /http/ endpoint. It reuses the same SQLite metadata
// store so there's zero TorBox API overhead for browsing.
package server

import (
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/ben/warpbox/internal/library"
)

// handleHTTP serves an HTML directory listing at /http/,
// or streams file content directly if the path resolves to a file.
func (s *Server) handleHTTP(w http.ResponseWriter, r *http.Request) {
	// Resolve the virtual path.
	reqPath := strings.TrimRight(r.URL.Path, "/")
	if reqPath == "" || strings.Count(reqPath, "/") < 2 {
		reqPath = "/http/"
	}

	rawVirtualPath := strings.TrimPrefix(reqPath, "/http")
	rawVirtualPath = strings.TrimPrefix(rawVirtualPath, "/")

	// Detect virtual path mounts from the first segment after /http/.
	firstSeg := rawVirtualPath
	if idx := strings.IndexByte(rawVirtualPath, '/'); idx >= 0 {
		firstSeg = rawVirtualPath[:idx]
	}

	var hFilter *library.Filter
	var virtualPath = rawVirtualPath

	if firstSeg == "__all__" {
		virtualPath = strings.TrimPrefix(rawVirtualPath, "__all__")
		virtualPath = strings.TrimPrefix(virtualPath, "/")
	} else if f, ok := s.virtualPathMap[firstSeg]; ok {
		hFilter = f
		virtualPath = strings.TrimPrefix(rawVirtualPath, firstSeg)
		virtualPath = strings.TrimPrefix(virtualPath, "/")
	}

	// Check if this path resolves to a file first.
	file, err := s.store.GetFileByPath(virtualPath)
	if err != nil {
		slog.Error("HTTP: store lookup failed", "path", virtualPath, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if file != nil {
		// File found — stream it through the CDN proxy pipeline.
		slog.Debug("HTTP: streaming file", "path", virtualPath, "size", file.Size)
		s.streamFileContent(w, r, file)
		return
	}

	// Not a file — serve HTML directory listing.
	records, err := s.store.ListDir(virtualPath)
	if err != nil {
		slog.Error("HTTP browser: ListDir failed", "prefix", virtualPath, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Apply filter if inside a virtual path.
	if hFilter != nil {
		records = hFilter.Apply(records)
	}

	// Build the breadcrumb trail.
	parts := strings.Split(rawVirtualPath, "/")
	var breadcrumbs []breadcrumb
	breadcrumbs = append(breadcrumbs, breadcrumb{Name: "root", Href: "/http/"})
	accum := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		accum += "/" + p
		breadcrumbs = append(breadcrumbs, breadcrumb{Name: p, Href: "/http" + accum + "/"})
	}

	// Determine href prefix for virtual mounts.
	mountPrefix := ""
	if hFilter != nil {
		mountPrefix = "/" + firstSeg
	} else if firstSeg == "__all__" {
		mountPrefix = "/__all__"
	}

	// Parse sort parameter (default: name).
	sortBy := strings.ToLower(r.URL.Query().Get("sort"))
	if sortBy != "size" {
		sortBy = "name"
	}

	// Build the directory listing with folder size accumulation.
	type dirAgg struct {
		name      string
		href      string
		totalSize int64
	}
	var dirs []entry
	var files []entry
	dirMap := map[string]*dirAgg{}
	dirOrder := []string{}

	// At the root level with virtual paths configured, show synthetic dirs.
	if rawVirtualPath == "" && len(s.virtualFilters) > 0 {
		// Compute total sizes for each virtual path (matching only, not largest).
		var allTotal int64
		filterTotals := make(map[int]int64, len(s.virtualFilters))
		for _, rec := range records {
			allTotal += rec.Size
			for i, vf := range s.virtualFilters {
				dir := library.ExtractDirectory(rec.Path)
				if !vf.MatchDirectory(dir) {
					continue
				}
				rel := library.ExtractRelativePath(rec.Path)
				if !vf.MatchFile(rel) {
					continue
				}
				filterTotals[i] += rec.Size
			}
		}
		dirs = append(dirs, entry{Name: "__all__/", Href: "/http/__all__/", Size: allTotal, IsDir: true})
		for i, vf := range s.virtualFilters {
			name := strings.TrimPrefix(vf.Mount, "/")
			dirs = append(dirs, entry{Name: name + "/", Href: "/http/" + name + "/", Size: filterTotals[i], IsDir: true})
		}
	} else {
		for _, rec := range records {
		rel := strings.TrimPrefix(rec.Path, virtualPath)
		rel = strings.TrimPrefix(rel, "/")

		firstSlash := strings.Index(rel, "/")
		var displayName string
		var href string

		if firstSlash >= 0 {
			displayName = rel[:firstSlash]
			if ag, ok := dirMap[displayName]; ok {
				ag.totalSize += rec.Size
				continue
			}
			if virtualPath == "" {
				href = "/http" + mountPrefix + "/" + displayName + "/"
			} else {
				href = "/http" + mountPrefix + "/" + virtualPath + "/" + displayName + "/"
			}
			dirMap[displayName] = &dirAgg{name: displayName, href: href, totalSize: rec.Size}
			dirOrder = append(dirOrder, displayName)
			continue
		} else {
			displayName = rel
			if virtualPath == "" {
				href = "/http" + mountPrefix + "/" + rel
			} else {
				href = "/http" + mountPrefix + "/" + virtualPath + "/" + rel
			}
		}

		mime := rec.MimeType
			if mime == "" {
				mime = "application/octet-stream"
			}
			fileHref := "/http" + mountPrefix + "/" + rec.Path
			files = append(files, entry{
				Name:   displayName,
				Href:   fileHref,
				Size:   rec.Size,
				IsDir:  false,
				Mime:   mime,
			})
		}
	}

	// Build dirs from accumulated map, sorted.
	for _, name := range dirOrder {
		ag := dirMap[name]
		dirs = append(dirs, entry{Name: ag.name + "/", Href: ag.href, Size: ag.totalSize, IsDir: true})
	}

	// Sort directories and files by the requested column.
	sort.Slice(dirs, func(i, j int) bool {
		if sortBy == "size" {
			if dirs[i].Size != dirs[j].Size {
				return dirs[i].Size < dirs[j].Size
			}
			return dirs[i].Name < dirs[j].Name
		}
		return dirs[i].Name < dirs[j].Name
	})
	sort.Slice(files, func(i, j int) bool {
		if sortBy == "size" {
			if files[i].Size != files[j].Size {
				return files[i].Size < files[j].Size
			}
			return files[i].Name < files[j].Name
		}
		return files[i].Name < files[j].Name
	})

	// Render the HTML page.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, htmlPageStart)
	fmt.Fprintf(w, "<title>warpbox /http/%s</title></head><body>\n", html.EscapeString(virtualPath))
	fmt.Fprint(w, "<div class=\"container\">\n")
	fmt.Fprint(w, "<h1>warpbox <span class=\"path\">/http/</span></h1>\n")
	fmt.Fprint(w, "<p class=\"nav\"><a href=\"/\">Back to status</a></p>\n")

	// Breadcrumbs.
	fmt.Fprint(w, "<p class=\"breadcrumbs\">")
	for i, crumb := range breadcrumbs {
		if i > 0 {
			fmt.Fprint(w, " / ")
		}
		if i == len(breadcrumbs)-1 {
			fmt.Fprintf(w, "<span class=\"current\">%s</span>", html.EscapeString(crumb.Name))
		} else {
			fmt.Fprintf(w, "<a href=\"%s\">%s</a>", html.EscapeString(crumb.Href), html.EscapeString(crumb.Name))
		}
	}
	fmt.Fprint(w, "</p>\n")

	fmt.Fprint(w, "<table>\n")
	fmt.Fprint(w, "<tr><th>Name</th><th>Size</th><th>Type</th></tr>\n")

	for _, d := range dirs {
		sizeStr := formatSize(d.Size)
		if d.Size == 0 {
			sizeStr = "—"
		}
		fmt.Fprintf(w, "<tr><td class=\"dir\"><a href=\"%s\">📁 %s</a></td><td>%s</td><td>directory</td></tr>\n",
			html.EscapeString(d.Href), html.EscapeString(d.Name), sizeStr)
	}
	for _, f := range files {
		sizeStr := formatSize(f.Size)
		fmt.Fprintf(w, "<tr><td><a href=\"%s\">%s</a></td><td>%s</td><td>%s</td></tr>\n",
			html.EscapeString(f.Href), html.EscapeString(f.Name), sizeStr, html.EscapeString(f.Mime))
	}

	fmt.Fprint(w, "</table>\n")
	fmt.Fprint(w, "</div>\n")
	fmt.Fprintf(w, "<div class=\"footer\">warpbox %s — <a href=\"/\">status</a></div>\n", s.cfg.Version)
	fmt.Fprint(w, "</body></html>\n")
}

// breadcrumb represents a single level in the breadcrumb trail.
type breadcrumb struct {
	Name string
	Href string
}

// entry represents a directory entry in the HTML listing.
type entry struct {
	Name  string
	Href  string
	Size  int64
	IsDir bool
	Mime  string
}

// htmlPageStart is the common HTML head sent before the page-specific title.
const htmlPageStart = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    background: #0f172a;
    color: #e2e8f0;
    padding: 2rem 1rem;
  }
  .container { max-width: 900px; margin: 0 auto; }
  h1 { font-size: 1.5rem; color: #38bdf8; margin-bottom: 0.5rem; }
  h1 .path { color: #94a3b8; font-weight: 400; }
  .nav { margin-bottom: 0.5rem; font-size: 0.85rem; }
  .nav a { color: #38bdf8; text-decoration: none; }
  .nav a:hover { text-decoration: underline; }
  .breadcrumbs { font-size: 0.85rem; margin-bottom: 1rem; color: #64748b; }
  .breadcrumbs a { color: #38bdf8; text-decoration: none; }
  .breadcrumbs a:hover { text-decoration: underline; }
  .breadcrumbs .current { color: #e2e8f0; }
  table { width: 100%; border-collapse: collapse; }
  th {
    background: #1e293b;
    color: #38bdf8;
    padding: 0.5rem 1rem;
    text-align: left;
    font-size: 0.85rem;
    font-weight: 600;
    border-bottom: 2px solid #334155;
  }
  td {
    padding: 0.4rem 1rem;
    border-bottom: 1px solid #1e293b;
    font-size: 0.85rem;
  }
  td:first-child { width: 50%; }
  td:nth-child(2) { width: 15%; color: #94a3b8; }
  td:nth-child(3) { width: 35%; color: #94a3b8; font-size: 0.8rem; }
  .dir a { color: #38bdf8; font-weight: 500; text-decoration: none; }
  .dir a:hover { text-decoration: underline; }
  a { color: #e2e8f0; text-decoration: none; }
  a:hover { text-decoration: underline; color: #38bdf8; }
  .footer {
    text-align: center;
    margin-top: 2rem;
    font-size: 0.8rem;
    color: #475569;
  }
  .footer a { color: #64748b; text-decoration: none; }
  .footer a:hover { color: #94a3b8; }
</style>
`

// formatSize returns a human-readable file size.
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}