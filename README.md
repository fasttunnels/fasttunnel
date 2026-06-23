# fasttunnel

> Expose localhost to the internet in seconds — open-source ngrok alternative.

```sh
fasttunnel http 3000
# → https://abc12345.fasttunnel.dev
```

---

## Install

**macOS (Homebrew)**

```sh
brew install fasttunnels/fasttunnel/fasttunnel
```

**macOS / Linux (curl)**

```sh
curl -sSL https://fasttunnel.dev/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://fasttunnel.dev/install.ps1 | iex
```

**Windows (Scoop)**

```powershell
scoop bucket add fasttunnel https://github.com/fasttunnels/scoop-fasttunnel
scoop install fasttunnel
```

Or download a binary directly from [Releases](https://github.com/fasttunnels/fasttunnel/releases).

---

## Usage

```
fasttunnel login                          # authenticate (opens browser)

fasttunnel http  <port>                   # HTTP tunnel
fasttunnel https <port>                   # HTTPS tunnel
fasttunnel http  <port> -s my-app         # with custom subdomain
fasttunnel http 3000 --memstats          # periodic memory snapshots
fasttunnel http 3000 --pprof-addr :6060  # live Go pprof endpoints

# All equivalent:
fasttunnel http 8080
fasttunnel http -p 8080
fasttunnel http --port 8080
fasttunnel --protocol http --port 8080
```

### Options

| Flag          | Short | Default | Description                                                |
| ------------- | ----- | ------- | ---------------------------------------------------------- |
| `--port`      | `-p`  | `8080`  | Local port to forward                                      |
| `--subdomain` | `-s`  | random  | Vanity subdomain (e.g. `my-app` → `my-app.fasttunnel.dev`) |
| `--memstats` |        | off     | Emit periodic runtime memory snapshots                     |
| `--memstats-interval` |        | `15s` | Interval for memory snapshots                     |
| `--pprof-addr` |      | off     | Serve Go `pprof` endpoints locally                         |
| `--cpu-profile` |     | off     | Write a CPU profile while the tunnel runs                  |
| `--heap-profile` |    | off     | Write a heap profile when the tunnel exits                 |

### Diagnostics

Use the tunnel diagnostics flags when you want a quick view of memory footprint
or need to attach standard Go profiling tools:

```sh
fasttunnel http 3000 \
  --memstats \
  --memstats-interval 5s \
  --pprof-addr 127.0.0.1:6060 \
  --cpu-profile /tmp/fasttunnel.cpu.pprof \
  --heap-profile /tmp/fasttunnel.heap.pprof
```

Then inspect profiles with standard Go tooling, for example:

```sh
go tool pprof /tmp/fasttunnel.cpu.pprof
go tool pprof http://127.0.0.1:6060/debug/pprof/heap
```

---

## Build from source

```sh
git clone https://github.com/fasttunnels/fasttunnel
cd fasttunnel
go build -o fasttunnel ./cmd/fasttunnel
```

Requires Go 1.22+.

### Raspberry Pi

The CLI is pure Go and should run on Raspberry Pi as long as you build for the
matching Linux ARM target.

```sh
# Raspberry Pi OS 64-bit
GOOS=linux GOARCH=arm64 go build -o fasttunnel ./cmd/fasttunnel

# Raspberry Pi OS 32-bit (Pi 3 / Pi 4 in 32-bit mode)
GOOS=linux GOARCH=arm GOARM=7 go build -o fasttunnel ./cmd/fasttunnel
```

The repo does not currently document prebuilt Raspberry Pi release artifacts, so
building from source is the safest path today.

---

## Verify a release

All release binaries are signed with [cosign](https://docs.sigstore.dev/cosign/overview).

```sh
# example for linux amd64; swap asset names for your platform
curl -LO https://github.com/fasttunnels/fasttunnel/releases/download/v0.1.1/fasttunnel_0.1.1_linux_amd64.tar.gz
curl -LO https://github.com/fasttunnels/fasttunnel/releases/download/v0.1.1/fasttunnel_0.1.1_linux_amd64.tar.gz.sig

cosign verify-blob \
  --key cosign.pub \
  --signature fasttunnel_0.1.1_linux_amd64.tar.gz.sig \
  fasttunnel_0.1.1_linux_amd64.tar.gz
```

Checksums are in `checksums.txt` attached to each release.

---

## License

[MIT](./LICENSE)
