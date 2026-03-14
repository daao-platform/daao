package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound   = errors.New("user not found")
	ErrDuplicateEmail = errors.New("email already exists")
)

// ValidRoles contains all valid user roles
var ValidRoles = []string{"owner", "admin", "viewer"}

// User represents a user in the database
type User struct {
	ID           uuid.UUID  `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	Role         string     `json:"role"`
	Provider     string     `json:"provider,omitempty"`
	ProviderID   string     `json:"provider_id,omitempty"`
	AvatarURL    string     `json:"avatar_url,omitempty"`
	PasswordHash *string    `json:"-"` // Never exposed in JSON
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// UserStore provides database operations for user records
type UserStore struct {
	dbPool *pgxpool.Pool
}

// NewUserStore creates a new UserStore with the given connection pool
func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{
		dbPool: pool,
	}
}

// isValidRole checks if the given role is valid
func isValidRole(role string) bool {
	for _, r := range ValidRoles {
		if role == r {
			return true
		}
	}
	return false
}

// userColumns is the canonical column list for user queries
const userColumns = `id, email, name, role, provider, provider_id, avatar_url, password_hash, last_login_at, created_at, updated_at`

// scanUser scans a user row into a User struct
func scanUser(row interface{ Scan(dest ...interface{}) error }) (*User, error) {
	var u User
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.Name,
		&u.Role,
		&u.Provider,
		&u.ProviderID,
		&u.AvatarURL,
		&u.PasswordHash,
		&u.LastLoginAt,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByID retrieves a user by their UUID
// Returns ErrUserNotFound if the user doesn't exist
func (s *UserStore) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	query := `
		SELECT ` + userColumns + `
		FROM users
		WHERE id = $1
	`
	u, err := scanUser(s.dbPool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}

// GetByEmail retrieves a user by their email address
// Returns ErrUserNotFound if no user with that email exists
func (s *UserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT ` + userColumns + `
		FROM users
		WHERE email = $1
	`
	u, err := scanUser(s.dbPool.QueryRow(ctx, query, email))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}

// GetByProviderID retrieves a user by their OAuth provider and provider-specific ID
// Returns ErrUserNotFound if no user with that provider ID combination exists
func (s *UserStore) GetByProviderID(ctx context.Context, provider, providerID string) (*User, error) {
	query := `
		SELECT ` + userColumns + `
		FROM users
		WHERE provider = $1 AND provider_id = $2
	`
	u, err := scanUser(s.dbPool.QueryRow(ctx, query, provider, providerID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return u, nil
}

// List retrieves all users ordered by created_at DESC
func (s *UserStore) List(ctx context.Context) ([]*User, error) {
	query := `
		SELECT ` + userColumns + `
		FROM users
		ORDER BY created_at DESC
	`
	rows, err := s.dbPool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// Create inserts a new user into the database
// Returns ErrDuplicateEmail if a user with the same email already exists
func (s *UserStore) Create(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (id, email, name, role)
		VALUES (gen_random_uuid(), $1, $2, $3)
		RETURNING id, email, name, role, provider, provider_id, avatar_url, last_login_at, created_at, updated_at
	`
	_, err := s.dbPool.Exec(ctx, query, user.Email, user.Name, user.Role)
	if err != nil {
		// Check for unique constraint violation on email
		if isUniqueConstraintViolation(err) {
			return ErrDuplicateEmail
		}
		return err
	}
	return nil
}

// isUniqueConstraintViolation checks if the error is a unique constraint violation
func isUniqueConstraintViolation(err error) bool {
	// Check for unique constraint violation (error code 23505)
	// In pgx v5, we can check the error message for the constraint violation
	return strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "23505")
}

// UpdateRole updates a user's role
// Returns ErrUserNotFound if the user doesn't exist
// Returns an error if the role is not valid
func (s *UserStore) UpdateRole(ctx context.Context, id uuid.UUID, role string) error {
	// Validate role first
	if !isValidRole(role) {
		return errors.New("invalid role: must be one of 'owner', 'admin', 'viewer'")
	}

	query := `
		UPDATE users
		SET role = $1, updated_at = NOW()
		WHERE id = $2
	`
	result, err := s.dbPool.Exec(ctx, query, role, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdateLastLogin updates a user's last login timestamp
// Returns ErrUserNotFound if the user doesn't exist
func (s *UserStore) UpdateLastLogin(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE users
		SET last_login_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`
	result, err := s.dbPool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// Delete removes a user from the database (hard delete)
// Returns ErrUserNotFound if the user doesn't exist
func (s *UserStore) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM users WHERE id = $1`
	result, err := s.dbPool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpsertFromOIDC creates or updates a user from OIDC authentication
// If the user exists by provider+provider_id, updates their email, name, avatar, and last_login_at
// If not found, creates a new user with role='viewer'
func (s *UserStore) UpsertFromOIDC(ctx context.Context, provider, providerID, email, name, avatarURL string) (*User, error) {
	// First try to find existing user by provider ID
	existing, err := s.GetByProviderID(ctx, provider, providerID)
	if err != nil && !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}

	if existing != nil {
		// User exists, update their info
		query := `
			UPDATE users
			SET email = $1, name = $2, avatar_url = $3, last_login_at = NOW(), updated_at = NOW()
			WHERE provider = $4 AND provider_id = $5
			RETURNING id, email, name, role, provider, provider_id, avatar_url, last_login_at, created_at, updated_at
		`
		u, err := scanUser(s.dbPool.QueryRow(ctx, query, email, name, avatarURL, provider, providerID))
		if err != nil {
			return nil, err
		}
		return u, nil
	}

	// User doesn't exist, create new user with role='viewer'
	query := `
		INSERT INTO users (id, email, name, role, provider, provider_id, avatar_url, last_login_at)
		VALUES (gen_random_uuid(), $1, $2, 'viewer', $3, $4, $5, NOW())
		RETURNING id, email, name, role, provider, provider_id, avatar_url, last_login_at, created_at, updated_at
	`
	u, err := scanUser(s.dbPool.QueryRow(ctx, query, email, name, provider, providerID, avatarURL))
	if err != nil {
		// Check for unique constraint violation on email
		if isUniqueConstraintViolation(err) {
			return nil, ErrDuplicateEmail
		}
		return nil, err
	}
	return u, nil
}

// ============================================================================
// Password Operations (Local Auth)
// ============================================================================

// HashPassword generates a bcrypt hash of the given password.
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", fmt.Errorf("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// SetPassword updates the password hash for a user.
func (s *UserStore) SetPassword(ctx context.Context, userID uuid.UUID, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	query := `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`
	result, err := s.dbPool.Exec(ctx, query, hash, userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// VerifyPassword checks email + password, returns the user if valid.
// Also updates last_login_at on success.
func (s *UserStore) VerifyPassword(ctx context.Context, email, password string) (*User, error) {
	user, err := s.GetByEmail(ctx, email)
	if err != nil {
		return nil, ErrUserNotFound
	}

	if user.PasswordHash == nil || *user.PasswordHash == "" {
		return nil, fmt.Errorf("password not set for this account")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid password")
	}

	// Update last login timestamp
	_ = s.UpdateLastLogin(ctx, user.ID)

	return user, nil
}

// CreateWithPassword inserts a new user with a pre-hashed password.
func (s *UserStore) CreateWithPassword(ctx context.Context, email, name, role, passwordHash string) (*User, error) {
	query := `
		INSERT INTO users (id, email, name, role, password_hash, provider, provider_id, avatar_url)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, 'local', $1, '')
		ON CONFLICT (email) DO UPDATE SET password_hash = $4, provider = 'local', provider_id = $1, updated_at = NOW()
		RETURNING ` + userColumns + `
	`
	u, err := scanUser(s.dbPool.QueryRow(ctx, query, email, name, role, passwordHash))
	if err != nil {
		return nil, fmt.Errorf("failed to create user with password: %w", err)
	}
	return u, nil
}

