# fasttunnel

> Expose localhost to the internet in seconds — modern, open-source ngrok alternative.

For walkthroughs and troubleshooting, see the [FastTunnel documentation](https://fasttunnel.dev/docs).

```sh
fasttunnel http 3000
# → Forwarding: https://my-app.fasttunnel.dev → http://localhost:3000
```

---

## Installation

### macOS (Homebrew)

```sh
brew tap fasttunnels/homebrew-fasttunnel
brew install fasttunnels/fasttunnel/fasttunnel
```

### macOS / Linux (curl)

```sh
curl -sSL https://fasttunnel.dev/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://fasttunnel.dev/install.ps1 | iex
```

### Windows (Scoop)

```powershell
scoop bucket add fasttunnel https://github.com/fasttunnels/scoop-fasttunnel
scoop install fasttunnel
```

Or download prebuilt binaries directly from [GitHub Releases](https://github.com/fasttunnels/fasttunnel/releases).

---

## Authentication

FastTunnel supports three flexible authentication methods:

### 1. Interactive Browser Login (Default)

Opens your default web browser to approve your CLI session:

```sh
fasttunnel login
```

You can optionally specify a custom local callback port:

```sh
fasttunnel login --callback-port 43001
```

### 2. Headless Device Login (Remote / SSH / Docker)

Authenticate without a local browser using the RFC 8628 Device Authorization flow (ideal for remote servers, SSH sessions, or containers):

```sh
fasttunnel login --device
# or shorthand:
fasttunnel login -d
```

This displays an 8-character verification code and URL to approve from any phone or browser:

```text
To authenticate FastTunnel CLI on this device:
  1. Visit:      https://fasttunnel.dev/auth/device
  2. Enter code: WDJB-MJHT

Direct link:
  https://fasttunnel.dev/auth/device?user_code=WDJB-MJHT

Waiting for authorization...
```

### 3. Personal Access Token (PAT)

If you have an API token from your FastTunnel dashboard:

```sh
fasttunnel configure ft_sk_your_token_here
```

---

## Creating Tunnels

### HTTP / HTTPS Tunnels

```sh
# Expose local port 3000 over HTTP (with public HTTPS endpoint)
fasttunnel http 3000

# Create a public HTTPS endpoint for the local HTTP app on port 8443
fasttunnel https 8443

# Request a custom vanity subdomain (e.g. https://my-app.fasttunnel.dev)
fasttunnel http 3000 -s my-app

# Equivalent syntax:
fasttunnel http -p 3000
fasttunnel http --port 3000 --subdomain my-app
fasttunnel --protocol http --port 3000
```

### Terminal UI & Headless Logging

By default, `fasttunnel http` launches an interactive terminal dashboard displaying connection state, the public URL, recent requests, status codes, and latency. Press `h` to show the controls: `q` quits, `p` pauses updates, `c` clears retained rows, and `f` cycles the status filter.

To run in plain text / headless mode (e.g. in CI/CD pipelines, Docker, or systemd services):

```sh
fasttunnel http 3000 --no-ui
```

---

## Commands & Options Reference

### Commands

| Command                         | Description                                              |
| :------------------------------ | :------------------------------------------------------- |
| `fasttunnel http <port>`        | Expose local HTTP server                                 |
| `fasttunnel https <port>`       | Expose local HTTPS server                                |
| `fasttunnel login`              | Authenticate CLI via browser or headless device flow     |
| `fasttunnel configure <token>`  | Save personal access token (`ft_sk_...`)                 |
| `fasttunnel completion <shell>` | Generate shell completion script (`zsh`, `bash`, `fish`) |
| `fasttunnel version`            | Show CLI version, commit hash, and build info            |

### Tunnel Options

| Flag          | Short | Default | Description                                                |
| :------------ | :---- | :------ | :--------------------------------------------------------- |
| `--port`      | `-p`  | `8080`  | Local port to forward traffic to                           |
| `--subdomain` | `-s`  | random  | Vanity subdomain (e.g. `my-app` → `my-app.fasttunnel.dev`) |
| `--ui`        |       | `true`  | Enable interactive terminal dashboard                      |
| `--no-ui`     |       | `false` | Disable terminal dashboard and emit raw log lines          |

The dashboard is used only when standard input and output are attached to a terminal. Piped output, CI runners, and service managers automatically receive plain logs.

### Diagnostics Options

| Flag                  | Default  | Description                                 |
| :-------------------- | :------- | :------------------------------------------ |
| `--memstats`          | `false`  | Emit periodic runtime memory snapshots      |
| `--memstats-interval` | `15s`    | Set the memory snapshot interval            |
| `--pprof-addr`        | disabled | Serve Go pprof endpoints on a local address |
| `--cpu-profile`       | disabled | Write a CPU profile for the tunnel lifetime |
| `--heap-profile`      | disabled | Write a heap profile when the CLI exits     |

### Login Options

| Flag              | Short | Default    | Description                                                        |
| :---------------- | :---- | :--------- | :----------------------------------------------------------------- |
| `--device`        | `-d`  | `false`    | Use RFC 8628 headless device code flow (no local browser required) |
| `--callback-port` | `-c`  | `0` (auto) | Local port for OAuth PKCE browser redirect callback                |

### Diagnostics & Profiling

Attach standard Go profiling tools or inspect memory consumption:

```sh
fasttunnel http 3000 \
  --memstats \
  --memstats-interval 5s \
  --pprof-addr 127.0.0.1:6060 \
  --cpu-profile /tmp/fasttunnel.cpu.pprof \
  --heap-profile /tmp/fasttunnel.heap.pprof
```

Inspect profiles using standard `go tool pprof`:

```sh
go tool pprof /tmp/fasttunnel.cpu.pprof
go tool pprof http://127.0.0.1:6060/debug/pprof/heap
```

Bind pprof to a loopback address. Do not expose the profiling server to the public internet.

### Endpoint Lifetime

A randomly assigned subdomain is cleaned up when the CLI exits. A subdomain requested with `--subdomain` remains associated with your account so it can be reused.

Public endpoints use HTTPS. The current agent forwards requests to `http://localhost:<port>`; the `https` command does not establish TLS to the local application.

---

## Shell Autocompletion

Generate shell completion scripts for Bash, Zsh, or Fish:

### Zsh

```sh
# Add to ~/.zshrc:
eval "$(fasttunnel completion zsh)"
```

### Bash

```sh
# Add to ~/.bashrc:
source <(fasttunnel completion bash)
```

### Fish

```sh
fasttunnel completion fish > ~/.config/fish/completions/fasttunnel.fish
```

---

## Configuration & Credentials

Credentials and settings are stored under `~/.fasttunnel/`:

| File                             | Purpose                                                        |
| :------------------------------- | :------------------------------------------------------------- |
| `~/.fasttunnel/config.json`      | Stores long-lived auth token (`ft_sk_...`) and CLI preferences |
| `~/.fasttunnel/credentials.json` | Stores active short-lived JWT access token                     |

---

## Build From Source

Requires **Go 1.24+**:

```sh
git clone https://github.com/fasttunnels/fasttunnel.git
cd fasttunnel/cli
go build -o fasttunnel ./cmd/fasttunnel
```

### Cross-compilation (e.g. Raspberry Pi / ARM)

```sh
# Raspberry Pi / Linux 64-bit ARM
GOOS=linux GOARCH=arm64 go build -o fasttunnel ./cmd/fasttunnel

# Raspberry Pi / Linux 32-bit ARM (ARMv7)
GOOS=linux GOARCH=arm GOARM=7 go build -o fasttunnel ./cmd/fasttunnel
```

---

## Verify Release Binaries

All official release binaries are signed with [cosign](https://docs.sigstore.dev/cosign/overview):

```sh
cosign verify-blob \
  --key cosign.pub \
  --signature fasttunnel_0.1.1_linux_amd64.tar.gz.sig \
  fasttunnel_0.1.1_linux_amd64.tar.gz
```

SHA-256 checksums are also provided in `checksums.txt` with every release.

---

## License

[MIT](./LICENSE)
