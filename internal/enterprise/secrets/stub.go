// Package secrets provides enterprise secret backend implementations.
//
// This is a stub implementation for the public repository. Enterprise
// customers receive the full implementation with their license.
package secrets

import (
	"context"
	"errors"
	"time"

	"github.com/daao/nexus/internal/license"
)

// ErrEnterpriseRequired is returned when vault integrations are used without license.
var ErrEnterpriseRequired = errors.New("enterprise license required: vault integrations — upgrade at https://daao.io")

// ---------------------------------------------------------------------------
// HashiCorp Vault (stub)
// ---------------------------------------------------------------------------

// VaultConfig contains configuration for HashiCorp Vault backend.
type VaultConfig struct {
	Address         string
	Token           string
	AppRoleID       string
	AppRoleSecretID string
	Namespace       string
	PathPrefix      string
	DefaultTTL      int
	MaxTTL          int
}

// VaultBackend implements SecretBackend for HashiCorp Vault.
type VaultBackend struct {
	config         *VaultConfig
	licenseManager *license.Manager
}

// NewVaultBackend creates a new HashiCorp Vault backend.
func NewVaultBackend(_ *VaultConfig, _ *license.Manager) (*VaultBackend, error) {
	return nil, ErrEnterpriseRequired
}

func (v *VaultBackend) FetchSecret(_ context.Context, _ string) (string, error) {
	return "", ErrEnterpriseRequired
}
func (v *VaultBackend) RenewLease(_ context.Context, _ string) error        { return ErrEnterpriseRequired }
func (v *VaultBackend) RevokeLease(_ context.Context, _ string) error       { return ErrEnterpriseRequired }
func (v *VaultBackend) RevokeAllLeases(_ context.Context) error             { return ErrEnterpriseRequired }
func (v *VaultBackend) StartAutoRenewal(_ context.Context, _ time.Duration) {}
func (v *VaultBackend) GetActiveLeases() []*leaseEntry                      { return nil }

type leaseEntry struct {
	LeaseID   string
	Renewable bool
	TTL       time.Duration
	ExpiresAt time.Time
	Path      string
}

// ---------------------------------------------------------------------------
// Azure Key Vault (stub)
// ---------------------------------------------------------------------------

// AzureAuthMethod defines the authentication method for Azure Key Vault.
type AzureAuthMethod string

const (
	AzureAuthManagedIdentity  AzureAuthMethod = "managed_identity"
	AzureAuthServicePrincipal AzureAuthMethod = "service_principal"
)

// AzureKeyVaultConfig contains configuration for Azure Key Vault backend.
type AzureKeyVaultConfig struct {
	VaultURL     string
	AuthMethod   AzureAuthMethod
	TenantID     string
	ClientID     string
	ClientSecret string
	UseMSI       bool
}

// AzureKeyVaultBackend implements SecretBackend for Azure Key Vault.
type AzureKeyVaultBackend struct {
	config         *AzureKeyVaultConfig
	licenseManager *license.Manager
}

// NewAzureKeyVaultBackend creates a new Azure Key Vault backend.
func NewAzureKeyVaultBackend(_ *AzureKeyVaultConfig, _ *license.Manager) (*AzureKeyVaultBackend, error) {
	return nil, ErrEnterpriseRequired
}

func (a *AzureKeyVaultBackend) FetchSecret(_ context.Context, _ string) (string, error) {
	return "", ErrEnterpriseRequired
}
func (a *AzureKeyVaultBackend) SetSecret(_ context.Context, _, _ string) error {
	return ErrEnterpriseRequired
}
func (a *AzureKeyVaultBackend) DeleteSecret(_ context.Context, _ string) error {
	return ErrEnterpriseRequired
}

// ---------------------------------------------------------------------------
// Infisical (stub)
// ---------------------------------------------------------------------------

// InfisicalConfig contains configuration for Infisical backend.
type InfisicalConfig struct {
	SiteURL   string
	Token     string
	ProjectID string
	EnvSlug   string
}

// InfisicalBackend implements SecretBackend for Infisical.
type InfisicalBackend struct {
	config         *InfisicalConfig
	licenseManager *license.Manager
}

// NewInfisicalBackend creates a new Infisical backend.
func NewInfisicalBackend(_ *InfisicalConfig, _ *license.Manager) (*InfisicalBackend, error) {
	return nil, ErrEnterpriseRequired
}

func (i *InfisicalBackend) FetchSecret(_ context.Context, _ string) (string, error) {
	return "", ErrEnterpriseRequired
}

// ---------------------------------------------------------------------------
// OpenBao (stub)
// ---------------------------------------------------------------------------

// OpenBaoConfig contains configuration for OpenBao backend.
type OpenBaoConfig struct {
	Address    string
	Token      string
	PathPrefix string
}

// OpenBaoBackend implements SecretBackend for OpenBao (open-source Vault fork).
type OpenBaoBackend struct {
	config         *OpenBaoConfig
	licenseManager *license.Manager
}

// NewOpenBaoBackend creates a new OpenBao backend.
func NewOpenBaoBackend(_ *OpenBaoConfig, _ *license.Manager) (*OpenBaoBackend, error) {
	return nil, ErrEnterpriseRequired
}

func (o *OpenBaoBackend) FetchSecret(_ context.Context, _ string) (string, error) {
	return "", ErrEnterpriseRequired
}
