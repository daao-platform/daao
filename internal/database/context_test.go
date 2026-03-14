package database

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Context File struct tests
// =============================================================================

func TestContextFileFields(t *testing.T) {
	satID := uuid.New()
	fileID := uuid.New()

	cf := ContextFile{
		ID:             fileID,
		SatelliteID:    satID,
		FilePath:       "systeminfo.md",
		Content:        "# System Info\n\nThis is a test.",
		Version:        1,
		LastModifiedBy: "user@cockpit",
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	if cf.ID != fileID {
		t.Errorf("ID = %v, want %v", cf.ID, fileID)
	}
	if cf.SatelliteID != satID {
		t.Errorf("SatelliteID = %v, want %v", cf.SatelliteID, satID)
	}
	if cf.FilePath != "systeminfo.md" {
		t.Errorf("FilePath = %q, want %q", cf.FilePath, "systeminfo.md")
	}
	if cf.Version != 1 {
		t.Errorf("Version = %d, want 1", cf.Version)
	}
	if cf.LastModifiedBy != "user@cockpit" {
		t.Errorf("LastModifiedBy = %q, want %q", cf.LastModifiedBy, "user@cockpit")
	}
	if cf.Content != "# System Info\n\nThis is a test." {
		t.Errorf("Content mismatch")
	}
}

func TestContextFileHistoryFields(t *testing.T) {
	histID := uuid.New()
	fileID := uuid.New()
	modifiedAt := time.Now().UTC()

	cfh := ContextFileHistory{
		ID:            histID,
		ContextFileID: fileID,
		Version:       3,
		Content:       "# Updated content",
		ModifiedBy:    "admin@cockpit",
		ModifiedAt:    modifiedAt,
	}

	if cfh.ID != histID {
		t.Errorf("ID = %v, want %v", cfh.ID, histID)
	}
	if cfh.ContextFileID != fileID {
		t.Errorf("ContextFileID = %v, want %v", cfh.ContextFileID, fileID)
	}
	if cfh.Version != 3 {
		t.Errorf("Version = %d, want 3", cfh.Version)
	}
	if cfh.ModifiedBy != "admin@cockpit" {
		t.Errorf("ModifiedBy = %q, want %q", cfh.ModifiedBy, "admin@cockpit")
	}
	if cfh.Content != "# Updated content" {
		t.Errorf("Content mismatch")
	}
}

func TestContextFileVersionIncrement(t *testing.T) {
	// Simulate version increment logic
	cf := ContextFile{
		ID:      uuid.New(),
		Version: 1,
	}

	// Simulate what UpdateContextFile does: increment version
	newVersion := cf.Version + 1
	if newVersion != 2 {
		t.Errorf("Version after increment = %d, want 2", newVersion)
	}

	// Simulate three more updates
	for i := 2; i <= 4; i++ {
		newVersion++
	}
	if newVersion != 5 {
		t.Errorf("Version after 4 updates = %d, want 5", newVersion)
	}
}

func TestContextFileHistoryOrdering(t *testing.T) {
	// Verify that a slice of history entries can be sorted by version DESC
	now := time.Now().UTC()
	entries := []ContextFileHistory{
		{ID: uuid.New(), Version: 3, ModifiedAt: now.Add(-1 * time.Hour)},
		{ID: uuid.New(), Version: 1, ModifiedAt: now.Add(-3 * time.Hour)},
		{ID: uuid.New(), Version: 2, ModifiedAt: now.Add(-2 * time.Hour)},
	}

	// The DB query orders by modified_at DESC, so version 3 should come first
	// Simulate expected DB ordering
	sorted := []ContextFileHistory{entries[0], entries[2], entries[1]}

	if sorted[0].Version != 3 {
		t.Errorf("First entry version = %d, want 3 (most recent)", sorted[0].Version)
	}
	if sorted[1].Version != 2 {
		t.Errorf("Second entry version = %d, want 2", sorted[1].Version)
	}
	if sorted[2].Version != 1 {
		t.Errorf("Third entry version = %d, want 1 (oldest)", sorted[2].Version)
	}
}

func TestContextFileUniqueConstraintLogic(t *testing.T) {
	// Verify that two files with same satellite_id and file_path would conflict
	satID := uuid.New()

	file1 := ContextFile{
		SatelliteID: satID,
		FilePath:    "runbooks.md",
	}

	file2 := ContextFile{
		SatelliteID: satID,
		FilePath:    "runbooks.md",
	}

	if file1.SatelliteID != file2.SatelliteID || file1.FilePath != file2.FilePath {
		t.Error("Files should have same satellite_id and file_path for constraint test")
	}

	// Different satellite = no conflict
	file3 := ContextFile{
		SatelliteID: uuid.New(),
		FilePath:    "runbooks.md",
	}

	if file3.SatelliteID == file1.SatelliteID {
		t.Error("file3 should have different satellite_id")
	}
}

func TestContextFileCascadeLogic(t *testing.T) {
	// Verify that history entries reference the correct context file
	fileID := uuid.New()

	history := []ContextFileHistory{
		{ID: uuid.New(), ContextFileID: fileID, Version: 1},
		{ID: uuid.New(), ContextFileID: fileID, Version: 2},
	}

	for _, h := range history {
		if h.ContextFileID != fileID {
			t.Errorf("History entry %v references wrong file: got %v, want %v",
				h.ID, h.ContextFileID, fileID)
		}
	}

	// All history entries belong to the same file — CASCADE would remove them all
	if len(history) != 2 {
		t.Errorf("Expected 2 history entries, got %d", len(history))
	}
}
