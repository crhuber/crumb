# crumbd

`crumbd` is the self-hosted sync server for [crumb](../README.md). It stores one versioned, `age`-encrypted blob per **vault** (a vault corresponds to one crumb profile) and lets `crumb sync` push/pull it between machines. It never sees plaintext secrets — the client encrypts before uploading and decrypts after downloading — and authentication is via SSH-signature challenge/response, reusing the same SSH keypair crumb already uses to encrypt/decrypt secrets. No accounts, passwords, or extra credentials to manage.

This is a separate Go module from the `crumb` CLI (see the root `go.work`), so the CLI never pulls in `crumbd`'s SQLite driver, HTTP router, or rate-limiting dependencies.

## How it fits together

- **Vault**: one encrypted blob + version counter. One vault per crumb profile you want to sync.
- **Device**: an SSH public key registered against a vault. Because crumb encrypts to a single recipient key, every machine syncing the same profile shares the same keypair — re-registering an already-known key against a vault is a no-op, not an error.
- **Invite**: a one-time code (from `crumb sync invite`) that lets a second device join a vault without an existing device having to approve it separately.

## What is a vault?

A vault is the top-level thing `crumbd` stores: one database row holding exactly one encrypted blob (the entire secrets store for a profile) plus an integer version counter. Everything else revolves around it:

- **One vault = one crumb profile.** Syncing two profiles ("work", "personal") means two vaults — on the same server or different ones. A profile's `sync.vault_id` config points at which vault it uses.
- **It's not a user account.** There's no login/signup — trust comes purely from an SSH key being a registered device on that vault (via `sync init` creating it, or joining via invite). No registered key, no access.
- **It's the unit of access control and storage.** Devices, invites, sessions, and auth challenges are all scoped by `vault_id` — a device approved on one vault has no access to another, even on the same server.
- **It only understands one blob.** The server has no concept of individual secrets or paths; the finest granularity it ever sees is "the whole encrypted store, at version N." Everything per-secret (which key changed, how to merge) happens client-side, after decryption.
- **One `crumbd` process can host many vaults.** Since everything is scoped by `vault_id`, a single server is multi-tenant by construction — it can serve any number of unrelated profiles at once, not just one.

## How sync works

`crumbd` stores exactly one thing per vault: the current encrypted blob and a version number. There's no per-secret history on the server — the client does all the reconciling. Running `crumb sync` does:

1. **Fetch** the server's current version + blob.
2. **Compare** it to the version this machine saw last time it synced.
   - If it hasn't moved, nobody else has pushed — this machine's local secrets are pushed as-is.
   - If it has moved, the client does a **three-way merge** between *base* (the blob as of this machine's last sync), *local* (its current secrets), and *remote* (the server's current secrets): a key changed on only one side takes that side; a key changed on **both** sides is a real conflict, resolved by whichever entry has the newer `updated` timestamp (a concurrent edit beats a concurrent delete, so a secret is never silently lost).
3. **Push** the (possibly merged) result as a compare-and-swap: `PUT /vaults/{id}/blob` with the version the client started from. If the vault moved again in that instant, the server replies `409` with its current version/blob, and the client re-merges and retries (up to 3 times).
4. **Write back** the merged secrets to local storage, so `get`/`list`/`export` on that machine reflect the result immediately.

All of this happens with the blob already encrypted — `crumbd` receives and returns opaque ciphertext throughout; the merge itself happens client-side after decrypting with the profile's SSH key.

## How invites work

An invite is how a second machine joins an existing vault, with no user accounts on either side:

1. `crumb sync invite` (run on an already-synced machine) asks the server for a fresh invite: a random, high-entropy code. The server stores only its **SHA-256 hash**, along with `max_uses` and an expiry — the plaintext code is returned to the caller exactly once and never persisted.
2. Because joining is scoped to one vault (`POST /vaults/{vault_id}/devices`), the CLI bundles the vault id and the code into a single `<vault_id>.<code>` token, so there's one string to copy to the other machine instead of two separate flags.
3. `crumb sync init --server <url> --invite <token>` on the new machine splits that token apart and presents the code to the join endpoint along with its own public key.
4. The server consumes the code with one atomic conditional update — succeeds only if it exists for that vault, isn't revoked, hasn't expired, and is under `max_uses`. Any failure returns the same generic "invalid or expired invite" error regardless of which condition failed, so a guesser can't use the response to narrow down a valid code. On success, the new device is registered **approved** immediately — no separate manual-approval step.
5. Because crumb encrypts to a single recipient key, every machine syncing one profile presents the *same* SSH fingerprint. If a device with that fingerprint is already registered on the vault, joining is a no-op that returns the existing device without touching the invite at all — so re-running `sync init --invite` on an already-joined machine doesn't burn a use.

## Quick start (local)

```sh
cd crumbd
go build -o crumbd ./cmd/crumbd
./crumbd --config deploy/config.example.yaml   # or omit --config and use defaults + env vars
```

By default it listens on `127.0.0.1:8420`, stores its database at `./crumbd.db`, and allows open vault registration. Check it's up:

```sh
curl http://127.0.0.1:8420/healthz
# {"status":"ok"}
```

Point a crumb profile at it:

```sh
crumb sync init --server http://127.0.0.1:8420
```

## Configuration

`crumbd` reads a YAML file (`--config /path/to/config.yaml`, optional) and then applies `CRUMBD_*` environment variable overrides on top. See [`deploy/config.example.yaml`](deploy/config.example.yaml) for the full annotated list. The main knobs:

| YAML key | Env override | Default | Notes |
|---|---|---|---|
| `listen_addr` | `CRUMBD_LISTEN_ADDR` | `127.0.0.1:8420` | Bind to localhost and put a reverse proxy in front for TLS (below). |
| `database_path` | `CRUMBD_DATABASE_PATH` | `crumbd.db` | SQLite file (WAL mode); the directory must be writable. |
| `registration_mode` | `CRUMBD_REGISTRATION_MODE` | `open` | `open`, `token`, or `closed` — who can create a new vault. Use `token` for a personal server. |
| `registration_token` | `CRUMBD_REGISTRATION_TOKEN` | — | Required bearer token when `registration_mode: token`. Generate with `openssl rand -base64 32`. |
| `max_blob_size` | — | 8 MiB | Cap on a single pushed secrets blob. |
| `session_ttl` | — | `1h` | How long a bearer session lasts before a device must re-authenticate. |
| `challenge_ttl` | — | `2m` | How long an issued auth nonce stays valid. |
| `invite_ttl` | — | `15m` | Default invite code lifetime (a client can request a shorter/longer one). |

Only `listen_addr`, `database_path`, `registration_mode`, and `registration_token` have env var overrides; everything else is YAML-only.

## Deploying on your own server

This is meant for "I have a box I control," not a managed platform — one static binary plus one SQLite file.

### 1. Build

```sh
CGO_ENABLED=0 go build -o crumbd ./cmd/crumbd
```

`CGO_ENABLED=0` works because the SQLite driver (`modernc.org/sqlite`) is pure Go — you can cross-compile from anywhere (`GOOS=linux GOARCH=amd64 go build ...`) without a C toolchain on either end.

### 2. Install

```sh
sudo useradd --system --home /var/lib/crumbd --shell /usr/sbin/nologin crumbd
sudo install -o crumbd -g crumbd -d /var/lib/crumbd
sudo install -m 755 crumbd /usr/local/bin/crumbd
sudo mkdir -p /etc/crumbd
sudo install -m 640 -o crumbd -g crumbd deploy/config.example.yaml /etc/crumbd/config.yaml
# edit /etc/crumbd/config.yaml: set registration_mode + registration_token at minimum
```

### 3. Run it as a service

Copy [`deploy/crumbd.service`](deploy/crumbd.service) to `/etc/systemd/system/crumbd.service`, then:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now crumbd
sudo systemctl status crumbd
journalctl -u crumbd -f
```

The unit runs as a dedicated `crumbd` user with `ProtectSystem=strict` and only `/var/lib/crumbd` writable — the database lives there.

### 4. Put TLS in front of it

`crumbd` itself only speaks plain HTTP and binds to localhost by design — it doesn't handle certificates. Terminate TLS with a reverse proxy. The simplest option is [Caddy](https://caddyserver.com/), which gets you automatic Let's Encrypt certs from a two-line config:

```
# /etc/caddy/Caddyfile
sync.example.com {
    reverse_proxy 127.0.0.1:8420
}
```

An nginx equivalent:

```nginx
server {
    listen 443 ssl;
    server_name sync.example.com;
    ssl_certificate     /etc/letsencrypt/live/sync.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/sync.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8420;
        proxy_set_header Host $host;
    }
}
```

Point your crumb profiles at `https://sync.example.com` from here on.

### 5. Back up the database

Everything crumbd knows is one SQLite file. It's safe to back up live (WAL mode):

```sh
sqlite3 /var/lib/crumbd/crumbd.db ".backup /path/to/backup/crumbd-$(date +%F).db"
```

Put that in cron, or — if you want continuous point-in-time recovery — run [Litestream](https://litestream.io/) against the same file to replicate it to S3-compatible storage. crumbd doesn't need any special support for either; it's just a file.

### 6. Upgrading

Stop the service, replace the binary, start it again. Schema migrations run automatically at startup against the existing database — there's no separate migrate step.

```sh
sudo systemctl stop crumbd
sudo install -m 755 crumbd-new /usr/local/bin/crumbd
sudo systemctl start crumbd
```

## Security notes

- `crumbd` never decrypts anything — it stores and serves opaque ciphertext plus routing metadata (vault/device ids, versions). Losing the database exposes ciphertext and who-has-access metadata, not secrets.
- Set `registration_mode: token` (or `closed` once your vaults exist) unless you genuinely want the server open to anyone who can reach it.
- Invite codes and session tokens are stored only as SHA-256 hashes; the plaintext is shown to the client exactly once.
- A device's SSH signature is verified over a domain-separated payload (`crumbd-auth-v1|<vault_id>|<nonce>`), not the raw nonce, so it can't be confused with a signature made for anything else (SSH login, commit signing, etc.).
- Revoking a device immediately deletes its sessions; it does not retroactively remove data it already contributed to the blob.

## API reference

All endpoints are under `/api/v0`, JSON in and out. See the top-level project docs for the full request/response shapes; in short:

- `POST /vaults` — create a vault + register the first (owner) device.
- `POST /vaults/{id}/invites`, `POST /vaults/{id}/devices` — invite/join flow.
- `GET /vaults/{id}/devices`, `.../devices/{id}/approve`, `.../devices/{id}/revoke` — device management.
- `POST /auth/challenge`, `POST /auth/verify` — SSH-signature login, returns a bearer session token.
- `GET /vaults/{id}/blob`, `PUT /vaults/{id}/blob` — fetch / compare-and-swap the vault's current secrets blob.
- `GET /healthz` — liveness check, no auth.
