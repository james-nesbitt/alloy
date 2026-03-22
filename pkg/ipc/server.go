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
}

type Server struct {
	logger *slog.Logger
	router Router
	config *tls.Config

	mu       sync.Mutex
	conns    map[string]*connection
	wg       sync.WaitGroup
	listener net.Listener
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
	network, addr := ParseAddress(rawAddr)
	if network == "unix" {
		// Ensure directory exists
		dir := filepath.Dir(addr)
		if strings.Contains(dir, "alloy") {
			_ = os.MkdirAll(dir, 0700)
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
		return err
	}
	s.mu.Lock()
	s.listener = l
	s.mu.Unlock()
	s.logger.Info("IPC server listening", "network", network, "addr", addr, "mtls", s.config != nil)

	return s.Serve()
}

func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *Server) Serve() error {
	for {
		s.mu.Lock()
		l := s.listener
		s.mu.Unlock()
		if l == nil {
			return net.ErrClosed
		}
		conn, err := l.Accept()
		if err != nil {
			return err
		}

		s.wg.Add(1)
		go s.handleConn(conn)
	}
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		err := s.listener.Close()
		if s.listener.Addr().Network() == "unix" {
			_ = os.Remove(s.listener.Addr().String())
		}
		s.listener = nil
		return err
	}
	return nil
}

func (s *Server) handleConn(netConn net.Conn) {
	defer s.wg.Done()
	defer netConn.Close()

	// Identity defaults to remote addr
	clientID := netConn.RemoteAddr().String()

	// If TLS, extract identity from certificate
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
			} else if msg.Sender != clientID {
				// Re-register if sender name changed
				s.router.RegisterFrontend(msg.Sender, sendCh)
				// Update clientID for this connection context
				clientID = msg.Sender
			}
			s.router.RouteMessage(ctx, msg)
		}
	}()

	// Write loop
	for {
		select {
		case msg := <-sendCh:
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
