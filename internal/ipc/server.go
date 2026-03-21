package ipc

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"sync"
)

// Handler holds callbacks for IPC messages.
type Handler struct {
	OnLaunch func(req LaunchRequest) LaunchResponse
	OnExit   func(notif ExitNotification)
}

// Server listens on a unix socket for IPC messages.
type Server struct {
	socketPath string
	listener   net.Listener
	handler    Handler
	mu         sync.Mutex
	closed     bool
	wg         sync.WaitGroup
}

// NewServer creates a server on the given socket path.
// Stale socket handling: if socket file exists but no listener, removes it and retries.
func NewServer(socketPath string) (*Server, error) {
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		// If the socket file exists but no one is listening, it's stale — remove and retry.
		if errors.Is(err, net.ErrClosed) || isAddrInUse(err) {
			if _, statErr := os.Stat(socketPath); statErr == nil {
				os.Remove(socketPath)
				ln, err = net.Listen("unix", socketPath)
				if err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return &Server{
		socketPath: socketPath,
		listener:   ln,
	}, nil
}

// isAddrInUse checks if the error is "address already in use".
func isAddrInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, errors.New("bind: address already in use")) ||
			opErr.Err.Error() == "bind: address already in use"
	}
	// Fallback: check error string
	return false
}

// SetHandler registers callbacks.
func (s *Server) SetHandler(h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = h
}

// Serve starts accepting connections. Blocks until the server is closed.
func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	var msg Message
	if err := json.NewDecoder(conn).Decode(&msg); err != nil {
		return
	}

	s.mu.Lock()
	handler := s.handler
	s.mu.Unlock()

	switch msg.Type {
	case "launch":
		var req LaunchRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			resp := LaunchResponse{OK: false, Error: "invalid launch request: " + err.Error()}
			json.NewEncoder(conn).Encode(resp)
			return
		}
		if handler.OnLaunch != nil {
			resp := handler.OnLaunch(req)
			json.NewEncoder(conn).Encode(resp)
		} else {
			resp := LaunchResponse{OK: false, Error: "no launch handler registered"}
			json.NewEncoder(conn).Encode(resp)
		}

	case "exit":
		var notif ExitNotification
		if err := json.Unmarshal(msg.Data, &notif); err != nil {
			return
		}
		if handler.OnExit != nil {
			handler.OnExit(notif)
		}
	}
}

// Close stops the server and removes the socket file.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	err := s.listener.Close()
	s.wg.Wait()
	os.Remove(s.socketPath)
	return err
}
