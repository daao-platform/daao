// Package hitl provides Human-in-the-Loop guardrails for enterprise customers.
//
// This is a stub implementation for the public repository. Enterprise
// customers receive the full implementation with their license.
package hitl

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/daao/nexus/internal/notification"
	"github.com/daao/nexus/internal/stream"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrEnterpriseRequired is returned when HITL features are used without license.
var ErrEnterpriseRequired = errors.New("enterprise license required: HITL guardrails — upgrade at https://daao.io")

// ProposalStatus represents the lifecycle state of a proposal.
type ProposalStatus string

const (
	StatusPending  ProposalStatus = "PENDING"
	StatusApproved ProposalStatus = "APPROVED"
	StatusDenied   ProposalStatus = "DENIED"
	StatusExpired  ProposalStatus = "EXPIRED"
)

// Proposal represents a human-in-the-loop approval request from an agent.
type Proposal struct {
	ID          uuid.UUID       `json:"id"`
	SessionID   uuid.UUID       `json:"session_id"`
	AgentName   string          `json:"agent_name"`
	Action      string          `json:"action"`
	Description string          `json:"description"`
	RiskLevel   string          `json:"risk_level"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Status      ProposalStatus  `json:"status"`
	ReviewedBy  *string         `json:"reviewed_by,omitempty"`
	ReviewedAt  *time.Time      `json:"reviewed_at,omitempty"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// Store provides persistence for HITL proposals.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new HITL proposal store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Create(_ context.Context, _ *Proposal) error { return ErrEnterpriseRequired }
func (s *Store) Get(_ context.Context, _ uuid.UUID) (*Proposal, error) {
	return nil, ErrEnterpriseRequired
}
func (s *Store) List(_ context.Context, _ *ProposalStatus, _ int) ([]*Proposal, error) {
	return nil, ErrEnterpriseRequired
}
func (s *Store) CountPending(_ context.Context) (int, error) { return 0, ErrEnterpriseRequired }
func (s *Store) UpdateStatus(_ context.Context, _ uuid.UUID, _ ProposalStatus, _ string) error {
	return ErrEnterpriseRequired
}

// Manager orchestrates the HITL approval flow.
type Manager struct {
	store    *Store
	streams  stream.StreamRegistryInterface
	eventBus *notification.EventBus
}

// NewManager creates a new HITL manager.
func NewManager(store *Store, streams stream.StreamRegistryInterface, eventBus *notification.EventBus) *Manager {
	return &Manager{store: store, streams: streams, eventBus: eventBus}
}

func (m *Manager) SubmitProposal(_ context.Context, _ *Proposal) error {
	return ErrEnterpriseRequired
}
func (m *Manager) Approve(_ context.Context, _ uuid.UUID, _ string) error {
	return ErrEnterpriseRequired
}
func (m *Manager) Deny(_ context.Context, _ uuid.UUID, _ string) error {
	return ErrEnterpriseRequired
}
func (m *Manager) Store() *Store { return m.store }
