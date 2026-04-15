// Package cmdparse handles all CLI argument and flag parsing.
//
// It normalises every supported input form into typed structs so that the
// command implementations in the commands package deal only with business
// logic and never with raw os.Args.
//
// Supported input forms
//
//	Tunnel (http / https subcommands):
//	  fasttunnel http  8080
//	  fasttunnel https 8080 --subdomain my-app
//	  fasttunnel http  -p 8080 -s my-app
//	  fasttunnel http  --port 8080 --subdomain my-app
//	  fasttunnel --protocol http  --port 8080
//	  fasttunnel --protocol=https --port=8080 --subdomain=my-app
//
//	Login:
//	  fasttunnel login
//	  fasttunnel login --callback-port 43001
//	  fasttunnel login -c 43001
package cmdparse

// CommandName identifies which top-level command was parsed.
type CommandName string

const (
	CmdHTTP    CommandName = "http"
	CmdHTTPS   CommandName = "https"
	CmdLogin   CommandName = "login"
	CmdVersion CommandName = "version"
)

// Parsed is the fully resolved, normalised result of a Parse call.
type Parsed struct {
	// Name is the resolved command.
	Name CommandName

	// Tunnel is populated when Name is CmdHTTP or CmdHTTPS.
	Tunnel Tunnel

	// Login is populated when Name is CmdLogin.
	Login Login
}

// Tunnel holds the normalised arguments for an http / https tunnel command.
type Tunnel struct {
	// Protocol is "http" or "https" — always lower-case.
	Protocol string

	// Port is the local TCP port to forward traffic to.
	Port int

	// Subdomain is an optional vanity subdomain.
	// Empty string means the control plane assigns one randomly.
	Subdomain string
}

// Login holds the normalised arguments for the login command.
type Login struct {
	// CallbackPort is the ephemeral local HTTP server port used for the
	// OAuth 2.0 PKCE redirect.  Defaults to 0 (OS-assigned free port).
	CallbackPort int
}
