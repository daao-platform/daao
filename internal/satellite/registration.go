// Package satellite provides satellite registration and mTLS functionality.
package satellite

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// KeyPair represents an Ed25519 key pair with its fingerprint
type KeyPair struct {
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
	Fingerprint string
}

// GenerateEd25519KeyPair generates a new Ed25519 key pair
func GenerateEd25519KeyPair() (*KeyPair, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Ed25519 key pair: %w", err)
	}

	fingerprint, err := ComputeFingerprint(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute fingerprint: %w", err)
	}

	return &KeyPair{
		PublicKey:   publicKey,
		PrivateKey:  privateKey,
		Fingerprint: fingerprint,
	}, nil
}

// ComputeFingerprint computes the SHA-256 fingerprint of a public key
// The fingerprint is computed from the DER-encoded public key
func ComputeFingerprint(publicKey ed25519.PublicKey) (string, error) {
	// Marshal the public key to DER format
	// For Ed25519, we use PKIX format
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	// Compute SHA-256 hash of the DER-encoded public key
	hash := sha256.Sum256(pubKeyBytes)

	// Encode as base64 for storage/transmission
	fingerprint := base64.StdEncoding.EncodeToString(hash[:])

	return fingerprint, nil
}

// ComputeFingerprintRaw computes SHA-256 fingerprint from DER-encoded public key bytes
func ComputeFingerprintRaw(derBytes []byte) string {
	hash := sha256.Sum256(derBytes)
	return base64.StdEncoding.EncodeToString(hash[:])
}

// SaveKeyPair saves a key pair to files
func (kp *KeyPair) SaveKeyPair(publicKeyPath, privateKeyPath string) error {
	// Save public key - marshal to DER format
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(kp.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}

	pubKeyPEM := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}
	if err := os.WriteFile(publicKeyPath, pem.EncodeToMemory(&pubKeyPEM), 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	// Save private key - marshal using PKCS8 format for Ed25519
	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(kp.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	privKeyPEM := pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privKeyBytes,
	}
	if err := os.WriteFile(privateKeyPath, pem.EncodeToMemory(&privKeyPEM), 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	return nil
}

// LoadKeyPair loads a key pair from files
func LoadKeyPair(publicKeyPath, privateKeyPath string) (*KeyPair, error) {
	// Load public key
	pubKeyData, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key: %w", err)
	}

	block, _ := pem.Decode(pubKeyData)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("invalid public key PEM")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	ed25519PubKey, ok := pubKey.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 public key")
	}

	// Load private key
	privKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key: %w", err)
	}

	block, _ = pem.Decode(privKeyData)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("invalid private key PEM")
	}

	privKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	ed25519PrivKey, ok := privKey.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an Ed25519 private key")
	}

	// Compute fingerprint
	fingerprint, err := ComputeFingerprint(ed25519PubKey)
	if err != nil {
		return nil, err
	}

	return &KeyPair{
		PublicKey:   ed25519PubKey,
		PrivateKey:  ed25519PrivKey,
		Fingerprint: fingerprint,
	}, nil
}

// GetDefaultKeyPaths returns the default paths for storing keys
func GetDefaultKeyPaths() (publicKeyPath, privateKeyPath string, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "daao")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", "", fmt.Errorf("failed to create config directory: %w", err)
	}

	publicKeyPath = filepath.Join(configDir, "satellite.pub")
	privateKeyPath = filepath.Join(configDir, "satellite.key")

	return publicKeyPath, privateKeyPath, nil
}

// SatelliteRegistration represents a satellite's registration with Nexus
type SatelliteRegistration struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint"`
	PublicKey   string `json:"public_key"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Status      string `json:"status"`
}

// NexusClient provides HTTP client for Nexus API communication
type NexusClient struct {
	httpClient *http.Client
	nexusURL   string
	apiKey     string
}

// NewNexusClient creates a new Nexus client
func NewNexusClient(nexusURL, apiKey string) *NexusClient {
	return &NexusClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
		},
		nexusURL: nexusURL,
		apiKey:   apiKey,
	}
}

// RegisterWithNexus registers the satellite with Nexus using the public key
func (kp *KeyPair) RegisterWithNexus(nexusURL string) (*SatelliteRegistration, error) {
	return kp.RegisterWithNexusAPI(nexusURL, "")
}

// RegisterWithNexusAPI registers the satellite with Nexus using HTTP API
func (kp *KeyPair) RegisterWithNexusAPI(nexusURL, apiKey string) (*SatelliteRegistration, error) {
	// Marshal public key to PEM format for API
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: func() []byte {
			b, _ := x509.MarshalPKIXPublicKey(kp.PublicKey)
			return b
		}(),
	})

	// Prepare registration request
	regReq := map[string]string{
		"fingerprint": kp.Fingerprint,
		"public_key":  base64.StdEncoding.EncodeToString(pubKeyPEM),
	}

	jsonReq, err := json.Marshal(regReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Try to register with Nexus API
	url := nexusURL + "/api/v1/satellites/register"
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(jsonReq))
	if err != nil {
		// If we can't make the request, fall back to local storage
		return kp.fallbackRegistration()
	}

	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// If we can't connect to Nexus, fall back to local storage
		return kp.fallbackRegistration()
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		var regResp SatelliteRegistration
		if err := json.NewDecoder(resp.Body).Decode(&regResp); err == nil {
			return &regResp, nil
		}
	}

	// Fall back to local storage if API fails
	return kp.fallbackRegistration()
}

// fallbackRegistration creates a local registration when Nexus is unreachable
func (kp *KeyPair) fallbackRegistration() (*SatelliteRegistration, error) {
	pubKeyBase64 := base64.StdEncoding.EncodeToString(kp.PublicKey)

	registration := &SatelliteRegistration{
		ID:          kp.Fingerprint[:16],
		Fingerprint: kp.Fingerprint,
		PublicKey:   pubKeyBase64,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		Status:      "pending",
	}

	// Save registration locally for later sync
	if err := kp.saveLocalRegistration(registration); err != nil {
		// Log but don't fail - registration still valid
		fmt.Printf("Warning: failed to save local registration: %v\n", err)
	}

	return registration, nil
}

// saveLocalRegistration saves registration locally for later sync
func (kp *KeyPair) saveLocalRegistration(reg *SatelliteRegistration) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(homeDir, ".config", "daao")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	regPath := filepath.Join(configDir, "registration.json")
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(regPath, data, 0600)
}

// SatelliteStore provides database operations for satellites
type SatelliteStore interface {
	CreateSatellite(ctx context.Context, satellite *Satellite) error
	GetSatellite(ctx context.Context, id string) (*Satellite, error)
	ListSatellites(ctx context.Context) ([]*Satellite, error)
	UpdateSatelliteStatus(ctx context.Context, id, status string) error
}

// Satellite represents a satellite in the database
type Satellite struct {
	ID          string    `json:"id"`
	Fingerprint string    `json:"fingerprint"`
	PublicKey   string    `json:"public_key"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SatelliteStoreImpl implements SatelliteStore using a file-based approach
type SatelliteStoreImpl struct {
	dataPath string
}

// NewSatelliteStore creates a new satellite store
func NewSatelliteStore(dataPath string) *SatelliteStoreImpl {
	return &SatelliteStoreImpl{dataPath: dataPath}
}

// CreateSatellite creates a new satellite record
func (s *SatelliteStoreImpl) CreateSatellite(ctx context.Context, satellite *Satellite) error {
	satellites, err := s.loadSatellites()
	if err != nil {
		return err
	}

	satellite.ID = satellite.Fingerprint[:16]
	satellite.CreatedAt = time.Now().UTC()
	satellite.UpdatedAt = satellite.CreatedAt
	satellite.Status = "registered"

	satellites[satellite.ID] = satellite

	return s.saveSatellites(satellites)
}

// GetSatellite retrieves a satellite by ID
func (s *SatelliteStoreImpl) GetSatellite(ctx context.Context, id string) (*Satellite, error) {
	satellites, err := s.loadSatellites()
	if err != nil {
		return nil, err
	}

	if sat, ok := satellites[id]; ok {
		return sat, nil
	}

	return nil, fmt.Errorf("satellite not found: %s", id)
}

// ListSatellites returns all satellites
func (s *SatelliteStoreImpl) ListSatellites(ctx context.Context) ([]*Satellite, error) {
	satellites, err := s.loadSatellites()
	if err != nil {
		return nil, err
	}

	result := make([]*Satellite, 0, len(satellites))
	for _, sat := range satellites {
		result = append(result, sat)
	}

	return result, nil
}

// UpdateSatelliteStatus updates a satellite's status
func (s *SatelliteStoreImpl) UpdateSatelliteStatus(ctx context.Context, id, status string) error {
	satellites, err := s.loadSatellites()
	if err != nil {
		return err
	}

	if sat, ok := satellites[id]; ok {
		sat.Status = status
		sat.UpdatedAt = time.Now().UTC()
		return s.saveSatellites(satellites)
	}

	return fmt.Errorf("satellite not found: %s", id)
}

// loadSatellites loads satellites from storage
func (s *SatelliteStoreImpl) loadSatellites() (map[string]*Satellite, error) {
	data, err := os.ReadFile(s.dataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*Satellite), nil
		}
		return nil, err
	}

	var satellites map[string]*Satellite
	if err := json.Unmarshal(data, &satellites); err != nil {
		return nil, err
	}

	return satellites, nil
}

// saveSatellites saves satellites to storage
func (s *SatelliteStoreImpl) saveSatellites(satellites map[string]*Satellite) error {
	data, err := json.MarshalIndent(satellites, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.dataPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	return os.WriteFile(s.dataPath, data, 0600)
}

// SatelliteDatabaseMigration returns SQL for creating the satellites table
const SatelliteDatabaseMigration = `
-- Create satellites table for storing registered satellite information
CREATE TABLE IF NOT EXISTS satellites (
    id VARCHAR(64) PRIMARY KEY,
    fingerprint VARCHAR(128) NOT NULL UNIQUE,
    public_key TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Index for faster lookups by fingerprint
CREATE INDEX IF NOT EXISTS idx_satellites_fingerprint ON satellites(fingerprint);

-- Index for listing satellites by status
CREATE INDEX IF NOT EXISTS idx_satellites_status ON satellites(status);
`

// GenerateMTLSCertificate generates an mTLS certificate for satellite-to-Nexus communication
func (kp *KeyPair) GenerateMTLSCertificate(caCert *x509.Certificate, caKey interface{}, commonName string) (*tls.Certificate, error) {
	// Generate a new Ed25519 key for the certificate
	_, certKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificate key: %w", err)
	}

	// Create certificate template
	serialNumber := big.NewInt(1)
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:          []net.IP{net.ParseIP("0.0.0.0")},
	}

	// Create the certificate using the Ed25519 private key
	certDER, err := x509.CreateCertificate(rand.Reader, &template, caCert, certKey.Public(), caKey)

	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	// Parse the certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Create tls.Certificate from the generated key and certificate
	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:   certKey,
		Leaf:        cert,
	}

	return &tlsCert, nil
}

// LoadClientCertificate loads mTLS certificate from files
func LoadClientCertificate(certPath, keyPath string) (*tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}

	return &cert, nil
}

// LoadCACertificatePool loads CA certificates for verifying Nexus
func LoadCACertificatePool(caPath string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return pool, nil
}
