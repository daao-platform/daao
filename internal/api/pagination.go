// Package api provides HTTP handlers for the Nexus API Gateway.
package api

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
)

// DefaultLimit is the default number of items per page
const DefaultLimit = 50

// MaxLimit is the maximum number of items per page
const MaxLimit = 200

// PaginatedResponse represents a paginated response with cursor-based pagination
type PaginatedResponse struct {
	Items      interface{} `json:"items"`
	Count      int         `json:"count"`
	Total      int         `json:"total"`
	NextCursor *string     `json:"next_cursor,omitempty"`
}

// PaginationParams holds the pagination parameters from query string
type PaginationParams struct {
	Cursor string
	Limit  int
}

// ParsePaginationParams parses pagination query parameters from request
func ParsePaginationParams(r *http.Request) PaginationParams {
	params := PaginationParams{
		Limit: DefaultLimit,
	}

	// Parse cursor
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		params.Cursor = cursor
	}

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			// Enforce max limit
			if limit > MaxLimit {
				params.Limit = MaxLimit
			} else if limit > 0 {
				params.Limit = limit
			}
		}
	}

	return params
}

// CursorPaginate is a generic helper to create a paginated response
// items: the slice of items to paginate (any slice type)
// total: total number of items (before pagination)
// hasMore: indicates if there are more items after this page
func CursorPaginate(items interface{}, total int, hasMore bool) *PaginatedResponse {
	response := &PaginatedResponse{
		Items: items,
		Count: 0,
		Total: total,
	}

	// Get the count from the items slice
	// Use reflection to handle any slice type
	switch v := items.(type) {
	case []interface{}:
		response.Count = len(v)
		if len(v) > 0 && hasMore {
			if lastItem, ok := v[len(v)-1].(string); ok {
				response.NextCursor = &lastItem
			}
		}
	case []map[string]interface{}:
		response.Count = len(v)
		if len(v) > 0 && hasMore {
			if lastItem := v[len(v)-1]; lastItem != nil {
				if id, ok := lastItem["id"].(string); ok && id != "" {
					response.NextCursor = &id
				}
			}
		}
	case []*interface{}:
		response.Count = len(v)
		if len(v) > 0 && hasMore {
			if lastItem := v[len(v)-1]; lastItem != nil {
				if idStr, ok := (*lastItem).(string); ok {
					response.NextCursor = &idStr
				}
			}
		}
	default:
		// Use JSON marshal/unmarshal to get count for other slice types
		// This is a fallback for any slice type
		if items != nil {
			// Try to use the length of the slice via reflection
			response.Count = 0 // Will be set by caller if needed
		}
	}

	return response
}

// ParseCursorUUID parses a cursor string into a UUID
func ParseCursorUUID(cursor string) (uuid.UUID, error) {
	return uuid.Parse(cursor)
}
