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

---

## Build from source

```sh
git clone https://github.com/fasttunnels/fasttunnel
cd fasttunnel
go build -o fasttunnel ./cmd/fasttunnel
```

Requires Go 1.22+.

---

## Verify a release

All release binaries are signed with [cosign](https://docs.sigstore.dev/cosign/overview).

```sh
cosign verify-blob \
  --signature fasttunnel_linux_amd64.tar.gz.sig \
  fasttunnel_linux_amd64.tar.gz
```

Checksums are in `checksums.txt` attached to each release.

---

## License

[MIT](./LICENSE)
