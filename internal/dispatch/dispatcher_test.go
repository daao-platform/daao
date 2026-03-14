package dispatch

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestScoreSatellite(t *testing.T) {
	tests := []struct {
		name       string
		candidate  SatelliteCandidate
		preferTags []string
		wantMin    float64
		wantMax    float64
	}{
		{
			name: "empty resources gives high score",
			candidate: SatelliteCandidate{
				CPUPercent:  0,
				MemPercent:  0,
				DiskPercent: 0,
				Tags:        []string{"linux"},
			},
			preferTags: nil,
			wantMin:    1.0,
			wantMax:    1.0,
		},
		{
			name: "full resources gives low score",
			candidate: SatelliteCandidate{
				CPUPercent:  100,
				MemPercent:  100,
				DiskPercent: 100,
				Tags:        []string{},
			},
			preferTags: nil,
			wantMin:    0.0,
			wantMax:    0.0,
		},
		{
			name: "partial resources gives medium score",
			candidate: SatelliteCandidate{
				CPUPercent:  50,
				MemPercent:  50,
				DiskPercent: 50,
				Tags:        []string{},
			},
			preferTags: nil,
			wantMin:    0.5,
			wantMax:    0.5,
		},
		{
			name: "preferred tags add bonus",
			candidate: SatelliteCandidate{
				CPUPercent:  0,
				MemPercent:  0,
				DiskPercent: 0,
				Tags:        []string{"linux", "gpu"},
			},
			preferTags: []string{"linux", "gpu"},
			wantMin:    1.1, // 1.0 + 0.05*2
			wantMax:    1.1,
		},
		{
			name: "partial preferred tags add partial bonus",
			candidate: SatelliteCandidate{
				CPUPercent:  0,
				MemPercent:  0,
				DiskPercent: 0,
				Tags:        []string{"linux"},
			},
			preferTags: []string{"linux", "gpu"},
			wantMin:    1.05, // 1.0 + 0.05*1
			wantMax:    1.05,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreSatellite(tt.candidate, tt.preferTags)
			assert.GreaterOrEqual(t, got, tt.wantMin)
			assert.LessOrEqual(t, got, tt.wantMax)
		})
	}
}

func TestHasAllRequiredTags(t *testing.T) {
	tests := []struct {
		name          string
		candidateTags []string
		requireTags   []string
		want          bool
	}{
		{
			name:          "empty require tags returns true",
			candidateTags: []string{"linux", "gpu"},
			requireTags:   []string{},
			want:          true,
		},
		{
			name:          "all required tags present",
			candidateTags: []string{"linux", "gpu", "production"},
			requireTags:   []string{"linux", "gpu"},
			want:          true,
		},
		{
			name:          "missing required tag",
			candidateTags: []string{"linux"},
			requireTags:   []string{"linux", "gpu"},
			want:          false,
		},
		{
			name:          "empty candidate tags with required",
			candidateTags: []string{},
			requireTags:   []string{"linux"},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasAllRequiredTags(tt.candidateTags, tt.requireTags)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCountMatchedTags(t *testing.T) {
	tests := []struct {
		name          string
		candidateTags []string
		preferTags    []string
		want          int
	}{
		{
			name:          "all preferred tags match",
			candidateTags: []string{"linux", "gpu"},
			preferTags:    []string{"linux", "gpu"},
			want:          2,
		},
		{
			name:          "some preferred tags match",
			candidateTags: []string{"linux"},
			preferTags:    []string{"linux", "gpu"},
			want:          1,
		},
		{
			name:          "no preferred tags match",
			candidateTags: []string{"windows"},
			preferTags:    []string{"linux", "gpu"},
			want:          0,
		},
		{
			name:          "empty prefer tags",
			candidateTags: []string{"linux"},
			preferTags:    []string{},
			want:          0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countMatchedTags(tt.candidateTags, tt.preferTags)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDispatchOptionsJSON(t *testing.T) {
	opts := DispatchOptions{
		RequireTags: []string{"linux", "production"},
		PreferTags:  []string{"gpu"},
		RequireGPU:  true,
	}

	data, err := json.Marshal(opts)
	assert.NoError(t, err)

	var decoded DispatchOptions
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, opts.RequireTags, decoded.RequireTags)
	assert.Equal(t, opts.PreferTags, decoded.PreferTags)
	assert.Equal(t, opts.RequireGPU, decoded.RequireGPU)
}

func TestDispatchResultJSON(t *testing.T) {
	result := DispatchResult{
		SatelliteID: uuid.New(),
		Score:       0.85,
		MatchedTags: []string{"linux"},
		Dispatched:  true,
	}

	data, err := json.Marshal(result)
	assert.NoError(t, err)

	var decoded DispatchResult
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, result.SatelliteID, decoded.SatelliteID)
	assert.Equal(t, result.Score, decoded.Score)
	assert.Equal(t, result.MatchedTags, decoded.MatchedTags)
	assert.Equal(t, result.Dispatched, decoded.Dispatched)
}

func TestSatelliteCandidateJSON(t *testing.T) {
	candidate := SatelliteCandidate{
		SatelliteID: uuid.New(),
		Name:        "test-satellite",
		Status:      "active",
		Tags:        []string{"linux", "gpu"},
		CPUPercent:  25.5,
		MemPercent:  40.0,
		DiskPercent: 60.0,
		GPUData:     json.RawMessage(`{"available": true}`),
		HasStream:   true,
	}

	data, err := json.Marshal(candidate)
	assert.NoError(t, err)

	var decoded SatelliteCandidate
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, candidate.SatelliteID, decoded.SatelliteID)
	assert.Equal(t, candidate.Name, decoded.Name)
	assert.Equal(t, candidate.Status, decoded.Status)
	assert.Equal(t, candidate.Tags, decoded.Tags)
	assert.Equal(t, candidate.CPUPercent, decoded.CPUPercent)
	assert.Equal(t, candidate.HasStream, decoded.HasStream)
}

func TestComputeMatchedTags(t *testing.T) {
	tests := []struct {
		name          string
		candidateTags []string
		preferTags    []string
		want          []string
	}{
		{
			name:          "returns matched preferred tags",
			candidateTags: []string{"linux", "gpu"},
			preferTags:    []string{"linux", "gpu", "production"},
			want:          []string{"linux", "gpu"},
		},
		{
			name:          "returns empty when no matches",
			candidateTags: []string{"windows"},
			preferTags:    []string{"linux", "gpu"},
			want:          nil,
		},
		{
			name:          "returns empty for empty prefer tags",
			candidateTags: []string{"linux"},
			preferTags:    []string{},
			want:          nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeMatchedTags(tt.candidateTags, tt.preferTags)
			assert.Equal(t, tt.want, got)
		})
	}
}
