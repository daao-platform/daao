package license

// Feature constants define the feature flags that can be enabled by a license key.
// These are used in license Claims.Features and checked via Manager.HasFeature().
const (
	// FeatureHITL enables Human-In-The-Loop command interception.
	FeatureHITL = "hitl"

	// FeatureSIEM enables SIEM integration (Splunk, Sentinel, Datadog).
	FeatureSIEM = "siem"

	// FeatureDiscovery enables autonomous discovery / audit mode.
	FeatureDiscovery = "discovery"

	// FeatureRBAC enables full RBAC with OIDC/SSO integration.
	FeatureRBAC = "rbac"

	// FeatureAdvancedTelemetry enables GPU metrics, historical trends, and alerting.
	FeatureAdvancedTelemetry = "advanced_telemetry"

	// FeatureAdvancedRecordings enables search, compliance export, and unlimited recordings.
	FeatureAdvancedRecordings = "advanced_recordings"

	// FeatureSessionChaining enables pipeline workflows across sessions.
	FeatureSessionChaining = "session_chaining"

	// FeatureScheduledSessions enables scheduled and event-triggered sessions.
	FeatureScheduledSessions = "scheduled_sessions"

	// FeatureAgentRouting enables GPU/capability-based agent scheduling.
	FeatureAgentRouting = "agent_routing"

	// FeatureVaultIntegrations enables enterprise vault backends (Vault, OpenBao, Azure Key Vault, Infisical).
	FeatureVaultIntegrations = "vault_integrations"

	// FeatureHA enables multi-instance Nexus clustering with NATS pub/sub,
	// distributed state (S3 recordings, Redis rate limiting), and leader-elected scheduler.
	FeatureHA = "ha"
)

// Community edition limits.
const (
	// CommunityMaxUsers is the maximum number of users in Community mode.
	CommunityMaxUsers = 3

	// CommunityMaxSatellites is the maximum number of satellites in Community mode.
	CommunityMaxSatellites = 5

	// CommunityMaxRecordings is the maximum number of retained recordings in Community mode.
	CommunityMaxRecordings = 50

	// CommunityTelemetryRetention is the telemetry retention window in Community mode.
	CommunityTelemetryRetention = 1 // hours
)

// MaxRecordings returns the recording limit for the current license.
func (m *Manager) MaxRecordings() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.claims != nil && m.claims.HasFeature(FeatureAdvancedRecordings) {
		return 0 // unlimited
	}
	return CommunityMaxRecordings
}

// TelemetryRetentionHours returns the telemetry retention window in hours.
func (m *Manager) TelemetryRetentionHours() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.claims != nil && m.claims.HasFeature(FeatureAdvancedTelemetry) {
		return 720 // 30 days
	}
	return CommunityTelemetryRetention
}

// FeatureInfo describes an enterprise feature for UI display.
type FeatureInfo struct {
	ID          string
	Name        string
	Description string
}

// AllEnterpriseFeatures returns metadata about all enterprise features.
// This is used by the Cockpit API to show locked features with descriptions.
func AllEnterpriseFeatures() []FeatureInfo {
	return []FeatureInfo{
		{FeatureHITL, "Human-in-the-Loop Guardrails", "Cryptographic command interception and MFA approval for AI-driven remediation"},
		{FeatureSIEM, "SIEM Integration", "Stream audit logs to Splunk, Azure Sentinel, or Datadog"},
		{FeatureDiscovery, "Autonomous Discovery", "Time-boxed audit mode for CMDB auto-population and Shadow IT detection"},
		{FeatureRBAC, "Multi-User & RBAC", "OIDC/SSO integration, role-based access control, and team management"},
		{FeatureAdvancedTelemetry, "Advanced Telemetry", "GPU metrics, historical trends, capacity planning, and alerting"},
		{FeatureAdvancedRecordings, "Advanced Recordings", "Unlimited recordings, full-text search, compliance export, and retention policies"},
		{FeatureSessionChaining, "Session Chaining", "Pipeline workflows across multiple sessions"},
		{FeatureScheduledSessions, "Scheduled Sessions", "Scheduled and event-triggered session automation"},
		{FeatureAgentRouting, "Agent Routing", "GPU and capability-based agent scheduling"},
		{FeatureVaultIntegrations, "Vault Integrations", "Enterprise secret backends: HashiCorp Vault, OpenBao, Azure Key Vault, and Infisical"},
		{FeatureHA, "High Availability", "Multi-instance Nexus clustering with NATS pub/sub, distributed state, and automatic failover"},
	}
}
