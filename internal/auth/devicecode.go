// Package auth provides OAuth2 Device Code Flow (RFC 8628) authentication.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// UserCodeLength is the length of the user-readable code (8 characters)
const UserCodeLength = 8

// DeviceCodeExpiry is the default expiration time for device codes
const DeviceCodeExpiry = 5 * time.Minute

// PollingInterval is the default interval for polling token endpoint
const PollingInterval = 5 * time.Second

// DeviceCode represents an OAuth2 Device Code flow request
type DeviceCode struct {
	DeviceCode      string    `json:"device_code"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn       int       `json:"expires_in"`
	Interval        int       `json:"interval"`
	ClientID        string    `json:"-"`
	ClientSecret    string    `json:"-"`
	Provider        OAuthProvider `json:"-"`
	Status          string    `json:"status"` // "pending", "authorized", "expired"
	CreatedAt       time.Time `json:"created_at"`
}

// TokenResponse represents the token response from the OAuth provider
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// JWTClaims represents the JWT claims for the satellite token
type JWTClaims struct {
	jwt.RegisteredClaims
	Subject   string `json:"sub"`
	Scopes    string `json:"scopes"`
	Provider  string `json:"provider"`
}

// OAuthProvider defines the interface for OAuth providers
type OAuthProvider interface {
	GetDeviceAuthorizationEndpoint() string
	GetTokenEndpoint() string
	GetClientID() string
	GetClientSecret() string
	GetScopes() []string
	GetUserCodeChars() string
}

// GitHubProvider implements OAuthProvider for GitHub
type GitHubProvider struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
}

func (p *GitHubProvider) GetDeviceAuthorizationEndpoint() string {
	return "https://github.com/login/device/code"
}

func (p *GitHubProvider) GetTokenEndpoint() string {
	return "https://github.com/login/oauth/access_token"
}

func (p *GitHubProvider) GetClientID() string {
	return p.ClientID
}

func (p *GitHubProvider) GetClientSecret() string {
	return p.ClientSecret
}

func (p *GitHubProvider) GetScopes() []string {
	if p.Scopes == nil {
		return []string{"read:user", "user:email"}
	}
	return p.Scopes
}

func (p *GitHubProvider) GetUserCodeChars() string {
	return "BCDFGHJKLMNPQRSTVWXZ" // No vowels to avoid inappropriate words
}

// GoogleProvider implements OAuthProvider for Google
type GoogleProvider struct {
	ClientID     string
	ClientSecret string
	Scopes       []string
}

func (p *GoogleProvider) GetDeviceAuthorizationEndpoint() string {
	return "https://oauth2.googleapis.com/device/code"
}

func (p *GoogleProvider) GetTokenEndpoint() string {
	return "https://oauth2.googleapis.com/token"
}

func (p *GoogleProvider) GetClientID() string {
	return p.ClientID
}

func (p *GoogleProvider) GetClientSecret() string {
	return p.ClientSecret
}

func (p *GoogleProvider) GetScopes() []string {
	if p.Scopes == nil {
		return []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"}
	}
	return p.Scopes
}

func (p *GoogleProvider) GetUserCodeChars() string {
	return "BCDFGHJKLMNPQRSTVWXZ23456789" // No vowels, numbers only
}

// DeviceCodeStore stores active device codes in memory
type DeviceCodeStore struct {
	mu         sync.RWMutex
	codes      map[string]*DeviceCode
	userCodes  map[string]*DeviceCode
	expiresAt  time.Time
}

// NewDeviceCodeStore creates a new in-memory device code store
func NewDeviceCodeStore() *DeviceCodeStore {
	return &DeviceCodeStore{
		codes:     make(map[string]*DeviceCode),
		userCodes: make(map[string]*DeviceCode),
	}
}

// GenerateDeviceCode generates a new device code and user code
func (s *DeviceCodeStore) GenerateDeviceCode(ctx context.Context, provider OAuthProvider, verificationBaseURL string) (*DeviceCode, error) {
	// Generate device_code (cryptographically secure)
	deviceCode, err := generateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate device_code: %w", err)
	}

	// Generate user_code in XXXX-XXXX format
	userCode, err := generateUserCode(provider.GetUserCodeChars())
	if err != nil {
		return nil, fmt.Errorf("failed to generate user_code: %w", err)
	}

	dc := &DeviceCode{
		DeviceCode:      deviceCode,
		UserCode:        userCode,
		VerificationURI: verificationBaseURL,
		ExpiresIn:       int(DeviceCodeExpiry.Seconds()),
		Interval:        int(PollingInterval.Seconds()),
		ClientID:        provider.GetClientID(),
		ClientSecret:    provider.GetClientSecret(),
		Provider:        provider,
		Status:          "pending",
		CreatedAt:       time.Now(),
	}

	// Add complete verification URI if supported
	dc.VerificationURIComplete = fmt.Sprintf("%s?user_code=%s", verificationBaseURL, userCode)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.codes[deviceCode] = dc
	s.userCodes[userCode] = dc

	return dc, nil
}

// generateRandomString generates a cryptographically secure random string
func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// generateUserCode generates a human-readable user code in XXXX-XXXX format
func generateUserCode(chars string) (string, error) {
	if len(chars) == 0 {
		chars = "BCDFGHJKLMNPQRSTVWXZ23456789"
	}

	result := make([]byte, UserCodeLength)
	for i := range result {
		idx, err := generateRandomInt(len(chars))
		if err != nil {
			return "", err
		}
		result[i] = chars[idx]
	}

	// Format as XXXX-XXXX
	return fmt.Sprintf("%s-%s", string(result[:4]), string(result[4:])), nil
}

// generateRandomInt generates a cryptographically secure random integer
func generateRandomInt(max int) (int, error) {
	bytes := make([]byte, 1)
	if _, err := rand.Read(bytes); err != nil {
		return 0, err
	}
	return int(bytes[0]) % max, nil
}

// GetByDeviceCode retrieves a device code by its device code
func (s *DeviceCodeStore) GetByDeviceCode(deviceCode string) (*DeviceCode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dc, ok := s.codes[deviceCode]
	if !ok {
		return nil, fmt.Errorf("device code not found")
	}

	// Check expiration
	if time.Since(dc.CreatedAt) > DeviceCodeExpiry {
		dc.Status = "expired"
		return nil, fmt.Errorf("device code expired")
	}

	return dc, nil
}

// GetByUserCode retrieves a device code by its user code
func (s *DeviceCodeStore) GetByUserCode(userCode string) (*DeviceCode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dc, ok := s.userCodes[strings.ToUpper(userCode)]
	if !ok {
		return nil, fmt.Errorf("user code not found")
	}

	// Check expiration
	if time.Since(dc.CreatedAt) > DeviceCodeExpiry {
		dc.Status = "expired"
		return nil, fmt.Errorf("user code expired")
	}

	return dc, nil
}

// UpdateStatus updates the status of a device code
func (s *DeviceCodeStore) UpdateStatus(deviceCode, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dc, ok := s.codes[deviceCode]
	if !ok {
		return fmt.Errorf("device code not found")
	}

	dc.Status = status
	return nil
}

// TokenIssuer handles JWT token issuance
type TokenIssuer struct {
	mu           sync.Mutex
	signingKey   []byte
	issuer       string
	defaultTTL   time.Duration
}

// NewTokenIssuer creates a new token issuer
func NewTokenIssuer(signingKey []byte, issuer string) *TokenIssuer {
	return &TokenIssuer{
		signingKey: signingKey,
		issuer:     issuer,
		defaultTTL: time.Hour * 24, // 24 hours default
	}
}

// IssueToken issues a JWT token for the authenticated user
func (ti *TokenIssuer) IssueToken(ctx context.Context, claims *JWTClaims) (string, error) {
	if claims.Issuer == "" {
		claims.Issuer = ti.issuer
	}
	if claims.IssuedAt == nil {
		claims.IssuedAt = jwt.NewNumericDate(time.Now())
	}
	if claims.ExpiresAt == nil {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(ti.defaultTTL))
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(ti.signingKey)
}

// ValidateToken validates a JWT token and returns the claims
func (ti *TokenIssuer) ValidateToken(ctx context.Context, tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return ti.signingKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// DeviceCodeService provides the device code flow functionality
type DeviceCodeService struct {
	store      *DeviceCodeStore
	httpClient *http.Client
}

// NewDeviceCodeService creates a new device code service
func NewDeviceCodeService() *DeviceCodeService {
	return &DeviceCodeService{
		store:      NewDeviceCodeStore(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Initiate initiates the device code flow with the given provider
func (s *DeviceCodeService) Initiate(ctx context.Context, provider OAuthProvider, verificationBaseURL string) (*DeviceCode, error) {
	// First, try to use the provider's device authorization endpoint if available
	if verificationBaseURL == "" {
		verificationBaseURL = "https://github.com/login/device"
		if _, ok := provider.(*GoogleProvider); ok {
			verificationBaseURL = "https://accounts.google.com/o/oauth2/device/code"
		}
	}

	// Generate local device code and user code
	dc, err := s.store.GenerateDeviceCode(ctx, provider, verificationBaseURL)
	if err != nil {
		return nil, err
	}

	// Optionally, make a request to the provider's device authorization endpoint
	// This is useful for getting the actual device_code from the OAuth provider
	reqBody := fmt.Sprintf("client_id=%s&scope=%s",
		urlEncode(provider.GetClientID()),
		urlEncode(strings.Join(provider.GetScopes(), " ")))

	req, err := http.NewRequestWithContext(ctx, "POST", provider.GetDeviceAuthorizationEndpoint(), strings.NewReader(reqBody))
	if err != nil {
		// If we can't make the request, return the locally generated codes
		return dc, nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		// If we can't make the request, return the locally generated codes
		return dc, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var providerResp map[string]string
		if body, err := io.ReadAll(resp.Body); err == nil {
			// Parse form response
			values := strings.Split(string(body), "&")
			providerResp = make(map[string]string)
			for _, v := range values {
				kv := strings.Split(v, "=")
				if len(kv) == 2 {
					providerResp[kv[0]] = kv[1]
				}
			}

			// Update with provider's device_code if available
			if dcStr, ok := providerResp["device_code"]; ok {
				dc.DeviceCode = dcStr
			}
			if uri, ok := providerResp["verification_uri"]; ok {
				dc.VerificationURI = uri
			}
			if uriComplete, ok := providerResp["verification_uri_complete"]; ok {
				dc.VerificationURIComplete = uriComplete
			}
			if expiresIn, ok := providerResp["expires_in"]; ok {
				fmt.Sscanf(expiresIn, "%d", &dc.ExpiresIn)
			}
			if interval, ok := providerResp["interval"]; ok {
				fmt.Sscanf(interval, "%d", &dc.Interval)
			}
		}
	}

	return dc, nil
}

// PollToken polls the token endpoint for a token
func (s *DeviceCodeService) PollToken(ctx context.Context, deviceCode string) (*TokenResponse, error) {
	dc, err := s.store.GetByDeviceCode(deviceCode)
	if err != nil {
		return nil, err
	}

	if dc.Status == "expired" {
		return nil, fmt.Errorf("device code expired")
	}

	if dc.Status == "authorized" {
		// Exchange device code for token
		return s.exchangeToken(ctx, dc)
	}

	// Make request to token endpoint
	reqBody := fmt.Sprintf("client_id=%s&client_secret=%s&device_code=%s&grant_type=urn:ietf:params:oauth:grant-type:device_code",
		urlEncode(dc.ClientID),
		urlEncode(dc.ClientSecret),
		urlEncode(deviceCode))

	req, err := http.NewRequestWithContext(ctx, "POST", dc.Provider.GetTokenEndpoint(), strings.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	// Check for authorization_pending
	if resp.StatusCode == http.StatusBadRequest {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			if errResp.Error == "authorization_pending" {
				return nil, fmt.Errorf("authorization_pending")
			}
		}
	}

	return &tokenResp, nil
}

// exchangeToken exchanges a device code for an access token
func (s *DeviceCodeService) exchangeToken(ctx context.Context, dc *DeviceCode) (*TokenResponse, error) {
	reqBody := fmt.Sprintf("client_id=%s&client_secret=%s&device_code=%s&grant_type=urn:ietf:params:oauth:grant-type:device_code",
		urlEncode(dc.ClientID),
		urlEncode(dc.ClientSecret),
		urlEncode(dc.DeviceCode))

	req, err := http.NewRequestWithContext(ctx, "POST", dc.Provider.GetTokenEndpoint(), strings.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

// urlEncode URL-encodes a string
func urlEncode(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "+", "%2B"), " ", "%20")
}
