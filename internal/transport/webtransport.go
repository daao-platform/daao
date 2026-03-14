package transport

import (
	"fmt"
	"log/slog"

	"github.com/daao/nexus/internal/router"
	"github.com/quic-go/webtransport-go"
)

// WebTransportConfig holds configuration for the WebTransport server
type WebTransportConfig struct {
	ServerCert string
	ServerKey  string
	ListenAddr string
}

// WebTransportServer handles WebTransport connections
type WebTransportServer struct {
	config       *WebTransportConfig
	streamRouter *router.Router
	wtServer     *router.WebTransportServer
}

// NewWebTransportServer creates a new WebTransport server with the given stream router
func NewWebTransportServer(streamRouter *router.Router, config *WebTransportConfig) *WebTransportServer {
	return &WebTransportServer{
		config:       config,
		streamRouter: streamRouter,
	}
}

// StartWebTransportServer starts the WebTransport server for Cockpit client connections
func (s *WebTransportServer) StartWebTransportServer() error {
	// Create WebTransport server using the router package which properly initializes H3
	s.wtServer = router.NewWebTransportServer(&router.WebTransportConfig{
		Addr:    s.config.ListenAddr,
		TLSCert: s.config.ServerCert,
		TLSKey:  s.config.ServerKey,
	})

	// Set connection handler to route through the stream router
	s.wtServer.SetConnectionHandler(func(sessionID string, wtSession *webtransport.Session) error {
		return s.streamRouter.RouteConnection(sessionID, wtSession)
	})

	// Initialize the H3 server
	if err := s.wtServer.Start(); err != nil {
		return fmt.Errorf("failed to start WebTransport server: %w", err)
	}

	// Start serving in a goroutine
	go func() {
		slog.Info(fmt.Sprintf("Starting WebTransport server on %s", s.config.ListenAddr), "component", "transport")
		if err := s.wtServer.Serve(s.config.ServerCert, s.config.ServerKey); err != nil {
			slog.Error(fmt.Sprintf("WebTransport server error: %v", err), "component", "transport")
		}
	}()

	return nil
}
