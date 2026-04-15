// Package tunnel provides the service layer for creating and registering
// tunnels.  It orchestrates the three-step sequence: CreateTunnel (control
// plane) → CreateLease (control plane) → RegisterEdge (data plane).
package tunnel

import (
	"fmt"

	"github.com/fasttunnels/fasttunnel/internal/agent"
)

// Lease contains every runtime value the CLI needs once a tunnel is active.
type Lease struct {
	SessionID    string
	SessionToken string
	EdgeURL      string
	ExpiresIn    int
	TunnelID     string
	Subdomain    string
	PublicURL    string
	Protocol     string
}

// Service orchestrates tunnel setup using an injected HTTP client.
type Service struct {
	client *agent.Client
}

// New constructs a Service.
func New(client *agent.Client) *Service {
	return &Service{client: client}
}

// Cleanup deletes the tunnel on the control plane, triggering a cascade that
// marks the active session as disconnected and releases the subdomain.
// Should be deferred immediately after CreateAndRegister returns successfully.
func (s *Service) Cleanup(tunnelID, accessToken string) error {
	return s.client.DeleteTunnel(tunnelID, accessToken)
}

// CreateAndRegister runs the full three-step setup and returns a ready Lease.
//
// subdomain may be empty — the control plane will assign a random one.
// accessToken must be a valid bearer token; an empty string returns an error
// with guidance to run `fasttunnel login`.
func (s *Service) CreateAndRegister(subdomain, protocol string, localPort int, accessToken string) (Lease, error) {
	if accessToken == "" {
		return Lease{}, fmt.Errorf("not authenticated — run: fasttunnel login")
	}

	tun, err := s.client.CreateTunnel(subdomain, localPort, protocol, accessToken)
	if err != nil {
		return Lease{}, err
	}

	lease, err := s.client.CreateLease(tun.TunnelID, protocol, accessToken)
	if err != nil {
		return Lease{}, err
	}

	if err := s.client.RegisterEdge(lease, tun.TunnelID, tun.Subdomain, protocol); err != nil {
		return Lease{}, err
	}

	return Lease{
		SessionID:    lease.SessionID,
		SessionToken: lease.SessionToken,
		EdgeURL:      lease.EdgeURL,
		ExpiresIn:    lease.ExpiresIn,
		TunnelID:     tun.TunnelID,
		Subdomain:    tun.Subdomain,
		PublicURL:    tun.PublicURL,
		Protocol:     tun.Protocol,
	}, nil
}
