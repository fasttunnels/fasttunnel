package cmdparse

import (
	"fmt"
	"strings"
)

// ParseConfigure parses the `fasttunnel configure <auth_token>` subcommand.
//
// The auth token is a required positional argument. It must start with "ft_sk_".
//
// Usage:
//
//	fasttunnel configure ft_sk_
func ParseConfigure(args []string) (Configure, error) {
	if len(args) == 0 {
		return Configure{}, fmt.Errorf("missing auth token\n\nUsage: fasttunnel configure <auth_token>\n\nObtain a token from: https://app.fasttunnel.dev/cli-access")
	}

	token := strings.TrimSpace(args[0])
	if token == "" {
		return Configure{}, fmt.Errorf("auth token cannot be empty")
	}
	if !strings.HasPrefix(token, "ft_sk_") {
		return Configure{}, fmt.Errorf("invalid auth token format (must start with ft_sk_)")
	}
	if len(token) < 20 {
		return Configure{}, fmt.Errorf("auth token is too short — copy the full token from the dashboard")
	}

	return Configure{AuthToken: token}, nil
}
