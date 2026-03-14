package audit

import (
	"context"
	"log/slog"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/daao/nexus/internal/auth"
	"github.com/daao/nexus/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contextKey string

const ipContextKey contextKey = "client_ip"

// WithClientIP stores the client IP in the context.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, ipContextKey, ip)
}

// ClientIPFromContext retrieves the client IP from the context.
func ClientIPFromContext(ctx context.Context) string {
	ip, ok := ctx.Value(ipContextKey).(string)
	if !ok {
		return ""
	}
	return ip
}

// ClientIPFromRequest extracts the client IP from an HTTP request.
// Checks X-Forwarded-For first, then X-Real-IP, then RemoteAddr.
func ClientIPFromRequest(r *http.Request) string {
	// Check X-Forwarded-For header first
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		// X-Forwarded-For may contain multiple IPs, take the first one
		ips := strings.Split(xForwardedFor, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	xRealIP := r.Header.Get("X-Real-IP")
	if xRealIP != "" {
		return strings.TrimSpace(xRealIP)
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

type AuditLogger struct {
	pool *pgxpool.Pool
}

func NewAuditLogger(pool *pgxpool.Pool) *AuditLogger {
	return &AuditLogger{pool: pool}
}

// Log records an audit event asynchronously.
// It extracts the actor from auth.UserFromContext and the IP from context.
// This function is fire-and-forget: errors are logged but never returned.
func (l *AuditLogger) Log(ctx context.Context, action, resourceType, resourceID string, details map[string]interface{}) {
	// Nil-safe check
	if l == nil || l.pool == nil {
		return
	}

	// Extract user from context
	var actorID *uuid.UUID
	var actorEmail string

	user, ok := auth.UserFromContext(ctx)
	if ok && user != nil {
		id, err := uuid.Parse(user.ID)
		if err == nil {
			actorID = &id
		}
		actorEmail = user.Email
	} else {
		// No user in context, use system
		actorEmail = "system"
	}

	// Extract IP from context
	ip := ClientIPFromContext(ctx)
	var ipAddress *string
	if ip != "" {
		ipAddress = &ip
	}

	// Build the audit log entry
	entry := &database.AuditLogEntry{
		ActorID:      actorID,
		ActorEmail:   actorEmail,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   &resourceID,
		Details:      details,
		IPAddress:    ipAddress,
		CreatedAt:    time.Now(),
	}

	// Write to DB asynchronously using a goroutine
	pool := l.pool
	go func() {
		// Use a fresh context for the async operation
		asyncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := database.WriteAuditLog(asyncCtx, pool, entry)
		if err != nil {
			slog.Error(fmt.Sprintf("ERROR: failed to write audit log: %v", err), "component", "audit")
		}
	}()
}
