package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store defines the persistence interface for notifications.
type Store interface {
	Create(ctx context.Context, n *Notification) error
	ListByUser(ctx context.Context, userID uuid.UUID, limit int, cursor *time.Time) ([]*Notification, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	GetPreferences(ctx context.Context, userID uuid.UUID) (*UserPreferences, error)
	UpdatePreferences(ctx context.Context, prefs *UserPreferences) error
}

// PgStore implements Store backed by PostgreSQL.
type PgStore struct {
	pool *pgxpool.Pool
}

// NewPgStore creates a new PostgreSQL-backed notification store.
func NewPgStore(pool *pgxpool.Pool) *PgStore {
	return &PgStore{pool: pool}
}

// Create inserts a new notification.
func (s *PgStore) Create(ctx context.Context, n *Notification) error {
	payloadJSON, err := json.Marshal(n.Payload)
	if err != nil {
		payloadJSON = []byte("{}")
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO notifications (id, user_id, type, priority, title, body, session_id, satellite_id, payload, read, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		n.ID, n.UserID, n.Type, n.Priority, n.Title, n.Body, n.SessionID, n.SatelliteID, payloadJSON, n.Read, n.CreatedAt,
	)
	return err
}

// ListByUser returns notifications for a user, newest first, with cursor-based pagination.
func (s *PgStore) ListByUser(ctx context.Context, userID uuid.UUID, limit int, cursor *time.Time) ([]*Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var rows interface{ Next() bool }
	var err error

	if cursor != nil {
		rows, err = s.pool.Query(ctx,
			`SELECT id, user_id, type, priority, title, body, session_id, satellite_id, payload, read, created_at
			 FROM notifications
			 WHERE user_id = $1 AND created_at < $2
			 ORDER BY created_at DESC
			 LIMIT $3`,
			userID, cursor, limit,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id, user_id, type, priority, title, body, session_id, satellite_id, payload, read, created_at
			 FROM notifications
			 WHERE user_id = $1
			 ORDER BY created_at DESC
			 LIMIT $2`,
			userID, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("query notifications: %w", err)
	}

	// Type-assert to get Rows interface with Scan/Close
	pgRows := rows.(interface {
		Next() bool
		Scan(dest ...any) error
		Close()
	})
	defer pgRows.Close()

	var notifications []*Notification
	for pgRows.Next() {
		n := &Notification{}
		var payloadJSON []byte
		if err := pgRows.Scan(
			&n.ID, &n.UserID, &n.Type, &n.Priority, &n.Title, &n.Body,
			&n.SessionID, &n.SatelliteID, &payloadJSON, &n.Read, &n.CreatedAt,
		); err != nil {
			slog.Error(fmt.Sprintf("Notifications: scan error: %v", err), "component", "notification")
			continue
		}
		if len(payloadJSON) > 0 {
			json.Unmarshal(payloadJSON, &n.Payload)
		}
		notifications = append(notifications, n)
	}

	return notifications, nil
}

// CountUnread returns the count of unread notifications for a user.
func (s *PgStore) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read = FALSE`,
		userID,
	).Scan(&count)
	return count, err
}

// MarkRead marks a single notification as read.
func (s *PgStore) MarkRead(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE notifications SET read = TRUE WHERE id = $1`,
		id,
	)
	return err
}

// MarkAllRead marks all of a user's notifications as read.
func (s *PgStore) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE notifications SET read = TRUE WHERE user_id = $1 AND read = FALSE`,
		userID,
	)
	return err
}

// GetPreferences retrieves notification preferences for a user.
// Returns nil, nil if no preferences are stored.
func (s *PgStore) GetPreferences(ctx context.Context, userID uuid.UUID) (*UserPreferences, error) {
	p := &UserPreferences{}
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, min_priority, browser_enabled, session_terminated, session_error, satellite_offline, session_suspended
		 FROM notification_preferences
		 WHERE user_id = $1`,
		userID,
	).Scan(&p.UserID, &p.MinPriority, &p.BrowserEnabled, &p.SessionTerminated, &p.SessionError, &p.SatelliteOffline, &p.SessionSuspended)

	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// UpdatePreferences creates or updates notification preferences for a user.
func (s *PgStore) UpdatePreferences(ctx context.Context, prefs *UserPreferences) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO notification_preferences (user_id, min_priority, browser_enabled, session_terminated, session_error, satellite_offline, session_suspended, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		 ON CONFLICT (user_id) DO UPDATE SET
		   min_priority = EXCLUDED.min_priority,
		   browser_enabled = EXCLUDED.browser_enabled,
		   session_terminated = EXCLUDED.session_terminated,
		   session_error = EXCLUDED.session_error,
		   satellite_offline = EXCLUDED.satellite_offline,
		   session_suspended = EXCLUDED.session_suspended,
		   updated_at = NOW()`,
		prefs.UserID, prefs.MinPriority, prefs.BrowserEnabled, prefs.SessionTerminated, prefs.SessionError, prefs.SatelliteOffline, prefs.SessionSuspended,
	)
	return err
}
