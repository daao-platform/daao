package api

import (
	"net/http"
	"testing"
)

func TestParsePaginationParams(t *testing.T) {
	tests := []struct {
		name           string
		queryString    string
		expectedCursor string
		expectedLimit  int
	}{
		{
			name:           "no params - use defaults",
			queryString:    "",
			expectedCursor: "",
			expectedLimit:  DefaultLimit,
		},
		{
			name:           "with cursor",
			queryString:    "cursor=abc123",
			expectedCursor: "abc123",
			expectedLimit:  DefaultLimit,
		},
		{
			name:           "with limit",
			queryString:    "limit=25",
			expectedCursor: "",
			expectedLimit:  25,
		},
		{
			name:           "with cursor and limit",
			queryString:    "cursor=abc123&limit=25",
			expectedCursor: "abc123",
			expectedLimit:  25,
		},
		{
			name:           "limit exceeds max",
			queryString:    "limit=500",
			expectedCursor: "",
			expectedLimit:  MaxLimit,
		},
		{
			name:           "invalid limit - use default",
			queryString:    "limit=abc",
			expectedCursor: "",
			expectedLimit:  DefaultLimit,
		},
		{
			name:           "zero limit - use default",
			queryString:    "limit=0",
			expectedCursor: "",
			expectedLimit:  DefaultLimit,
		},
		{
			name:           "negative limit - use default",
			queryString:    "limit=-1",
			expectedCursor: "",
			expectedLimit:  DefaultLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/?"+tt.queryString, nil)
			params := ParsePaginationParams(r)

			if params.Cursor != tt.expectedCursor {
				t.Errorf("expected cursor %q, got %q", tt.expectedCursor, params.Cursor)
			}
			if params.Limit != tt.expectedLimit {
				t.Errorf("expected limit %d, got %d", tt.expectedLimit, params.Limit)
			}
		})
	}
}

func TestCursorPaginate(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		items := []interface{}{}
		resp := CursorPaginate(items, 0, false)
		if resp.Count != 0 {
			t.Errorf("expected count 0, got %d", resp.Count)
		}
		if resp.NextCursor != nil {
			t.Errorf("expected nil next_cursor, got %v", *resp.NextCursor)
		}
	})

	t.Run("slice with items no more", func(t *testing.T) {
		items := []interface{}{"a", "b", "c"}
		resp := CursorPaginate(items, 3, false)
		if resp.Count != 3 {
			t.Errorf("expected count 3, got %d", resp.Count)
		}
		if resp.NextCursor != nil {
			t.Errorf("expected nil next_cursor when hasMore=false, got %v", *resp.NextCursor)
		}
	})

	t.Run("slice with items has more", func(t *testing.T) {
		items := []interface{}{"a", "b", "c"}
		resp := CursorPaginate(items, 100, true)
		if resp.Count != 3 {
			t.Errorf("expected count 3, got %d", resp.Count)
		}
		if resp.NextCursor == nil {
			t.Errorf("expected next_cursor when hasMore=true, got nil")
		}
		if resp.NextCursor != nil && *resp.NextCursor != "c" {
			t.Errorf("expected next_cursor 'c', got %v", *resp.NextCursor)
		}
	})
}

func TestConstants(t *testing.T) {
	if DefaultLimit != 50 {
		t.Errorf("expected DefaultLimit to be 50, got %d", DefaultLimit)
	}
	if MaxLimit != 200 {
		t.Errorf("expected MaxLimit to be 200, got %d", MaxLimit)
	}
}
