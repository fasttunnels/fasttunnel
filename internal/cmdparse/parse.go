package cmdparse

import (
	"fmt"
	"strings"
)

// Parse is the single entry point for all CLI argument parsing.
// It accepts os.Args[1:] and returns a fully resolved Parsed value.
//
// Top-level dispatch strategy
//
//  1. If the first non-flag token is a known subcommand (http, https, login),
//     it is treated as the command name; remaining tokens go to the
//     subcommand-specific parser.
//
//  2. If no subcommand token is present but --protocol / -P is set anywhere,
//     it acts as an http/https tunnel command and all remaining tokens
//     (minus the protocol flag+value) are forwarded to parseTunnel:
//
//     fasttunnel --protocol https  --port 8080 --subdomain my-app
//     fasttunnel --protocol=https  -p 3000
//     fasttunnel -P http --port 3000
func Parse(args []string) (Parsed, error) {
	if len(args) == 0 {
		return Parsed{}, usageError()
	}

	// ── Fast path: first arg is a known subcommand ─────────────────────────
	switch cmd := strings.ToLower(args[0]); cmd {
	case "http", "https":
		t, err := parseTunnel(cmd, args[1:])
		if err != nil {
			return Parsed{}, err
		}
		return Parsed{Name: CommandName(cmd), Tunnel: t}, nil

	case "login":
		l, err := parseLogin(args[1:])
		if err != nil {
			return Parsed{}, err
		}
		return Parsed{Name: CmdLogin, Login: l}, nil
	}

	// ── Slow path: scan for --protocol / -P without flag.FlagSet ──────────
	// flag.FlagSet stops at the first unknown flag (even with ContinueOnError),
	// so we manually extract the protocol value and leave the rest intact for
	// parseTunnel to handle.
	protocol, rest, found := extractProtocol(args)
	if found {
		p := strings.ToLower(protocol)
		if p != "http" && p != "https" {
			return Parsed{}, fmt.Errorf("--protocol must be \"http\" or \"https\", got %q", protocol)
		}
		t, err := parseTunnel(p, rest)
		if err != nil {
			return Parsed{}, err
		}
		return Parsed{Name: CommandName(p), Tunnel: t}, nil
	}

	return Parsed{}, fmt.Errorf("unknown command %q\n\n%s", args[0], Usage())
}

// extractProtocol scans args for --protocol=val, --protocol val, -P=val, or
// -P val. It returns the value, the remaining args with that flag+value
// removed, and whether the flag was found.
func extractProtocol(args []string) (value string, rest []string, found bool) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		// --protocol=value  or  -P=value
		if strings.HasPrefix(a, "--protocol=") {
			return strings.TrimPrefix(a, "--protocol="), append(rest, args[i+1:]...), true
		}
		if strings.HasPrefix(a, "-P=") {
			return strings.TrimPrefix(a, "-P="), append(rest, args[i+1:]...), true
		}
		// --protocol value  or  -P value
		if (a == "--protocol" || a == "-P") && i+1 < len(args) {
			rest = append(rest, args[i+2:]...)
			return args[i+1], rest, true
		}
		rest = append(rest, a)
	}
	return "", args, false
}

// Usage returns a short usage string suitable for printing to stderr.
func Usage() string {
	return strings.TrimSpace(`
usage:
  fasttunnel http  <port> [-s subdomain]
  fasttunnel https <port> [-s subdomain]
  fasttunnel http  -p <port> [-s <subdomain>]
  fasttunnel https --port <port> [--subdomain <subdomain>]
  fasttunnel --protocol http  --port <port> [--subdomain <subdomain>]
  fasttunnel --protocol https -p <port> [-s <subdomain>]
  fasttunnel login [-c <callback-port>]
`)
}

func usageError() error {
	return fmt.Errorf("no command specified\n\n%s", Usage())
}

