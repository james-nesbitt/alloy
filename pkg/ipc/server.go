package ipc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/james-nesbitt/alloy/api"
)

// ParseAddress returns the network and address for net.Listen/net.Dial.
func ParseAddress(raw string) (network, address string) {
	if strings.HasPrefix(raw, "unix://") {
		return "unix", strings.TrimPrefix(raw, "unix://")
	}
	if strings.HasPrefix(raw, "tcp://") {
		return "tcp", strings.TrimPrefix(raw, "tcp://")
	}

	// Smart detection for local paths
	if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") {
		return "unix", raw
	}

	return "tcp", raw
}

type Router interface {
	RouteMessage(ctx context.Context, msg api.Message)
	RegisterFrontend(id string, ch chan<- api.Message)
	RegisterFrontendExt(id string, ch chan<- api.Message, headless bool)
}

type Server struct {
	logger *slog.Logger
	router Router
	config *tls.Config

	mu        sync.Mutex
	conns     map[string]*connection
	wg        sync.WaitGroup
	listeners []net.Listener
}

type connection struct {
	id     string
	conn   net.Conn
	enc    *json.Encoder
	dec    *json.Decoder
	sendCh chan api.Message
}

func NewServer(logger *slog.Logger, router Router, tlsConfig *tls.Config) *Server {
	return &Server{
		logger: logger,
		router: router,
		config: tlsConfig,
		conns:  make(map[string]*connection),
	}
}

func (s *Server) ListenAndServe(rawAddr string) error {
	l, err := s.Listen(rawAddr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listeners = append(s.listeners, l)
	s.mu.Unlock()

	return s.Serve()
}

func (s *Server) Listen(rawAddr string) (net.Listener, error) {
	network, addr := ParseAddress(rawAddr)
	if network == "unix" {
		// Ensure directory exists
		dir := filepath.Dir(addr)
		if strings.Contains(dir, "alloy") {
			_ = os.MkdirAll(dir, 0755)
		}
		// Remove existing socket file if it exists
		_ = os.Remove(addr)
	}

	var l net.Listener
	var err error
	if s.config != nil {
		l, err = tls.Listen(network, addr, s.config)
	} else {
		l, err = net.Listen(network, addr)
	}

	if err != nil {
		return nil, err
	}

	s.logger.Info("IPC server listening", "network", network, "addr", addr, "mtls", s.config != nil)
	return l, nil
}

func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.listeners) == 0 {
		return nil
	}
	return s.listeners[0].Addr()
}

func (s *Server) Serve() error {
	// Serve handles all existing and future listeners
	// For now we assume Serve blocks on the first one or we run it in a loop
	// Actually we need to be able to add listeners while serving.

	// Since Serve is expected to block, we'll just wait on a channel
	// and accept connections from all listeners in separate goroutines.

	s.mu.Lock()
	ll := s.listeners
	s.mu.Unlock()

	for _, l := range ll {
		go s.serveListener(l)
	}

	// Wait for stop
	s.wg.Add(1)
	s.wg.Wait()
	return nil
}

func (s *Server) serveListener(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}

		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *Server) AddListener(rawAddr string) error {
	l, err := s.Listen(rawAddr)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.listeners = append(s.listeners, l)
	s.mu.Unlock()

	go s.serveListener(l)
	return nil
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.listeners {
		_ = l.Close()
		if l.Addr().Network() == "unix" {
			_ = os.Remove(l.Addr().String())
		}
	}
	s.listeners = nil
	return nil
}

func (s *Server) handleConn(netConn net.Conn) {
	defer s.wg.Done()
	defer netConn.Close()

	// Identity defaults to remote addr
	clientID := netConn.RemoteAddr().String()

	// If Unix Domain Socket on Linux, extract OS identity
	if peerID, ok := getFormattedPeerIdentity(netConn); ok {
		clientID = peerID
		s.logger.Info("peer credentials detected", "client_id", clientID)
	}

	// If TLS, extract identity from certificate (highest priority)
	if tlsConn, ok := netConn.(*tls.Conn); ok {
		if err := tlsConn.Handshake(); err != nil {
			s.logger.Error("TLS handshake failed", "error", err)
			return
		}
		state := tlsConn.ConnectionState()
		if len(state.PeerCertificates) > 0 {
			clientID = state.PeerCertificates[0].Subject.CommonName
			s.logger.Info("mtls identity verified", "client_id", clientID)
			s.router.RouteMessage(context.Background(), api.Message{
				ID:      "audit-conn-" + clientID,
				Type:    api.TypeEvent,
				Sender:  "ipc-server",
				Target:  "events",
				Method:  "publish",
				Payload: []byte(`{"topic":"system:audit","data":{"actor":"` + clientID + `","action":"connection","status":"success","details":{"type":"mtls"}}}`),
			})
		}
	} else {
		s.router.RouteMessage(context.Background(), api.Message{
			ID:      "audit-conn-" + clientID,
			Type:    api.TypeEvent,
			Sender:  "ipc-server",
			Target:  "events",
			Method:  "publish",
			Payload: []byte(`{"topic":"system:audit","data":{"actor":"` + clientID + `","action":"connection","status":"success","details":{"type":"plain"}}}`),
		})
	}

	s.logger.Info("new connection", "client_id", clientID)

	sendCh := make(chan api.Message, 100)
	c := &connection{
		id:     clientID,
		conn:   netConn,
		enc:    json.NewEncoder(netConn),
		dec:    json.NewDecoder(netConn),
		sendCh: sendCh,
	}

	s.mu.Lock()
	s.conns[clientID] = c
	s.mu.Unlock()

	s.router.RegisterFrontend(clientID, sendCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Read loop
	go func() {
		defer cancel()
		for {
			var msg api.Message
			if err := c.dec.Decode(&msg); err != nil {
				if err != io.EOF {
					s.logger.Error("decode error", "client_id", clientID, "error", err)
				}
				return
			}
			// In plain TCP, we trust the sender for now, but ensure it's set
			if msg.Sender == "" {
				msg.Sender = clientID
			}

			// Capture headless intent from metadata
			headless, _ := msg.Metadata["headless"].(bool)

			if msg.Sender != clientID || headless {
				// Re-register if sender name changed or headless flag is set
				s.router.RegisterFrontendExt(msg.Sender, sendCh, headless)
				// Update clientID for this connection context
				clientID = msg.Sender
			}

			// Security: Set message actor based on verified connection identity
			msg.Actor = clientID

			s.router.RouteMessage(ctx, msg)
		}
	}()

	// Write loop
	for {
		select {
		case msg := <-sendCh:
			s.logger.Debug("sending message to client", "client_id", clientID, "msgID", msg.ID)
			if err := c.enc.Encode(msg); err != nil {
				s.logger.Error("encode error", "client_id", clientID, "error", err)
				return
			}
		case <-ctx.Done():
			s.logger.Info("client disconnected", "client_id", clientID)
			s.mu.Lock()
			delete(s.conns, clientID)
			s.mu.Unlock()
			return
		}
	}
}
