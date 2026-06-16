// Package metadata provides a SQLite-backed store for the TorBox directory tree.
//
// This file adds the stats table used for time-series metrics on the landing page.
package metadata

import (
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// StatsRecord represents a single data point in the stats time-series table.
type StatsRecord struct {
	Timestamp time.Time
	Metric    string
	Value     float64
}

// RecordStats batch-inserts a map of metric → value at the current timestamp.
func (s *Store) RecordStats(metrics map[string]float64) error {
	start := time.Now()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin stats tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	stmt, err := tx.Prepare(`INSERT INTO stats (timestamp, metric, value) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare stats insert: %w", err)
	}
	defer stmt.Close()

	for metric, value := range metrics {
		if _, err := stmt.Exec(now, metric, value); err != nil {
			return fmt.Errorf("insert stats %q: %w", metric, err)
		}
	}

	err = tx.Commit()
	slog.Debug("db write duration", "method", "RecordStats", "duration_ms", time.Since(start).Milliseconds(), "metrics", len(metrics), "error", err)
	return err
}

// QueryStats returns time-ordered values for a given metric since the specified time.
func (s *Store) QueryStats(metric string, since time.Time) ([]StatsRecord, error) {
	rows, err := s.db.Query(`
		SELECT timestamp, metric, value FROM stats
		WHERE metric = ? AND timestamp >= ?
		ORDER BY timestamp ASC
	`, metric, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("query stats %q: %w", metric, err)
	}
	defer rows.Close()

	var records []StatsRecord
	for rows.Next() {
		var r StatsRecord
		var ts string
		if err := rows.Scan(&ts, &r.Metric, &r.Value); err != nil {
			return nil, fmt.Errorf("scan stats row: %w", err)
		}
		if t, parseErr := time.Parse("2006-01-02 15:04:05", ts); parseErr != nil {
			slog.Debug("stats: failed to parse timestamp", "ts", ts, "error", parseErr)
		} else {
			r.Timestamp = t
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// QueryAllStatsSince returns all metrics since the given time, grouped by metric.
func (s *Store) QueryAllStatsSince(since time.Time) (map[string][]StatsRecord, error) {
	rows, err := s.db.Query(`
		SELECT timestamp, metric, value FROM stats
		WHERE timestamp >= ?
		ORDER BY metric, timestamp ASC
	`, since.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("query all stats: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]StatsRecord)
	for rows.Next() {
		var r StatsRecord
		var ts string
		if err := rows.Scan(&ts, &r.Metric, &r.Value); err != nil {
			return nil, fmt.Errorf("scan stats row: %w", err)
		}
		if t, parseErr := time.Parse("2006-01-02 15:04:05", ts); parseErr != nil {
			slog.Debug("stats: failed to parse timestamp", "ts", ts, "error", parseErr)
		} else {
			r.Timestamp = t
		}
		result[r.Metric] = append(result[r.Metric], r)
	}
	return result, rows.Err()
}

// PruneStats deletes all stats rows older than the given duration.
// Returns the number of rows deleted.
func (s *Store) PruneStats(retention time.Duration) (int, error) {
	start := time.Now()
	cutoff := time.Now().UTC().Add(-retention).Format("2006-01-02 15:04:05")
	result, err := s.db.Exec(`DELETE FROM stats WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune stats: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected after prune: %w", err)
	}
	slog.Debug("db write duration", "method", "PruneStats", "duration_ms", time.Since(start).Milliseconds(), "rows", int(n), "error", err)
	return int(n), nil
}

// GetMetricValuesSince returns just the numeric values for a metric since a time,
// which is the most the landing page needs for a sparkline.
func (s *Store) GetMetricValuesSince(metric string, since time.Time) ([]float64, error) {
	records, err := s.QueryStats(metric, since)
	if err != nil {
		return nil, err
	}
	vals := make([]float64, len(records))
	for i, r := range records {
		vals[i] = r.Value
	}
	return vals, nil
}

// GetAllMetricValuesSince returns a map of metric name → float64 slice.
func (s *Store) GetAllMetricValuesSince(since time.Time) (map[string][]float64, error) {
	grouped, err := s.QueryAllStatsSince(since)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]float64, len(grouped))
	// Sort metrics for deterministic ordering.
	metrics := make([]string, 0, len(grouped))
	for m := range grouped {
		metrics = append(metrics, m)
	}
	sort.Strings(metrics)
	for _, m := range metrics {
		vals := make([]float64, len(grouped[m]))
		for i, r := range grouped[m] {
			vals[i] = r.Value
		}
		result[m] = vals
	}
	return result, nil
}