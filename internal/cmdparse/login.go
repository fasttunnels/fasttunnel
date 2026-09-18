package cmdparse

import "flag"

const defaultCallbackPort = 0

// parseLogin resolves arguments for the login subcommand.
//
// Supported forms:
//
//	fasttunnel login
//	fasttunnel login --device
//	fasttunnel login -d
//	fasttunnel login --callback-port 43001
//	fasttunnel login -c 43001
func parseLogin(args []string) (Login, error) {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	callbackPort := fs.Int("callback-port", defaultCallbackPort, "local port for OAuth redirect")
	fs.IntVar(callbackPort, "c", defaultCallbackPort, "local port for OAuth redirect (shorthand)")
	device := fs.Bool("device", false, "authenticate using headless device code flow (RFC 8628)")
	fs.BoolVar(device, "d", false, "authenticate using headless device code flow (shorthand)")

	if err := fs.Parse(args); err != nil {
		return Login{}, err
	}
	return Login{
		CallbackPort: *callbackPort,
		Device:       *device,
	}, nil
}
