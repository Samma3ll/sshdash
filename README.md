# sshdash

An SSH-first home dashboard built with Go, Bubble Tea, and [Charmbracelet Wish](https://github.com/charmbracelet/wish).

The first slice is intentionally small: run it in Docker, connect over SSH, and see configurable TCP service checks plus HTTP API checks. The code is split so new modules can be added without rewriting the SSH or TUI plumbing.

## Run Locally

```sh
cp config.example.yaml config.yaml
go run ./cmd/sshdash
```

In another terminal:

```sh
ssh localhost -p 23234
```

The first run creates the SSH host key at `.ssh/sshdash_ed25519`.

## Run With Docker

```sh
docker build -t sshdash .
docker run --rm -it -p 23234:23234 --mount type=bind,source="$(pwd)/config.yaml",target=/app/config.yaml,readonly sshdash
```

Connect with:

```sh
ssh localhost -p 23234
```

On PowerShell, prefer `Resolve-Path` so Docker receives an absolute Windows path:

```powershell
docker run --rm -it -p 23234:23234 --mount type=bind,source="$(Resolve-Path .\config.yaml)",target=/app/config.yaml,readonly sshdash
```

Or use Compose:

```sh
docker compose up --build
```

The app reads config once during startup. After editing `config.yaml`, restart the container.

## Configuration

```yaml
server:
  host: 0.0.0.0
  port: 23234
  host_key_path: .ssh/sshdash_ed25519
refresh: 30s
services:
  - name: router
    address: 192.168.1.1:80
apis:
  - name: public-ip
    url: https://api.ipify.org?format=json
    parser: public_ip
  - name: github
    url: https://www.githubstatus.com/api/v2/status.json
    parser: github_status
  - name: homeassistant
    url: http://homeassistant.local:8123
    parser: homeassistant-status
    headers:
      Authorization: Bearer YOUR_LONG_LIVED_ACCESS_TOKEN
  - name: jellyfin
    url: http://jellyfin.local:8096
    parser: jellyfin_status
docker:
  enabled: true
  name: docker
  url: http://192.168.10.21:2375
  timeout: 5s
  show_stopped: true
proxmox:
  enabled: true
  name: proxmox
  url: https://192.168.10.10:8006
  token: "PVEAPIToken=user@pve!sshdash=secret"
  timeout: 8s
  mode: cluster
  nodes: []
  skip_tls_verify: true
proxmox_backup:
  enabled: true
  name: pbs
  url: https://192.168.10.11:8007
  token: "PBSAPIToken=user@pbs!sshdash:secret"
  datastores:
    - datastore-name
  timeout: 8s
  skip_tls_verify: true
weather:
  enabled: true
  name: weather
  location: Amsterdam
  timeout: 5s
```

Durations use Go duration syntax such as `2s`, `30s`, or `1m`.

API parsers are built into the binary and selected with `parser`. Available parsers:

- `default`: shows the HTTP status and a compact JSON or text preview.
- `github_status`: parses GitHub Status API responses and shows the status description plus indicator.
- `homeassistant-status`: treats `url` as the Home Assistant base URL, checks `/api/`, and shows the returned API status message.
- `jellyfin_status`: treats `url` as the Jellyfin base URL, checks `/health`, and shows `Healthy` for a successful status response.
- `public_ip`: parses `{"ip":"..."}` responses and shows `IP: ...`.

Add new API parser modules in Go by implementing a parser in `internal/apiparsers` and registering it by name.

Home Assistant's REST API runs on the same host and port as the frontend, usually `http://IP_ADDRESS:8123`. The `homeassistant-status` parser appends `/api/` itself, so keep `url` as the base URL. Home Assistant normally requires a long-lived access token for `/api/`.

Optional infrastructure modules:

- `docker`: uses the Docker Engine TCP API and renders a `Docker Containers` card.
- `proxmox`: uses the Proxmox VE API and renders `Proxmox Health` plus `Proxmox VMs` cards. Leave `nodes: []` for the whole cluster, or list node names to filter.
- `proxmox_backup`: uses the Proxmox Backup Server API. `PBS Health` shows tasks from the last 24 hours; `PBS Datastore Details` shows datastore usage.
- `weather`: uses `wttr.in` by default and renders the current weather in the top summary bar.

Proxmox VE usually serves its API over HTTPS on port `8006`; PBS usually uses HTTPS on port `8007`. If either uses a self-signed certificate, set `skip_tls_verify: true` for that module.

Proxmox API token headers must use this shape:

```text
PVEAPIToken=user@realm!tokenid=secret
```

For example, if the token ID is `sshdash`, use `user@realm!sshdash=secret`, not `user@realm!sshdash!tokenid=secret`.

PBS API token headers use a colon before the secret:

```text
PBSAPIToken=user@realm!tokenid:secret
```

PBS also checks permissions for the token itself. For each configured datastore, grant the token an audit/read-capable role on `/datastore/<name>`; otherwise the dashboard will show `HTTP 403 Forbidden`.

The `PBS Health` task card reads `/nodes/localhost/tasks` for the last 24 hours. If it shows `no visible tasks`, grant the token node/task visibility, such as `Sys.Audit` on `/system/status`, or use a token that can read PBS task history.

## Structure

- `cmd/sshdash`: process entrypoint and Wish SSH server setup.
- `internal/config`: YAML config loading and defaults.
- `internal/checks`: pluggable status checkers for services and APIs.
- `internal/dashboard`: Bubble Tea model, rendering, and styles.

To add a module, implement `checks.Checker` or create a richer package with its own model and feed its result into the dashboard.
