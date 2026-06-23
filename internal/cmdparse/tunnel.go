package cmdparse

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseTunnel resolves all supported flag / positional forms for the http and
// https subcommands into a Tunnel struct.
//
// Supported forms (protocol is already known from the caller):
//
//	fasttunnel http  8080
//	fasttunnel https 8080 -s my-app
//	fasttunnel http  -p 8080 -s my-app
//	fasttunnel http  --port 8080 --subdomain my-app
//	fasttunnel http  --port=8080 --subdomain=my-app
func parseTunnel(protocol string, args []string) (Tunnel, error) {
	// Separate any leading positional port from flag tokens.
	// e.g. ["8080", "--subdomain", "foo"] → port=8080, rest=["--subdomain","foo"]
	// e.g. ["-p", "8080"]                 → flag parsing handles it
	positionalPort := 0
	flagArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if p, err := strconv.Atoi(args[0]); err == nil {
			if p < 1 || p > 65535 {
				return Tunnel{}, fmt.Errorf("port %d out of range (1-65535)", p)
			}
			positionalPort = p
			flagArgs = args[1:]
		} else {
			return Tunnel{}, fmt.Errorf("invalid port %q: must be a number", args[0])
		}
	}

	fs := flag.NewFlagSet(protocol, flag.ContinueOnError)
	port := fs.Int("port", 8080, "local port to forward")
	fs.IntVar(port, "p", 8080, "local port to forward (shorthand)")
	subdomain := fs.String("subdomain", "", "optional vanity subdomain")
	fs.StringVar(subdomain, "s", "", "optional vanity subdomain (shorthand)")
	uiEnabled := fs.Bool("ui", true, "enable interactive tunnel dashboard")
	noUI := fs.Bool("no-ui", false, "disable interactive tunnel dashboard")
	memstats := fs.Bool("memstats", false, "emit periodic runtime memory snapshots")
	memstatsInterval := fs.Duration("memstats-interval", 15*time.Second, "interval for memory snapshots")
	pprofAddr := fs.String("pprof-addr", "", "serve Go pprof endpoints on a local address")
	cpuProfile := fs.String("cpu-profile", "", "write a CPU profile to the given file")
	heapProfile := fs.String("heap-profile", "", "write a heap profile to the given file on exit")

	if err := fs.Parse(flagArgs); err != nil {
		return Tunnel{}, err
	}

	// Positional arg wins over the default; explicit --port wins over positional.
	resolvedPort := *port
	if positionalPort != 0 {
		// Only apply positioned value if --port was not explicitly set.
		// flag doesn't expose "was this flag set?", so compare against default.
		if resolvedPort == 8080 {
			resolvedPort = positionalPort
		}
	}

	if resolvedPort < 1 || resolvedPort > 65535 {
		return Tunnel{}, fmt.Errorf("port %d out of range (1-65535)", resolvedPort)
	}

	resolvedUI := *uiEnabled
	if *noUI {
		resolvedUI = false
	}

	return Tunnel{
		Protocol:            strings.ToLower(protocol),
		Port:                resolvedPort,
		Subdomain:           *subdomain,
		UIEnabled:           resolvedUI,
		MemoryStatsEnabled:  *memstats,
		MemoryStatsInterval: *memstatsInterval,
		PprofAddr:           strings.TrimSpace(*pprofAddr),
		CPUProfilePath:      strings.TrimSpace(*cpuProfile),
		HeapProfilePath:     strings.TrimSpace(*heapProfile),
	}, nil
}
