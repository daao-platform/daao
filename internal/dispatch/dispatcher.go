package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// Error Definitions
// ============================================================================

var ErrNoSatelliteAvailable = errors.New("no satellite matches dispatch requirements")

// ============================================================================
// Type Definitions
// ============================================================================

// DispatchOptions specifies criteria for selecting a satellite.
type DispatchOptions struct {
	Mode        string   // Dispatch mode: "" (manual) or "auto-dispatch"
	RequireTags []string // Satellite must have all of these tags
	PreferTags []string  // Satellite gets bonus points for these tags
	RequireGPU bool      // Satellite must have GPU data (future use)
}

// DispatchResult contains the result of a dispatch operation.
type DispatchResult struct {
	SatelliteID uuid.UUID
	Score       float64
	MatchedTags []string
	Dispatched  bool
}

// SatelliteCandidate represents a satellite that can be considered for dispatch.
type SatelliteCandidate struct {
	SatelliteID uuid.UUID
	Name        string
	Status      string
	Tags        []string
	CPUPercent  float64
	MemPercent  float64
	DiskPercent float64
	GPUData     json.RawMessage
	TelemetryAt time.Time
	HasStream   bool
	Score       float64
	MatchedTags []string
}

// ============================================================================
// StreamChecker Interface
// ============================================================================

// StreamChecker defines an interface for checking if a satellite has an active stream.
// This allows the dispatcher to depend on the stream package without a direct import.
type StreamChecker interface {
	HasStream(satelliteID uuid.UUID) bool
}

// ============================================================================
// Dispatcher
// ============================================================================

// Dispatcher handles satellite dispatching based on resource availability and tags.
type Dispatcher struct {
	dbPool         *pgxpool.Pool
	streamRegistry StreamChecker
}

// NewDispatcher creates a new Dispatcher with the given database pool and stream checker.
func NewDispatcher(dbPool *pgxpool.Pool, streamChecker StreamChecker) *Dispatcher {
	return &Dispatcher{
		dbPool:         dbPool,
		streamRegistry: streamChecker,
	}
}

// Dispatch selects the best satellite for an agent deployment.
// It filters by status=active, has gRPC stream, telemetry <60s old, matches required tags.
// It scores using: 0.5*(1-cpu) + 0.3*(1-mem) + 0.2*(1-disk), with preferred tags adding 0.05 bonus each.
// Returns the highest-scoring satellite.
func (d *Dispatcher) Dispatch(ctx context.Context, opts DispatchOptions) (*DispatchResult, error) {
	candidates, err := d.candidates(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get candidates: %w", err)
	}

	if len(candidates) == 0 {
		return nil, ErrNoSatelliteAvailable
	}

	// Sort by score descending and take the first
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	best := candidates[0]
	return &DispatchResult{
		SatelliteID: best.SatelliteID,
		Score:       best.Score,
		MatchedTags: best.MatchedTags,
		Dispatched:  true,
	}, nil
}

// Preview returns all candidates with scores, without selecting one.
// Results are sorted by score in descending order.
func (d *Dispatcher) Preview(ctx context.Context, opts DispatchOptions) ([]DispatchResult, error) {
	candidates, err := d.candidates(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get candidates: %w", err)
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	results := make([]DispatchResult, len(candidates))
	for i, c := range candidates {
		results[i] = DispatchResult{
			SatelliteID: c.SatelliteID,
			Score:       c.Score,
			MatchedTags: c.MatchedTags,
			Dispatched:  false,
		}
	}
	return results, nil
}

// candidates fetches and filters satellites, computing scores for each.
func (d *Dispatcher) candidates(ctx context.Context, opts DispatchOptions) ([]SatelliteCandidate, error) {
	// Query active satellites with their latest telemetry and tags
	query := `
		SELECT s.id, s.name, s.status, s.tags,
		       t.cpu_percent, t.memory_percent, t.disk_percent, t.gpu_data, t.created_at AS telemetry_at
		FROM satellites s
		LEFT JOIN LATERAL (
		    SELECT cpu_percent, memory_percent, disk_percent, gpu_data, created_at
		    FROM satellite_telemetry
		    WHERE satellite_id = s.id
		    ORDER BY created_at DESC LIMIT 1
		) t ON TRUE
		WHERE s.status = 'active'
	`

	rows, err := d.dbPool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query satellites: %w", err)
	}
	defer rows.Close()

	var candidates []SatelliteCandidate
	now := time.Now()

	for rows.Next() {
		var c SatelliteCandidate
		var tags []string
		var cpu, mem, disk *float64
		var gpuData []byte
		var telemetryAt *time.Time

		err := rows.Scan(
			&c.SatelliteID,
			&c.Name,
			&c.Status,
			&tags,
			&cpu,
			&mem,
			&disk,
			&gpuData,
			&telemetryAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		c.Tags = tags
		if cpu != nil {
			c.CPUPercent = *cpu
		}
		if mem != nil {
			c.MemPercent = *mem
		}
		if disk != nil {
			c.DiskPercent = *disk
		}
		if gpuData != nil {
			c.GPUData = gpuData
		}
		if telemetryAt != nil {
			c.TelemetryAt = *telemetryAt
		}

		// Check stream availability
		if d.streamRegistry != nil {
			c.HasStream = d.streamRegistry.HasStream(c.SatelliteID)
		}

		// Apply filters
		if !d.filterCandidate(&c, opts, now) {
			continue
		}

		// Compute score
		matchedTags := computeMatchedTags(c.Tags, opts.PreferTags)
		c.Score = scoreSatellite(c, opts.PreferTags)
		c.MatchedTags = matchedTags

		candidates = append(candidates, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return candidates, nil
}

// filterCandidate applies all filtering rules to determine if a candidate is valid.
func (d *Dispatcher) filterCandidate(c *SatelliteCandidate, opts DispatchOptions, now time.Time) bool {
	// Filter: status must be active (already in query WHERE clause)

	// Filter: must have gRPC stream
	if !c.HasStream {
		return false
	}

	// Filter: telemetry must be less than 60 seconds old
	if now.Sub(c.TelemetryAt) > 60*time.Second {
		return false
	}

	// Filter: must have all required tags
	if !hasAllRequiredTags(c.Tags, opts.RequireTags) {
		return false
	}

	return true
}

// hasAllRequiredTags checks if candidate has all required tags.
func hasAllRequiredTags(candidateTags, requiredTags []string) bool {
	if len(requiredTags) == 0 {
		return true
	}

	tagSet := make(map[string]bool)
	for _, t := range candidateTags {
		tagSet[t] = true
	}

	for _, required := range requiredTags {
		if !tagSet[required] {
			return false
		}
	}
	return true
}

// computeMatchedTags returns the list of preferred tags that the candidate has.
func computeMatchedTags(candidateTags, preferTags []string) []string {
	if len(preferTags) == 0 {
		return nil
	}

	tagSet := make(map[string]bool)
	for _, t := range candidateTags {
		tagSet[t] = true
	}

	var matched []string
	for _, preferred := range preferTags {
		if tagSet[preferred] {
			matched = append(matched, preferred)
		}
	}
	return matched
}

// scoreSatellite computes the dispatch score for a candidate.
// Formula: 0.5*(1-cpu%) + 0.3*(1-mem%) + 0.2*(1-disk%)
// Preferred tags add 0.05 bonus per matched tag.
func scoreSatellite(candidate SatelliteCandidate, preferTags []string) float64 {
	// Convert percentages to 0-1 range (they come as 0-100)
	cpuScore := 1.0 - (candidate.CPUPercent / 100.0)
	memScore := 1.0 - (candidate.MemPercent / 100.0)
	diskScore := 1.0 - (candidate.DiskPercent / 100.0)

	// Base score formula
	baseScore := 0.5*cpuScore + 0.3*memScore + 0.2*diskScore

	// Add bonus for preferred tags
	matchedCount := countMatchedTags(candidate.Tags, preferTags)
	tagBonus := float64(matchedCount) * 0.05

	return baseScore + tagBonus
}

// countMatchedTags returns the count of preferred tags that the candidate has.
func countMatchedTags(candidateTags, preferTags []string) int {
	if len(preferTags) == 0 {
		return 0
	}

	tagSet := make(map[string]bool)
	for _, t := range candidateTags {
		tagSet[t] = true
	}

	count := 0
	for _, preferred := range preferTags {
		if tagSet[preferred] {
			count++
		}
	}
	return count
}
