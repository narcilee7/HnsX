// Package controlplane provides the HnsX control plane server and clients.
package controlplane

// Server is the control plane HTTP/gRPC server.
type Server struct {
	addr string
}

// NewServer creates a control plane server.
func NewServer(addr string) *Server {
	return &Server{addr: addr}
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return s.addr
}
