package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Baseline represents a single satellite baseline snapshot as stored in the DB.
type Baseline struct {
	ID           uuid.UUID       `json:"id"`
	SatelliteID  uuid.UUID       `json:"satellite_id"`
	Tier         int             `json:"tier"`
	SnapshotType string          `json:"snapshot_type"`
	SnapshotData json.RawMessage `json:"snapshot_data"`
	AgentRunID   *uuid.UUID      `json:"agent_run_id,omitempty"`
	IsBaseline   bool            `json:"is_baseline"`
	DriftScore   *float64        `json:"drift_score,omitempty"`
	DriftSummary json.RawMessage `json:"drift_summary,omitempty"`
	CollectedAt  time.Time       `json:"collected_at"`
	CreatedAt    time.Time       `json:"created_at"`
}

// SatelliteProfile represents the machine classification derived from discovery.
type SatelliteProfile struct {
	SatelliteID        uuid.UUID  `json:"satellite_id"`
	OSFamily           string     `json:"os_family"`
	OSDistro           *string    `json:"os_distro,omitempty"`
	OSVersion          *string    `json:"os_version,omitempty"`
	Arch               *string    `json:"arch,omitempty"`
	MachineRoles       []string   `json:"machine_roles"`
	DetectedServices   []string   `json:"detected_services"`
	DetectedContainers []string   `json:"detected_containers"`
	ListeningPorts     []int      `json:"listening_ports"`
	RiskLevel          string     `json:"risk_level"`
	RecommendedAgents  []string   `json:"recommended_agents"`
	LastDiscovery      *time.Time `json:"last_discovery,omitempty"`
	LastPulse          *time.Time `json:"last_pulse,omitempty"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// BaselinesHandler provides HTTP handlers for baseline and profile API operations.
type BaselinesHandler struct {
	dbPool *pgxpool.Pool
}

// NewBaselinesHandler creates a new BaselinesHandler instance.
func NewBaselinesHandler(pool *pgxpool.Pool) *BaselinesHandler {
	return &BaselinesHandler{dbPool: pool}
}

// HandleBaselinesAPI handles all baseline API requests.
// Routes:
//
//	GET /api/v1/satellites/{id}/baselines          - list baselines
//	GET /api/v1/satellites/{id}/baselines/latest   - get latest per type
//	GET /api/v1/satellites/{id}/baselines/golden   - get golden baselines
func (h *BaselinesHandler) HandleBaselinesAPI(w http.ResponseWriter, r *http.Request) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	p := strings.TrimRight(r.URL.Path, "/")
	if !strings.HasPrefix(p, "/api/v1/satellites/") {
		http.NotFound(w, r)
		return
	}

	remainder := strings.TrimPrefix(p, "/api/v1/satellites/")
	parts := strings.SplitN(remainder, "/", 3) // satelliteID / baselines / subpath
	if len(parts) < 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	satelliteID, err := uuid.Parse(parts[0])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid satellite ID")
		return
	}

	subPath := ""
	if len(parts) > 2 {
		subPath = parts[2]
	}

	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch subPath {
	case "latest":
		h.handleGetLatestBaselines(w, r, satelliteID)
	case "golden":
		h.handleGetGoldenBaselines(w, r, satelliteID)
	default:
		h.handleListBaselines(w, r, satelliteID)
	}
}

// HandleProfileAPI handles satellite profile API requests.
// Route: GET /api/v1/satellites/{id}/profile
func (h *BaselinesHandler) HandleProfileAPI(w http.ResponseWriter, r *http.Request) {
	if h.dbPool == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	p := strings.TrimRight(r.URL.Path, "/")
	remainder := strings.TrimPrefix(p, "/api/v1/satellites/")
	parts := strings.SplitN(remainder, "/", 2) // satelliteID / profile
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	satelliteID, err := uuid.Parse(parts[0])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid satellite ID")
		return
	}

	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	h.handleGetSatelliteProfile(w, r, satelliteID)
}

// handleListBaselines returns all baselines for a satellite.
// GET /api/v1/satellites/:id/baselines?type=os_profile&tier=4&limit=50
func (h *BaselinesHandler) handleListBaselines(w http.ResponseWriter, r *http.Request, satelliteID uuid.UUID) {
	snapshotType := r.URL.Query().Get("type")
	tierStr := r.URL.Query().Get("tier")
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
			limit = l
		}
	}

	query := `SELECT id, satellite_id, tier, snapshot_type, snapshot_data, agent_run_id,
	                 is_baseline, drift_score, drift_summary, collected_at, created_at
	          FROM satellite_baselines
	          WHERE satellite_id = $1`
	args := []interface{}{satelliteID}
	argN := 2

	if snapshotType != "" {
		query += fmt.Sprintf(" AND snapshot_type = $%d", argN)
		args = append(args, snapshotType)
		argN++
	}
	if tierStr != "" {
		if tier, err := strconv.Atoi(tierStr); err == nil {
			query += fmt.Sprintf(" AND tier = $%d", argN)
			args = append(args, tier)
			argN++
		}
	}

	query += fmt.Sprintf(" ORDER BY collected_at DESC LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := h.dbPool.Query(r.Context(), query, args...)
	if err != nil {
		slog.Info(fmt.Sprintf("ListBaselines: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to list baselines")
		return
	}
	defer rows.Close()

	baselines := []Baseline{}
	for rows.Next() {
		var b Baseline
		if err := rows.Scan(&b.ID, &b.SatelliteID, &b.Tier, &b.SnapshotType, &b.SnapshotData,
			&b.AgentRunID, &b.IsBaseline, &b.DriftScore, &b.DriftSummary, &b.CollectedAt, &b.CreatedAt); err != nil {
			slog.Info(fmt.Sprintf("ScanBaseline: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to scan baseline")
			return
		}
		baselines = append(baselines, b)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"baselines": baselines,
		"count":     len(baselines),
	})
}

// handleGetLatestBaselines returns the most recent baseline for each snapshot type.
func (h *BaselinesHandler) handleGetLatestBaselines(w http.ResponseWriter, r *http.Request, satelliteID uuid.UUID) {
	query := `SELECT DISTINCT ON (snapshot_type)
	                 id, satellite_id, tier, snapshot_type, snapshot_data, agent_run_id,
	                 is_baseline, drift_score, drift_summary, collected_at, created_at
	          FROM satellite_baselines
	          WHERE satellite_id = $1
	          ORDER BY snapshot_type, collected_at DESC`

	rows, err := h.dbPool.Query(r.Context(), query, satelliteID)
	if err != nil {
		slog.Info(fmt.Sprintf("LatestBaselines: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to query latest baselines")
		return
	}
	defer rows.Close()

	baselines := []Baseline{}
	for rows.Next() {
		var b Baseline
		if err := rows.Scan(&b.ID, &b.SatelliteID, &b.Tier, &b.SnapshotType, &b.SnapshotData,
			&b.AgentRunID, &b.IsBaseline, &b.DriftScore, &b.DriftSummary, &b.CollectedAt, &b.CreatedAt); err != nil {
			slog.Info(fmt.Sprintf("ScanBaseline: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to scan baseline")
			return
		}
		baselines = append(baselines, b)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"baselines": baselines,
		"count":     len(baselines),
	})
}

// handleGetGoldenBaselines returns only the marked golden baselines.
func (h *BaselinesHandler) handleGetGoldenBaselines(w http.ResponseWriter, r *http.Request, satelliteID uuid.UUID) {
	query := `SELECT id, satellite_id, tier, snapshot_type, snapshot_data, agent_run_id,
	                 is_baseline, drift_score, drift_summary, collected_at, created_at
	          FROM satellite_baselines
	          WHERE satellite_id = $1 AND is_baseline = TRUE
	          ORDER BY snapshot_type, collected_at DESC`

	rows, err := h.dbPool.Query(r.Context(), query, satelliteID)
	if err != nil {
		slog.Info(fmt.Sprintf("GoldenBaselines: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to query golden baselines")
		return
	}
	defer rows.Close()

	baselines := []Baseline{}
	for rows.Next() {
		var b Baseline
		if err := rows.Scan(&b.ID, &b.SatelliteID, &b.Tier, &b.SnapshotType, &b.SnapshotData,
			&b.AgentRunID, &b.IsBaseline, &b.DriftScore, &b.DriftSummary, &b.CollectedAt, &b.CreatedAt); err != nil {
			slog.Info(fmt.Sprintf("ScanBaseline: %v", err), "component", "api")
			writeJSONError(w, http.StatusInternalServerError, "failed to scan baseline")
			return
		}
		baselines = append(baselines, b)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"baselines": baselines,
		"count":     len(baselines),
	})
}

// handleGetSatelliteProfile returns the machine classification profile.
func (h *BaselinesHandler) handleGetSatelliteProfile(w http.ResponseWriter, r *http.Request, satelliteID uuid.UUID) {
	var p SatelliteProfile
	err := h.dbPool.QueryRow(r.Context(),
		`SELECT satellite_id, os_family, os_distro, os_version, arch,
		        machine_roles, detected_services, detected_containers,
		        listening_ports, risk_level, recommended_agents,
		        last_discovery, last_pulse, updated_at
		 FROM satellite_profiles
		 WHERE satellite_id = $1`, satelliteID).Scan(
		&p.SatelliteID, &p.OSFamily, &p.OSDistro, &p.OSVersion, &p.Arch,
		&p.MachineRoles, &p.DetectedServices, &p.DetectedContainers,
		&p.ListeningPorts, &p.RiskLevel, &p.RecommendedAgents,
		&p.LastDiscovery, &p.LastPulse, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"profile": nil,
			"message": "no discovery has been run on this satellite yet",
		})
		return
	}
	if err != nil {
		slog.Info(fmt.Sprintf("GetSatelliteProfile: %v", err), "component", "api")
		writeJSONError(w, http.StatusInternalServerError, "failed to get satellite profile")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"profile": p,
	})
}
