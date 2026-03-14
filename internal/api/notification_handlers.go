package api

import (
	"encoding/json"
	"log/slog"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/daao/nexus/internal/notification"
	"github.com/google/uuid"
)

// HandleListNotifications returns paginated notifications for the current user.
// GET /api/v1/notifications?limit=N&cursor=RFC3339
func (h *Handlers) HandleListNotifications(w http.ResponseWriter, r *http.Request) {
	if h.notificationStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notifications not configured"})
		return
	}

	userID := h.getCurrentUserID()

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var cursor *time.Time
	if c := r.URL.Query().Get("cursor"); c != "" {
		if t, err := time.Parse(time.RFC3339Nano, c); err == nil {
			cursor = &t
		}
	}

	notifications, err := h.notificationStore.ListByUser(r.Context(), userID, limit, cursor)
	if err != nil {
		slog.Info(fmt.Sprintf("ListNotifications: %v", err), "component", "api")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list notifications"})
		return
	}
	if notifications == nil {
		notifications = []*notification.Notification{}
	}

	// Build next cursor from the last notification's timestamp
	var nextCursor string
	if len(notifications) == limit {
		nextCursor = notifications[len(notifications)-1].CreatedAt.Format(time.RFC3339Nano)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": notifications,
		"count":         len(notifications),
		"next_cursor":   nextCursor,
	})
}

// HandleUnreadCount returns the count of unread notifications.
// GET /api/v1/notifications/unread-count
func (h *Handlers) HandleUnreadCount(w http.ResponseWriter, r *http.Request) {
	if h.notificationStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notifications not configured"})
		return
	}

	userID := h.getCurrentUserID()

	count, err := h.notificationStore.CountUnread(r.Context(), userID)
	if err != nil {
		slog.Info(fmt.Sprintf("UnreadCount: %v", err), "component", "api")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to count unread"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"count": count})
}

// HandleMarkRead marks a single notification as read.
// PATCH /api/v1/notifications/{id}/read
func (h *Handlers) HandleMarkRead(w http.ResponseWriter, r *http.Request, notifID string) {
	if h.notificationStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notifications not configured"})
		return
	}

	id, err := uuid.Parse(notifID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid notification ID"})
		return
	}

	if err := h.notificationStore.MarkRead(r.Context(), id); err != nil {
		slog.Info(fmt.Sprintf("MarkRead: %v", err), "component", "api")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to mark read"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleMarkAllRead marks all of the user's notifications as read.
// POST /api/v1/notifications/read-all
func (h *Handlers) HandleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	if h.notificationStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notifications not configured"})
		return
	}

	userID := h.getCurrentUserID()

	if err := h.notificationStore.MarkAllRead(r.Context(), userID); err != nil {
		slog.Info(fmt.Sprintf("MarkAllRead: %v", err), "component", "api")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to mark all read"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleGetPreferences returns the user's notification preferences.
// GET /api/v1/notifications/preferences
func (h *Handlers) HandleGetPreferences(w http.ResponseWriter, r *http.Request) {
	if h.notificationStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notifications not configured"})
		return
	}

	userID := h.getCurrentUserID()

	prefs, err := h.notificationStore.GetPreferences(r.Context(), userID)
	if err != nil {
		slog.Info(fmt.Sprintf("GetPreferences: %v", err), "component", "api")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get preferences"})
		return
	}

	if prefs == nil {
		prefs = notification.DefaultPreferences(userID)
	}

	writeJSON(w, http.StatusOK, prefs)
}

// HandleUpdatePreferences updates the user's notification preferences.
// PUT /api/v1/notifications/preferences
func (h *Handlers) HandleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	if h.notificationStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notifications not configured"})
		return
	}

	userID := h.getCurrentUserID()

	var req notification.UserPreferences
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.UserID = userID

	// Validate min_priority
	switch req.MinPriority {
	case notification.PriorityInfo, notification.PriorityWarning, notification.PriorityCritical:
		// valid
	default:
		req.MinPriority = notification.PriorityInfo
	}

	if err := h.notificationStore.UpdatePreferences(r.Context(), &req); err != nil {
		slog.Info(fmt.Sprintf("UpdatePreferences: %v", err), "component", "api")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update preferences"})
		return
	}

	writeJSON(w, http.StatusOK, &req)
}

// getCurrentUserID returns the current user ID.
// In v1 without multi-user auth, returns the default user.
// When #23 (multi-user) lands, extract from JWT claims in request context.
func (h *Handlers) getCurrentUserID() uuid.UUID {
	return uuid.MustParse("00000000-0000-0000-0000-000000000000")
}
