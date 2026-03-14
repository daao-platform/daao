package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"

	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/daao/nexus/db"
	"github.com/daao/nexus/internal/agentstream"
	"github.com/daao/nexus/internal/api"
	"github.com/daao/nexus/internal/audit"
	"github.com/daao/nexus/internal/auth"
	"github.com/daao/nexus/internal/database"
	"github.com/daao/nexus/internal/dispatch"
	"github.com/daao/nexus/internal/enterprise/forge"
	"github.com/daao/nexus/internal/enterprise/ha"
	"github.com/daao/nexus/internal/enterprise/hitl"
	"github.com/daao/nexus/internal/grpc"
	"github.com/daao/nexus/internal/license"
	"github.com/daao/nexus/internal/logging"
	"github.com/daao/nexus/internal/metrics"
	"github.com/daao/nexus/internal/notification"
	"github.com/daao/nexus/internal/recording"
	"github.com/daao/nexus/internal/router"
	"github.com/daao/nexus/internal/secrets"
	"github.com/daao/nexus/internal/session"
	"github.com/daao/nexus/internal/stream"
	"github.com/daao/nexus/internal/transport"
	nats "github.com/nats-io/nats.go"
	"github.com/daao/nexus/proto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	grpccreds "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// embeddedLicensePubKey is set at build time via:
//
//	go build -ldflags "-X main.embeddedLicensePubKey=<base64-encoded-ed25519-public-key>"
//
// Official Docker images have this baked in, so only licenses signed by
// the DAAO licensor's private key are accepted. Source builds leave this
// empty and fall back to DAAO_LICENSE_PUBLIC_KEY env vars.
var embeddedLicensePubKey string

type NexusConfig struct {
	ServerCert, ServerKey, ClientCAs, ListenAddr, GRPCAddr string
}

type NexusServer struct {
	proto.UnimplementedSatelliteGatewayServer
	httpServer            *http.Server
	grpcServer            *grpccreds.Server
	config                *NexusConfig
	sessionStore          session.Store
	sessionStoreImpl      *session.SessionStore
	streamRegistry        stream.StreamRegistryInterface
	ringBufferPool        *session.RingBufferPool
	recordingPool         recording.RecordingPoolInterface
	leaderGuard           *ha.LeaderSchedulerGuard
	dbPool                *pgxpool.Pool
	pool                  *pgxpool.Pool
	ready                 bool
	handlers              *api.Handlers
	agentHandler          *api.AgentHandler
	scheduleHandler       *api.ScheduleHandler
	contextHandler        *api.ContextHandler
	secretHandler         *api.SecretHandler
	userHandler           *api.UserHandler
	authHandler           *api.AuthHandler
	auditHandler          *api.AuditHandler
	streamRouter          *router.Router
	terminalStreamHandler *transport.TerminalStreamHandler
	notifService          *notification.NotificationService
	sseHub                *notification.SSEHub
	runEventHub           agentstream.RunEventHubInterface
	providerConfigHandler *api.ProviderConfigHandler
	jwtValidator          *auth.JWTTokenValidator
	jwtSecret             string
	jwtIssuer             string
	licenseMgr            *license.Manager
	scheduler             *forge.Scheduler
	pipelineHandler       *api.PipelineHandler
	oidcEnabled           bool
	natsConn              *nats.Conn
}

func NewNexusServer(c *NexusConfig) *NexusServer {
	return newNexusServerWithDeps(c, nil, nil, nil)
}

func NewNexusServerWithDB(c *NexusConfig, p *pgxpool.Pool) *NexusServer {
	return newNexusServerWithDeps(c, p, session.NewSessionStore(p), session.NewRingBufferPool())
}

func newNexusServerWithDeps(c *NexusConfig, dbp *pgxpool.Pool, ss session.Store, rb *session.RingBufferPool) *NexusServer {
	sr2 := router.NewRouter(nil, ss)
	ca := &nexusConfigAccessor{config: c}
	var ssi *session.SessionStore
	if s, ok := ss.(*session.SessionStore); ok {
		ssi = s
	}

	// Initialize license manager — three-layer key resolution:
	//   1. Build-time embedded key (official Docker images) — injected via -ldflags
	//   2. DAAO_LICENSE_PUBLIC_KEY env var (inline base64)
	//   3. DAAO_LICENSE_PUBLIC_KEY_FILE env var (path to file)
	// Official builds have the embedded key baked in, so env vars can't override it.
	// Source builds (no embedded key) fall back to env vars.
	licPubKey := embeddedLicensePubKey
	if licPubKey != "" {
		slog.Info("using embedded public key", "component", "license", "source", "official-build")
	} else {
		licPubKey = os.Getenv("DAAO_LICENSE_PUBLIC_KEY")
		if licPubKey == "" {
			if f := os.Getenv("DAAO_LICENSE_PUBLIC_KEY_FILE"); f != "" {
				if data, err := os.ReadFile(f); err != nil {
					slog.Error("failed to read public key file", "component", "license", "file", f, "error", err)
				} else {
					licPubKey = strings.TrimSpace(string(data))
				}
			}
		}
	}
	licMgr, err := license.NewManager(licPubKey)
	if err != nil {
		slog.Error("failed to initialize license manager, continuing in community mode", "component", "license", "error", err)
		licMgr, _ = license.NewManager("")
	}
	licKey := os.Getenv("DAAO_LICENSE_KEY")
	if licKey == "" {
		if f := os.Getenv("DAAO_LICENSE_KEY_FILE"); f != "" {
			if data, err := os.ReadFile(f); err != nil {
				slog.Error("failed to read license key file", "component", "license", "file", f, "error", err)
			} else {
				licKey = strings.TrimSpace(string(data))
			}
		}
	}
	if licKey != "" {
		if err := licMgr.LoadKey(licKey); err != nil {
			slog.Error("invalid license key, running in community mode", "component", "license", "error", err)
		} else {
			claim := licMgr.Claims()
			slog.Info("license activated", "component", "license", "tier", licMgr.Tier(), "customer", claim.Customer,
				"max_users", claim.MaxUsers, "max_satellites", claim.MaxSatellites, "features", len(claim.Features),
				"expires", time.Unix(claim.ExpiresAt, 0).Format("2006-01-02"))
		}
	} else {
		slog.Info("no license key configured, running in community mode", "component", "license")
	}

	// Initialize recording pool with configurable directory
	recordingsDir := envOr("DAAO_RECORDINGS_DIR", "/data/recordings")
	rp, err := ha.NewRecordingPool(licMgr, recordingsDir)
	if err != nil {
		slog.Error("failed to init recording pool", "err", err)
		os.Exit(1)
	}

	// Reconcile orphaned .cast files on disk with DB metadata (async so startup isn't blocked)
	if dbp != nil {
		go func() {
			rp.ReconcileRecordings(func(query string, args ...interface{}) error {
				_, err := dbp.Exec(context.Background(), query, args...)
				return err
			})
		}()
	}

	// Initialize notification system
	sseHub := notification.NewSSEHub()
	var notifStore notification.Store
	var notifService *notification.NotificationService
	if dbp != nil {
		notifStore = notification.NewPgStore(dbp)
		bus := notification.NewEventBus()
		notifService = notification.NewNotificationService(bus, notifStore)
		notifService.RegisterDispatcher(notification.NewSSEDispatcher(sseHub))
		slog.Info("notification system initialized", "component", "notifications", "dispatcher", "sse")
	}

	// Initialize HA stream registry and run event hub (after license manager)
	sr, natsConn := ha.NewStreamRegistry(licMgr)
	runEventHub := ha.NewRunEventHub(licMgr, natsConn)

	// Initialize HITL manager (enterprise-gated)
	var hitlMgr *hitl.Manager
	if licMgr.HasFeature(license.FeatureHITL) && dbp != nil {
		hitlStore := hitl.NewStore(dbp)
		var eventBus *notification.EventBus
		if notifService != nil {
			eventBus = notifService.Bus()
		}
		hitlMgr = hitl.NewManager(hitlStore, sr, eventBus)
		slog.Info("HITL guardrails enabled", "component", "hitl")
	} else {
		slog.Info("HITL disabled", "component", "hitl", "reason", "community mode or no database")
	}

	// Initialize audit logger (needed by pipeline handler)
	var auditLogger *audit.AuditLogger
	if dbp != nil {
		auditLogger = audit.NewAuditLogger(dbp)
		slog.Info("audit logger initialized", "component", "audit")
	}

	// Initialize Pipeline Executor (enterprise-gated for session chaining) - before scheduler so setupFn can capture it
	var pipelineExecutor *forge.PipelineExecutor
	var pipelineHandler *api.PipelineHandler
	if licMgr.HasFeature(license.FeatureSessionChaining) && dbp != nil {
		// Create agentRunnerAdapter for pipeline executor
		agentRunner := &agentRunnerAdapter{
			dbPool:         dbp,
			streamRegistry: sr,
		}

		// Create failure handler using notification service
		var failureHandler forge.FailureHandler
		if notifService != nil {
			failureHandler = forge.NewDefaultFailureHandler(notifService.Bus())
		}

		// Create pipeline executor
		var err error
		pipelineExecutor, err = forge.NewPipelineExecutor(dbp, agentRunner, runEventHub, failureHandler, licMgr)
		if err != nil {
			slog.Error("pipeline executor initialization failed, continuing without pipelines", "component", "forge", "error", err)
			pipelineExecutor = nil
		} else {
			slog.Info("pipeline executor initialized", "component", "forge")
		}

		// Create pipeline handler
		pipelineHandler = api.NewPipelineHandler(dbp, pipelineExecutor, licMgr, auditLogger)
		slog.Info("pipeline handler initialized", "component", "api")
	} else {
		slog.Info("session chaining disabled", "component", "forge", "reason", "community mode, no database, or license feature not enabled")
	}

	// Initialize Forge Scheduler (enterprise-gated for scheduled/triggered sessions)
	var leaderGuard *ha.LeaderSchedulerGuard
	var scheduler *forge.Scheduler
	var scheduleHandler *api.ScheduleHandler
	if licMgr.HasFeature(license.FeatureScheduledSessions) && dbp != nil {
		// Create AgentRunnerAdapter that implements forge.AgentRunner
		agentRunnerAdapter := &agentRunnerAdapter{
			dbPool:         dbp,
			streamRegistry: sr,
		}

		// Create failure handler using notification service
		var failureHandler forge.FailureHandler
		if notifService != nil {
			failureHandler = forge.NewDefaultFailureHandler(notifService.Bus())
		}

		// Create leader scheduler guard with factory and setup function
		leaderGuard = ha.NewLeaderSchedulerGuard(
			licMgr,
			dbp,
			func() (*forge.Scheduler, error) {
				sched, err := forge.NewScheduler(licMgr, agentRunnerAdapter, failureHandler)
				if err != nil {
					return nil, err
				}
				if err := sched.LoadFromDB(context.Background(), dbp); err != nil {
					slog.Warn("scheduler: LoadFromDB error", "err", err)
				}
				return sched, nil
			},
			func(s *forge.Scheduler) {
				if pipelineExecutor != nil {
					s.SetPipelineRunner(pipelineExecutor)
				}
			},
		)

		if leaderGuard != nil {
			// HA mode: guard manages scheduler lifecycle
			leaderGuard.Start(context.Background())
			// Get scheduler from guard for handler creation (may be nil if not leader)
			scheduler = leaderGuard.Scheduler()
		} else {
			// Community mode: create scheduler directly
			scheduler, err = forge.NewScheduler(licMgr, agentRunnerAdapter, failureHandler)
			if err != nil {
				slog.Error("scheduler initialization failed, continuing without scheduled sessions", "component", "forge", "error", err)
				scheduler = nil
			} else {
				// Load schedules and triggers from database
				if err := scheduler.LoadFromDB(context.Background(), dbp); err != nil {
					slog.Error("failed to load schedules from database", "component", "forge", "error", err)
					// Continue anyway - schedules will be empty
				}
				// Inject pipeline runner for scheduled pipelines
				if pipelineExecutor != nil {
					scheduler.SetPipelineRunner(pipelineExecutor)
				}
				slog.Info("scheduler initialized with persisted schedules", "component", "forge")
			}
		}

		// Create schedule handler (may be nil if scheduler is nil)
		scheduleHandler = api.NewScheduleHandler(dbp, scheduler, licMgr)
		slog.Info("schedule handler initialized", "component", "forge")
	} else {
		slog.Info("scheduled sessions disabled", "component", "forge", "reason", "community mode, no database, or license feature not enabled")
	}

	h := api.NewHandlers(ss, dbp, sr, rb, ca, rp, notifStore, notifService, licMgr, hitlMgr, auditLogger)

	// Configure optional read replica pool for list queries
	if dbp != nil {
		if readURL := os.Getenv("DATABASE_READ_URL"); readURL != "" {
			readCfg, err := pgxpool.ParseConfig(readURL)
			if err != nil {
				slog.Warn("read replica: invalid DATABASE_READ_URL, using primary", "err", err, "component", "db")
			} else {
				readCfg.MinConns = 1
				readCfg.MaxConns = 10
				if readPool, err := pgxpool.NewWithConfig(context.Background(), readCfg); err != nil {
					slog.Warn("read replica: connection failed, using primary", "err", err, "component", "db")
				} else {
					slog.Info("read replica: active", "url", readURL, "component", "db")
					h.SetReadPool(readPool)
				}
			}
		}
	}

	// Initialize JWT validator and OIDC flag for WebSocket handlers
	oidcEnabled := os.Getenv("OIDC_ISSUER_URL") != ""
	jwtSecret := getJWTSecret()
	jwtIssuer := envOr("JWT_ISSUER", "daao")
	jwtValidator := auth.NewJWTTokenValidator(jwtSecret, jwtIssuer)

	tsh := transport.NewTerminalStreamHandler(ssi, rb, sr, jwtValidator, oidcEnabled)

	// Initialize agent, context, and secret handlers
	var agentHandler *api.AgentHandler
	var contextHandler *api.ContextHandler
	var secretHandler *api.SecretHandler
	var userHandler *api.UserHandler
	var auditHandler *api.AuditHandler
	var dispatcher *dispatch.Dispatcher
	if dbp != nil {
		// Initialize dispatcher for agent routing
		dispatcher = dispatch.NewDispatcher(dbp, sr)
		slog.Info("dispatcher initialized", "component", "dispatch")

		agentHandler = api.NewAgentHandler(dbp, sr, dispatcher, licMgr, auditLogger)
		contextHandler = api.NewContextHandler(dbp, sr)
		// Initialize secrets broker
		broker := secrets.NewBroker(dbp)
		secretHandler = api.NewSecretHandler(dbp, broker)
		slog.Info("secrets broker initialized", "component", "secrets")
		// Initialize user handler
		userHandler = api.NewUserHandler(dbp, auditLogger)
		slog.Info("user handler initialized", "component", "api")
		// Initialize audit handler
		auditHandler = api.NewAuditHandler(dbp)
		slog.Info("audit handler initialized", "component", "api")
	}

	// Initialize auth handler (local login) — always, even without enterprise features
	var authHandler *api.AuthHandler
	if dbp != nil {
		authHandler = api.NewAuthHandler(dbp, jwtSecret, jwtIssuer, auditLogger)
		slog.Info("local auth handler initialized", "component", "api")
	}

	// Initialize provider config handler (stores API keys encrypted in DB)
	var providerCfgHandler *api.ProviderConfigHandler
	if dbp != nil {
		providerCfgHandler = api.NewProviderConfigHandler(dbp, auditLogger)
		slog.Info("provider config handler initialized", "component", "api")
	}

	return &NexusServer{config: c, sessionStore: ss, sessionStoreImpl: ssi, streamRegistry: sr, ringBufferPool: rb, recordingPool: rp, leaderGuard: leaderGuard, dbPool: dbp, pool: dbp, ready: true, handlers: h, agentHandler: agentHandler, scheduleHandler: scheduleHandler, contextHandler: contextHandler, secretHandler: secretHandler, userHandler: userHandler, authHandler: authHandler, auditHandler: auditHandler, providerConfigHandler: providerCfgHandler, streamRouter: sr2, terminalStreamHandler: tsh, notifService: notifService, sseHub: sseHub, runEventHub: runEventHub, jwtValidator: jwtValidator, jwtSecret: jwtSecret, jwtIssuer: jwtIssuer, licenseMgr: licMgr, scheduler: scheduler, pipelineHandler: pipelineHandler, oidcEnabled: oidcEnabled, natsConn: natsConn}
}

type nexusConfigAccessor struct{ config *NexusConfig }

func (c *nexusConfigAccessor) GetServerCert() string { return c.config.ServerCert }
func (c *nexusConfigAccessor) GetServerKey() string  { return c.config.ServerKey }
func (c *nexusConfigAccessor) GetClientCAs() string  { return c.config.ClientCAs }
func (c *nexusConfigAccessor) GetListenAddr() string { return c.config.ListenAddr }
func (c *nexusConfigAccessor) GetGRPCAddr() string   { return c.config.GRPCAddr }

type grpcSessionStoreAdapter struct{ store *session.SessionStore }

func (a *grpcSessionStoreAdapter) GetSession(ctx context.Context, id uuid.UUID) (*session.Session, error) {
	return a.store.GetSession(ctx, id)
}
func (a *grpcSessionStoreAdapter) UpdateState(ctx context.Context, id uuid.UUID, s session.SessionState) error {
	return a.store.UpdateState(ctx, id, s)
}
func (a *grpcSessionStoreAdapter) TransitionSession(ctx context.Context, id uuid.UUID, ts session.SessionState) (*session.Session, error) {
	return a.store.TransitionSession(ctx, id, ts)
}
func (a *grpcSessionStoreAdapter) WriteEventLog(ctx context.Context, e *session.EventLog) error {
	return a.store.WriteEventLog(ctx, e)
}

var _ grpc.SessionStoreInterface = (*grpcSessionStoreAdapter)(nil)

// agentRunnerAdapter implements forge.AgentRunner to run agents via the deploy flow.
type agentRunnerAdapter struct {
	dbPool         *pgxpool.Pool
	streamRegistry stream.StreamRegistryInterface
}

func (a *agentRunnerAdapter) RunAgent(ctx context.Context, agentID, satelliteID uuid.UUID, triggerSource string) error {
	if a.dbPool == nil {
		return fmt.Errorf("database not configured")
	}

	// Get agent definition
	agent, err := database.GetAgentDefinition(ctx, a.dbPool, agentID)
	if err != nil {
		return fmt.Errorf("failed to get agent definition: %w", err)
	}
	if agent == nil {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	// Create a new session for the agent
	sessionID := uuid.New()

	// Create agent run record
	_, err = database.CreateAgentRun(ctx, a.dbPool, agentID, satelliteID, &sessionID, triggerSource)
	if err != nil {
		return fmt.Errorf("failed to create agent run: %w", err)
	}

	// Build agent definition proto
	toolsConfig := make(map[string]string)
	if agent.ToolsConfig != "" {
		json.Unmarshal([]byte(agent.ToolsConfig), &toolsConfig)
	}

	agentDefProto := &proto.AgentDefinitionProto{
		Name:         agent.Name,
		Version:      agent.Version,
		Image:        agent.Name,
		Config:       toolsConfig,
		Capabilities: []string{agent.Type, agent.Category},
		Entrypoint:   agent.Name,
	}

	// Send deploy command to satellite via stream registry
	if a.streamRegistry != nil {
		deployCmd := &proto.NexusMessage{
			Payload: &proto.NexusMessage_DeployAgentCommand{
				DeployAgentCommand: &proto.DeployAgentCommand{
					SessionId:       sessionID.String(),
					AgentDefinition: agentDefProto,
					Secrets:         nil,
				},
			},
		}
		if !a.streamRegistry.SendToSatellite(satelliteID.String(), deployCmd) {
			return fmt.Errorf("failed to send deploy command to satellite %s", satelliteID)
		}
		slog.Info("dispatched deploy agent command", "component", "forge", "agent_id", agentID, "satellite_id", satelliteID, "trigger", triggerSource)
	}

	return nil
}

var _ forge.AgentRunner = (*agentRunnerAdapter)(nil)

func (s *NexusServer) createMTLSConfig() (*tls.Config, error) {
	cert, _ := tls.LoadX509KeyPair(s.config.ServerCert, s.config.ServerKey)
	if cert.Certificate == nil {
		cert = tls.Certificate{}
	}
	caCert, _ := os.ReadFile(s.config.ClientCAs)
	caPool := x509.NewCertPool()
	if len(caCert) > 0 {
		caPool.AppendCertsFromPEM(caCert)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, ClientCAs: caPool, ClientAuth: tls.VerifyClientCertIfGiven, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}, nil
}

func (s *NexusServer) startHTTPServer() error {
	tc, err := s.createMTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to create mTLS config: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handlers.HandleHealth)
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/api/v1/sessions/stream", s.handleSessionStream)
	mux.HandleFunc("/api/v1/transport/cert-hash", s.handleCertHash)
	mux.HandleFunc("/api/v1/notifications/stream", notification.HandleSSEStream(s.sseHub))
	mux.HandleFunc("/api/v1/runs/", api.HandleAgentRunRoutes(s.runEventHub, s.dbPool))
	mux.HandleFunc("/api/v1/auth/cookie", api.HandleSetAuthCookie(s.jwtValidator))
	mux.HandleFunc("/api/v1/sessions/", s.handleSessionsSubpath)
	// Exact-match for /api/v1/sessions (without trailing slash) prevents Go's
	// ServeMux from issuing a 301 redirect to /api/v1/sessions/ — that redirect
	// converts POST to GET and drops the request body, breaking session creation.
	mux.HandleFunc("/api/v1/sessions", s.handleAPI)
	mux.HandleFunc("/api/v1/satellites", s.handleAPI)
	mux.HandleFunc("/api/v1/satellites/", s.handleSatellitesSubpath)
	mux.HandleFunc("/api/v1/", s.handleAPI)

	// Register agent, context, and secret routes
	if s.agentHandler != nil {
		api.RegisterAgentRoutes(mux, s.agentHandler)
		slog.Info("agent routes registered", "component", "api", "features", "versions, import/export")
	}
	if s.contextHandler != nil {
		api.RegisterContextRoutes(mux, s.contextHandler)
		slog.Info("context routes registered", "component", "api")
	}
	if s.secretHandler != nil {
		api.RegisterSecretRoutes(mux, s.secretHandler)
		slog.Info("secret routes registered", "component", "api")
	}
	if s.providerConfigHandler != nil {
		api.RegisterProviderConfigRoutes(mux, s.providerConfigHandler)
		slog.Info("provider config routes registered", "component", "api")
	}
	if s.userHandler != nil {
		api.RegisterUserRoutes(mux, s.userHandler)
		slog.Info("user routes registered", "component", "api")
	}
	if s.authHandler != nil {
		api.RegisterAuthRoutes(mux, s.authHandler)
		slog.Info("local auth routes registered", "component", "api")
	}
	if s.auditHandler != nil {
		api.RegisterAuditRoutes(mux, s.auditHandler)
		slog.Info("audit log routes registered", "component", "api")
	}
	if s.scheduleHandler != nil {
		api.RegisterScheduleRoutes(mux, s.scheduleHandler)
		slog.Info("schedule routes registered", "component", "api")
	}
	if s.pipelineHandler != nil {
		api.RegisterPipelineRoutes(mux, s.pipelineHandler)
		slog.Info("pipeline routes registered", "component", "api")
	}

	caPool := x509.NewCertPool()
	if c, err := os.ReadFile(s.config.ClientCAs); err == nil && len(c) > 0 {
		caPool.AppendCertsFromPEM(c)
	}
	rateLimiter := ha.NewRateLimiter(s.licenseMgr)
	middleware := api.AuthMiddleware(s.jwtValidator, auth.NewSatelliteCertValidator(caPool), rateLimiter, s.dbPool)

	s.httpServer = &http.Server{Addr: s.config.ListenAddr, Handler: metrics.MetricsMiddleware(api.SecurityHeadersMiddleware(api.RequestBodyLimitMiddleware(1 << 20)(middleware(mux)))), TLSConfig: tc}
	go func() {
		slog.Info("starting HTTPS server", "component", "http", "addr", s.config.ListenAddr)
		if err := s.httpServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "component", "http", "error", err)
		}
	}()
	return nil
}

func (s *NexusServer) handleAPI(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimRight(r.URL.Path, "/")
	if p == "" {
		p = "/"
	}
	h := s.handlers
	switch {
	case p == "/api/v1/sessions" && r.Method == "GET":
		h.HandleListSessions(w, r)
	case p == "/api/v1/sessions" && r.Method == "POST":
		h.HandleCreateSession(w, r)
	case strings.HasPrefix(p, "/api/v1/sessions/") && !strings.Contains(strings.TrimPrefix(p, "/api/v1/sessions/"), "/") && r.Method == "GET":
		h.HandleGetSession(w, r, strings.TrimPrefix(p, "/api/v1/sessions/"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/attach") && r.Method == "POST":
		h.HandleAttachSession(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/attach"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/detach") && r.Method == "POST":
		h.HandleDetachSession(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/detach"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/suspend") && r.Method == "POST":
		h.HandleSuspendSession(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/suspend"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/resume") && r.Method == "POST":
		h.HandleResumeSession(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/resume"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/kill") && r.Method == "POST":
		h.HandleKillSession(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/kill"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/preview") && r.Method == "GET":
		h.HandleSessionPreview(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/preview"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/name") && r.Method == "PATCH":
		h.HandleRenameSession(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/name"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && !strings.Contains(strings.TrimPrefix(p, "/api/v1/sessions/"), "/") && r.Method == "DELETE":
		h.HandleDeleteSession(w, r, strings.TrimPrefix(p, "/api/v1/sessions/"))
	case p == "/api/v1/satellites" && r.Method == "GET":
		h.HandleListSatellites(w, r)
	case p == "/api/v1/satellites" && r.Method == "POST":
		h.HandleCreateSatellite(w, r)
	case strings.HasPrefix(p, "/api/v1/satellites/") && strings.HasSuffix(p, "/name") && r.Method == "PATCH":
		satID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/satellites/"), "/name")
		h.HandleRenameSatellite(w, r, satID)
	case strings.HasPrefix(p, "/api/v1/satellites/") && strings.HasSuffix(p, "/tags") && r.Method == "PATCH":
		satID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/satellites/"), "/tags")
		h.HandleUpdateSatelliteTags(w, r, satID)
	case strings.HasPrefix(p, "/api/v1/satellites/") && r.Method == "DELETE":
		h.HandleDeleteSatellite(w, r, strings.TrimPrefix(p, "/api/v1/satellites/"))
	case p == "/api/v1/config" && r.Method == "GET":
		h.HandleGetConfig(w, r)
	case p == "/api/v1/satellites/heartbeat" && r.Method == "POST":
		h.HandleSatelliteHeartbeat(w, r)
	case strings.HasPrefix(p, "/api/v1/satellites/") && strings.HasSuffix(p, "/telemetry/history") && r.Method == "GET":
		satID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/satellites/"), "/telemetry/history")
		h.HandleGetTelemetryHistory(w, r, satID)
	case strings.HasPrefix(p, "/api/v1/satellites/") && strings.HasSuffix(p, "/telemetry") && r.Method == "GET":
		satID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/satellites/"), "/telemetry")
		h.HandleGetTelemetry(w, r, satID)
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/recordings") && r.Method == "GET":
		h.HandleListRecordings(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/recordings"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/recording/start") && r.Method == "POST":
		h.HandleStartRecording(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/recording/start"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/recording/stop") && r.Method == "POST":
		h.HandleStopRecording(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/recording/stop"))
	case strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/recording") && r.Method == "PATCH":
		h.HandleToggleSessionRecording(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/recording"))
	case strings.HasPrefix(p, "/api/v1/recordings/") && strings.HasSuffix(p, "/stream") && r.Method == "GET":
		recID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/recordings/"), "/stream")
		h.HandleStreamRecording(w, r, recID)
	case p == "/api/v1/recordings" && r.Method == "GET":
		h.HandleListAllRecordings(w, r)
	case strings.HasPrefix(p, "/api/v1/recordings/") && !strings.Contains(strings.TrimPrefix(p, "/api/v1/recordings/"), "/") && r.Method == "GET":
		h.HandleGetRecording(w, r, strings.TrimPrefix(p, "/api/v1/recordings/"))
	case p == "/api/v1/config/recording" && r.Method == "GET":
		h.HandleGetRecordingConfig(w, r)
	case p == "/api/v1/config/recording" && r.Method == "PUT":
		h.HandleSetRecordingConfig(w, r)
	case p == "/api/v1/notifications" && r.Method == "GET":
		h.HandleListNotifications(w, r)
	case p == "/api/v1/notifications/unread-count" && r.Method == "GET":
		h.HandleUnreadCount(w, r)
	case p == "/api/v1/notifications/read-all" && r.Method == "POST":
		h.HandleMarkAllRead(w, r)
	case p == "/api/v1/notifications/preferences" && r.Method == "GET":
		h.HandleGetPreferences(w, r)
	case p == "/api/v1/notifications/preferences" && r.Method == "PUT":
		h.HandleUpdatePreferences(w, r)
	case strings.HasPrefix(p, "/api/v1/notifications/") && strings.HasSuffix(p, "/read") && r.Method == "PATCH":
		notifID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/notifications/"), "/read")
		h.HandleMarkRead(w, r, notifID)
	case p == "/api/v1/license" && r.Method == "GET":
		h.HandleLicenseInfo(w, r)
	case p == "/api/v1/proposals" && r.Method == "GET":
		h.HandleListProposals(w, r)
	case p == "/api/v1/proposals/count" && r.Method == "GET":
		h.HandleProposalCount(w, r)
	case strings.HasPrefix(p, "/api/v1/proposals/") && strings.HasSuffix(p, "/approve") && r.Method == "POST":
		h.HandleApproveProposal(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/proposals/"), "/approve"))
	case strings.HasPrefix(p, "/api/v1/proposals/") && strings.HasSuffix(p, "/deny") && r.Method == "POST":
		h.HandleDenyProposal(w, r, strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/proposals/"), "/deny"))
	case strings.HasPrefix(p, "/api/v1/proposals/") && !strings.Contains(strings.TrimPrefix(p, "/api/v1/proposals/"), "/") && r.Method == "GET":
		h.HandleGetProposal(w, r, strings.TrimPrefix(p, "/api/v1/proposals/"))
	default:
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"service":"Nexus API Gateway","version":"0.1.0"}`))
	}
}

func (s *NexusServer) handleSessionStream(w http.ResponseWriter, r *http.Request) {
	transport.NewWebSocketHandler(s.sessionStoreImpl, s.jwtValidator, s.oidcEnabled).HandleSessionStream(w, r)
}

// handleCertHash returns the SHA-256 hash of the server certificate in base64.
// Chrome requires this for WebTransport connections to servers with self-signed
// certificates (via the serverCertificateHashes option).
func (s *NexusServer) handleCertHash(w http.ResponseWriter, r *http.Request) {
	certPEM, err := os.ReadFile(s.config.ServerCert)
	if err != nil {
		http.Error(w, `{"error":"cert not found"}`, http.StatusInternalServerError)
		return
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		http.Error(w, `{"error":"invalid cert PEM"}`, http.StatusInternalServerError)
		return
	}
	hash := sha256.Sum256(block.Bytes)
	b64 := base64.StdEncoding.EncodeToString(hash[:])
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"hash":"%s","algorithm":"sha-256"}`, b64)))
}

// handleSessionsSubpath routes /api/v1/sessions/{id}/... requests.
// Methods that Go 1.22's mux won't route to a subtree handler (e.g. PATCH)
// are handled explicitly here before delegating to handleAPI.
func (s *NexusServer) handleSessionsSubpath(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	slog.Debug("session subpath request", "component", "api", "method", r.Method, "path", p, "upgrade", r.Header.Get("Upgrade"))

	// Terminal WebSocket stream
	if strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/stream") && r.Method == "GET" {
		sessionID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/stream")
		if s.terminalStreamHandler != nil {
			s.terminalStreamHandler.HandleTerminalStream(w, r, sessionID)
		} else {
			http.Error(w, `{"error":"terminal stream not configured"}`, http.StatusServiceUnavailable)
		}
		return
	}

	// PATCH /api/v1/sessions/{id}/name — rename session
	// Handled here because Go 1.22's mux doesn't forward PATCH to subtree handlers.
	if strings.HasPrefix(p, "/api/v1/sessions/") && strings.HasSuffix(p, "/name") && r.Method == "PATCH" {
		sessionID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/v1/sessions/"), "/name")
		s.handlers.HandleRenameSession(w, r, sessionID)
		return
	}

	s.handleAPI(w, r)
}

// handleSatellitesSubpath routes /api/v1/satellites/{id}/... requests.
// Context-related paths (containing /context) are dispatched to the
// ContextHandler; everything else (delete, rename, telemetry, heartbeat)
// goes to handleAPI.
func (s *NexusServer) handleSatellitesSubpath(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	// If the path contains /context, delegate to the context handler
	if s.contextHandler != nil && strings.Contains(p, "/context") {
		s.contextHandler.HandleContextAPI(w, r)
		return
	}
	// Otherwise fall through to the main API handler for satellite CRUD
	s.handleAPI(w, r)
}

func (s *NexusServer) startGRPCServer() error {
	tc, err := s.createMTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to create mTLS config: %w", err)
	}
	s.grpcServer = grpccreds.NewServer(
		grpccreds.Creds(credentials.NewTLS(tc)),
		grpccreds.MaxRecvMsgSize(8*1024*1024), // 8MB — ring buffer replay can be up to 5MB + protobuf overhead
	)
	var ssAdapter grpc.SessionStoreInterface
	if s.sessionStoreImpl != nil {
		ssAdapter = &grpcSessionStoreAdapter{store: s.sessionStoreImpl}
	}

	// Create telemetry handler that forwards to scheduler
	var telemetryHandler func(ctx context.Context, satelliteID string, metrics map[string]float64)
	if s.scheduler != nil || s.leaderGuard != nil {
		telemetryHandler = func(ctx context.Context, satelliteID string, metrics map[string]float64) {
			satUUID, err := uuid.Parse(satelliteID)
			if err != nil {
				slog.Warn("telemetryHandler: skipping non-UUID satellite ID", "satellite_id", satelliteID, "error", err, "component", "grpc")
				return
			}
			report := &forge.TelemetryReport{
				SatelliteID: satUUID,
				Timestamp:   time.Now(),
				Metrics:     metrics,
			}
			sched := s.scheduler
			if s.leaderGuard != nil {
				sched = s.leaderGuard.Scheduler()
			}
			if sched != nil {
				sched.OnTelemetry(ctx, report)
			}
		}
	}

	// Create batch writer for high-frequency agent events (nil in community mode)
	var batchWriter *database.BatchEventWriter
	if s.dbPool != nil {
		batchWriter = database.NewBatchEventWriter(s.dbPool)
	}

	gs := grpc.NewSatelliteGatewayServerImpl(ssAdapter, s.streamRegistry, s.dbPool, s.ringBufferPool, s.recordingPool, s.runEventHub, telemetryHandler, batchWriter)
	proto.RegisterSatelliteGatewayServer(s.grpcServer, gs)
	go func() {
		slog.Info("starting gRPC server", "component", "grpc", "addr", s.config.GRPCAddr)
		if l, err := net.Listen("tcp", s.config.GRPCAddr); err != nil {
			slog.Error("gRPC listener error", "component", "grpc", "error", err)
		} else if err := s.grpcServer.Serve(l); err != nil {
			slog.Error("gRPC server error", "component", "grpc", "error", err)
		}
	}()
	return nil
}

func (s *NexusServer) startWebTransportServer() error {
	wts := transport.NewWebTransportServer(s.streamRouter, &transport.WebTransportConfig{ServerCert: s.config.ServerCert, ServerKey: s.config.ServerKey, ListenAddr: ":8446"})
	if err := wts.StartWebTransportServer(); err != nil {
		return fmt.Errorf("failed to start WebTransport server: %w", err)
	}
	return nil
}

func (s *NexusServer) Start() error {
	s.recoverActiveSessions()
	if err := s.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}
	if err := s.startGRPCServer(); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}
	if err := s.startWebTransportServer(); err != nil {
		return fmt.Errorf("failed to start WebTransport server: %w", err)
	}
	s.startSatelliteLivenessChecker()
	return nil
}

// recoverActiveSessions transitions RUNNING/RE_ATTACHING sessions to DETACHED
// on Nexus startup. After a restart, no browser is attached — the satellite
// daemon will replay its ring buffer when it reconnects, re-populating the
// Nexus-side buffer pool. Sessions in DETACHED state can then be reattached.
func (s *NexusServer) recoverActiveSessions() {
	if s.dbPool == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := s.dbPool.Exec(ctx,
		`UPDATE sessions SET state = 'DETACHED', updated_at = NOW()
		 WHERE state IN ('RUNNING', 'RE_ATTACHING')`)
	if err != nil {
		slog.Error("failed to recover active sessions", "component", "session-recovery", "error", err)
		return
	}
	if result.RowsAffected() > 0 {
		slog.Info("transitioned sessions to DETACHED after restart", "component", "session-recovery", "count", result.RowsAffected())
	}
}

// startSatelliteLivenessChecker runs a background goroutine that marks satellites
// as 'offline' when their heartbeat timestamp is older than 45 seconds.
// Satellites send heartbeats every ~15s; 3 missed heartbeats = offline.
func (s *NexusServer) startSatelliteLivenessChecker() {
	if s.dbPool == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			result, err := s.dbPool.Exec(ctx,
				`UPDATE satellites
				 SET status = 'offline', updated_at = NOW()
				 WHERE status = 'active'
				   AND updated_at < NOW() - INTERVAL '45 seconds'`,
			)
			cancel()
			if err != nil {
				slog.Error("failed to mark stale satellites offline", "component", "satellite-liveness", "error", err)
			} else if result.RowsAffected() > 0 {
				slog.Info("marked satellites offline due to heartbeat timeout", "component", "satellite-liveness", "count", result.RowsAffected())
				// Emit notification for satellite(s) going offline
				if s.notifService != nil {
					s.notifService.Emit(&notification.Event{
						Type:     notification.EventSatelliteOffline,
						Priority: notification.PriorityCritical,
						Title:    "Satellite Offline",
						Body:     fmt.Sprintf("%d satellite(s) went offline (heartbeat timeout)", result.RowsAffected()),
					})
				}
			}
		}
	}()
	slog.Info("satellite liveness checker started", "component", "satellite-liveness", "timeout", "45s", "interval", "15s")
}

func (s *NexusServer) Stop() error {
	startTime := time.Now()
	// 15s hard deadline for overall shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = ctx // kept for potential future use (hard deadline enforcement)

	// Step 1: Log shutdown start with timestamp
	slog.Info("shutting down Nexus server", "component", "shutdown", "time", startTime.Format(time.RFC3339))

	// Step 2: Stop accepting new connections (set ready=false for health check)
	s.ready = false

	// Step 3: Drain NATS connection (if connected)
	if s.natsConn != nil {
		if err := s.natsConn.Drain(); err != nil {
			slog.Warn("NATS drain error on shutdown", "err", err, "component", "ha")
		} else {
			slog.Info("NATS connection drained", "component", "ha", "elapsed_s", fmt.Sprintf("%.2f", time.Since(startTime).Seconds()))
		}
	}

	// Step 4: Drain HTTP server (10s timeout)
	if s.httpServer != nil {
		httpCtx, httpCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := s.httpServer.Shutdown(httpCtx); err != nil {
			slog.Error("HTTP server shutdown error", "component", "shutdown", "error", err)
		}
		httpCancel()
		slog.Info("HTTP server drained", "component", "shutdown", "elapsed_s", fmt.Sprintf("%.2f", time.Since(startTime).Seconds()))
	}

	// Step 4: Stop scheduler (if running)
	if s.leaderGuard != nil {
		s.leaderGuard.Stop()
		slog.Info("scheduler guard stopped", "component", "shutdown", "elapsed_s", fmt.Sprintf("%.2f", time.Since(startTime).Seconds()))
	} else if s.scheduler != nil {
		s.scheduler.Stop()
		slog.Info("scheduler stopped", "component", "shutdown", "elapsed_s", fmt.Sprintf("%.2f", time.Since(startTime).Seconds()))
	}

	// Step 5: GracefulStop gRPC with 10s deadline, fallback to hard Stop if exceeded
	if s.grpcServer != nil {
		grpcCtx, grpcCancel := context.WithTimeout(context.Background(), 10*time.Second)
		done := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
			slog.Info("gRPC server gracefully stopped", "component", "shutdown", "elapsed_s", fmt.Sprintf("%.2f", time.Since(startTime).Seconds()))
		case <-grpcCtx.Done():
			s.grpcServer.Stop()
			slog.Warn("gRPC server force stopped (deadline exceeded)", "component", "shutdown", "elapsed_s", fmt.Sprintf("%.2f", time.Since(startTime).Seconds()))
		}
		grpcCancel()
	}

	// Step 6: Close DB pool if present
	if s.pool != nil {
		s.pool.Close()
		slog.Info("database pool closed", "component", "shutdown", "elapsed_s", fmt.Sprintf("%.2f", time.Since(startTime).Seconds()))
	}

	// Step 7: Log total shutdown duration
	slog.Info("Nexus server shutdown complete", "component", "shutdown", "total_s", fmt.Sprintf("%.2f", time.Since(startTime).Seconds()))

	return nil
}

func (s *NexusServer) Connect(stream proto.SatelliteGateway_ConnectServer) error {
	var ssAdapter grpc.SessionStoreInterface
	if s.sessionStoreImpl != nil {
		ssAdapter = &grpcSessionStoreAdapter{store: s.sessionStoreImpl}
	}
	// Create telemetry handler that forwards to scheduler
	var telemetryHandler func(ctx context.Context, satelliteID string, metrics map[string]float64)
	if s.scheduler != nil || s.leaderGuard != nil {
		telemetryHandler = func(ctx context.Context, satelliteID string, metrics map[string]float64) {
			satUUID, err := uuid.Parse(satelliteID)
			if err != nil {
				slog.Warn("telemetryHandler: skipping non-UUID satellite ID", "satellite_id", satelliteID, "error", err, "component", "grpc")
				return
			}
			report := &forge.TelemetryReport{
				SatelliteID: satUUID,
				Timestamp:   time.Now(),
				Metrics:     metrics,
			}
			sched := s.scheduler
			if s.leaderGuard != nil {
				sched = s.leaderGuard.Scheduler()
			}
			if sched != nil {
				sched.OnTelemetry(ctx, report)
			}
		}
	}
	// Create batch writer for high-frequency agent events (nil in community mode)
	var batchWriter *database.BatchEventWriter
	if s.dbPool != nil {
		batchWriter = database.NewBatchEventWriter(s.dbPool)
	}
	return grpc.NewSatelliteGatewayServerImpl(ssAdapter, s.streamRegistry, s.dbPool, s.ringBufferPool, s.recordingPool, s.runEventHub, telemetryHandler, batchWriter).Connect(stream)
}

var _ proto.SatelliteGatewayServer = (*NexusServer)(nil)

func main() {
	sc, sk, ca := envOr("SERVER_CERT", "certs/server.crt"), envOr("SERVER_KEY", "certs/key.pem"), envOr("CLIENT_CAS", "certs/ca.crt")
	c := &NexusConfig{ServerCert: sc, ServerKey: sk, ClientCAs: ca, ListenAddr: envOr("NEXUS_HTTP_ADDR", ":8443"), GRPCAddr: envOr("NEXUS_GRPC_ADDR", ":8444")}
	var srv *NexusServer

	// Initialize structured logging as the very first thing
	logging.Init()

	if du := os.Getenv("DATABASE_URL"); du != "" {
		slog.Info("connecting to PostgreSQL", "component", "db")
		var pool *pgxpool.Pool
		for i := 1; i <= 5; i++ {
			if p, err := database.NewPool(du); err == nil {
				pool = p
				break
			}
			slog.Warn("DB connect attempt failed", "component", "db", "attempt", i, "max", 5)
			time.Sleep(3 * time.Second)
		}
		if pool == nil {
			slog.Error("could not connect to database after 5 attempts", "component", "db")
			os.Exit(1)
		}
		defer pool.Close()
		slog.Info("PostgreSQL connected", "component", "db")
		if err := db.RunMigrations(context.Background()); err != nil {
			slog.Warn("migration failed", "component", "db", "error", err)
		}

		// Bootstrap owner user if DAAO_OWNER_EMAIL is set and no owner exists
		if ownerEmail := os.Getenv("DAAO_OWNER_EMAIL"); ownerEmail != "" {
			var ownerCount int
			err := pool.QueryRow(context.Background(),
				`SELECT COUNT(*) FROM users WHERE role = 'owner'`).Scan(&ownerCount)
			if err != nil {
				slog.Warn("failed to check for existing owner", "component", "bootstrap", "error", err)
			} else if ownerCount == 0 {
				ownerName := ownerEmail
				if idx := len(ownerEmail) - 1; idx > 0 {
					for i := 0; i < len(ownerEmail); i++ {
						if ownerEmail[i] == '@' {
							ownerName = ownerEmail[:i]
							break
						}
					}
				}

				// Hash owner password (auto-generate if not set)
				ownerPassword := os.Getenv("DAAO_OWNER_PASSWORD")
				if ownerPassword == "" {
					// Generate a random 16-char password
					b := make([]byte, 12)
					rand.Read(b)
					ownerPassword = base64.URLEncoding.EncodeToString(b)[:16]
					slog.Info("=== GENERATED OWNER PASSWORD ===", "component", "bootstrap")
					slog.Info("Owner login credentials", "component", "bootstrap", "email", ownerEmail, "password", ownerPassword)
					slog.Info("Set DAAO_OWNER_PASSWORD in .env to use a fixed password", "component", "bootstrap")
					slog.Info("================================", "component", "bootstrap")
				}

				passwordHash, err := auth.HashPassword(ownerPassword)
				if err != nil {
					slog.Warn("failed to hash owner password", "component", "bootstrap", "error", err)
				} else {
					userStore := auth.NewUserStore(pool)
					_, err := userStore.CreateWithPassword(context.Background(), ownerEmail, ownerName, "owner", passwordHash)
					if err != nil {
						slog.Warn("failed to bootstrap owner user", "component", "bootstrap", "error", err)
					} else {
						slog.Info("owner user created with local auth", "component", "bootstrap", "email", ownerEmail)
					}
				}
			} else {
				slog.Info("owner user already exists, skipping", "component", "bootstrap", "count", ownerCount)
			}
		}

		srv = NewNexusServerWithDB(c, pool)
	} else {
		slog.Info("no DATABASE_URL set, running without database", "component", "db")
		srv = NewNexusServer(c)
	}

	// Set HealthDeps after server construction
	srv.handlers.SetHealthDeps(&api.HealthDeps{
		Pool:      srv.dbPool,
		StartTime: time.Now(),
		Version:   "0.1.0",
		GetSatelliteCount: func() int {
			if srv.streamRegistry != nil {
				return srv.streamRegistry.ConnectedCount()
			}
			return 0
		},
		GetSessionCount: func() int {
			if srv.sessionStore != nil {
				count, _ := srv.sessionStore.CountActiveSessions(context.Background())
				return count
			}
			return 0
		},
	})

	// Configure TimescaleDB continuous aggregate support for telemetry charts
	timescaleEnabled := os.Getenv("TIMESCALEDB_ENABLED") == "true"
	if timescaleEnabled {
		slog.Info("timescaledb: hypertable mode active — using satellite_telemetry_hourly for charts", "component", "db")
	} else {
		slog.Info("timescaledb: standard mode (set TIMESCALEDB_ENABLED=true for hypertable)", "component", "db")
	}
	srv.handlers.SetTimescaleEnabled(timescaleEnabled)

	if err := srv.Start(); err != nil {
		slog.Error("failed to start Nexus server", "error", err)
		os.Exit(1)
	}
	q := make(chan os.Signal, 1)
	signal.Notify(q, syscall.SIGINT, syscall.SIGTERM)
	<-q
	slog.Info("shutdown signal received", "component", "main")
	if err := srv.Stop(); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("Nexus server exited", "component", "main")
}

func envOr(k, f string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return f
}

func getJWTSecret() string {
	env := envOr("DAAO_ENV", "development")
	secret := os.Getenv("JWT_SECRET")

	// Check for insecure defaults
	isInsecure := secret == "" || secret == "default-secret" || secret == "change-me-in-production"

	if isInsecure {
		if env == "production" {
			slog.Error("JWT_SECRET must be set in production environment", "component", "auth")
			os.Exit(1)
		}

		// Generate a random secret for non-production environments
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			slog.Error("failed to generate random JWT secret", "component", "auth", "error", err)
			os.Exit(1)
		}
		generatedSecret := base64.StdEncoding.EncodeToString(b)
		slog.Warn("JWT_SECRET not set or insecure, using randomly generated secret", "component", "auth", "env", env)
		return generatedSecret
	}

	return secret
}
