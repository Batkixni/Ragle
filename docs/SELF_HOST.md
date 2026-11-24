# Self-Hosting Raglex Sync Server

Run your own private Raglex sync server (`raglex-server`) on a NAS, home server, or VPS. Combined with Raglex's **default-ON AES-256-GCM Zero-Knowledge End-to-End Encryption (E2EE)**, your devices sync libraries, tags, folders, ratings, boards, and media files over your own infrastructure with zero third-party dependency.

---

## What You Get

- **Open Sync API v1** — Incremental delta sync with local `remote_known` caching and smart background auto-sync (~150 B idle polling).
- **Zero-Knowledge AES-256-GCM E2EE (Enabled by Default)** — Personal sync filenames, tags, folders, notes, source URLs, and media files are encrypted on-device with a locally generated 256-bit random AES key (`rglx-aes256-...`) before upload.
- **Selective Folder Sync** — Choose exactly which folders sync to your server to conserve disk space.
- **Mobile PWA (`/mobile`)** — Browse and upload reference images from iOS Safari or Android Chrome (`HttpOnly` Access Key protected).
- **Public Share Links (`/s/<id>`)** — Publish read-only folders or boards with strict per-share allowlists.
- **Cross-Network E2E Relay** — Optional blind relay for 6-character P2P device transfers when direct LAN discovery is blocked.
- **Optional S3 / Cloudflare R2 Blob Storage** — Direct AWS SigV4 presigned uploads/downloads for zero-egress object storage.
- **MCP Server (`/mcp`)** — Connect AI coding/design agents to your reference library via bearer token.

---

## Option 1: Docker Compose (Recommended)

### 1. Clone and start

```bash
git clone https://github.com/Batkixni/Raglex.git
cd Raglex/deploy

# Generate a strong bearer token (macOS / Linux)
export RAGLEX_TOKEN="$(openssl rand -hex 32)"

# Start the container on port 47595
docker compose up -d
```

On Windows PowerShell:

```powershell
git clone https://github.com/Batkixni/Raglex.git
cd Raglex\deploy
$env:RAGLEX_TOKEN = -join ((1..32) | ForEach-Object { "{0:x2}" -f (Get-Random -Maximum 256) })
docker compose up -d
```

### 2. Verify health

```bash
curl http://127.0.0.1:47595/api/v1/health
```

Your data persists in `./data/library` on the host.

---

## Option 2: Standalone Binary (No Docker)

Download `raglex-cli-<os>-<arch>.zip` from [GitHub Releases](https://github.com/Batkixni/Raglex/releases) (or compile with `CGO_ENABLED=0 go build ./cmd/raglex-server`) and run:

```bash
./raglex-server \
  -addr ":47595" \
  -library "/var/lib/raglex" \
  -token "your-strong-secret-token"
```

---

## Reverse Proxy with HTTPS (Caddy / Nginx)

Always terminate TLS with HTTPS when exposing your server over the internet.

### Caddy (`Caddyfile`)

```caddyfile
sync.yourdomain.com {
    reverse_proxy 127.0.0.1:47595
}
```

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name sync.yourdomain.com;

    # Allow large video / RAW media uploads
    client_max_body_size 512M;

    location / {
        proxy_pass http://127.0.0.1:47595;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 900s;
        proxy_send_timeout 900s;
    }
}
```

---

## Optional: Cloudflare R2 / S3 / MinIO Blob Storage

Uncomment the S3 variables in `deploy/docker-compose.yml` to store encrypted media blobs in object storage instead of local server disk:

```yaml
environment:
  RAGLEX_TOKEN: "${RAGLEX_TOKEN}"
  RAGLEX_S3_ENDPOINT: "https://<account-id>.r2.cloudflarestorage.com"
  RAGLEX_S3_BUCKET: "raglex-blobs"
  RAGLEX_S3_REGION: "auto"
  RAGLEX_S3_ACCESS_KEY: "<access-key>"
  RAGLEX_S3_SECRET_KEY: "<secret-key>"
```

---

## Connecting Raglex Desktop Clients

1. Open **Raglex → Preferences → Account / Sync → Self-Host**.
2. Enter:
   - **Server URL**: `https://sync.yourdomain.com` (or `http://<lan-ip>:47595` on a private LAN / Tailscale network)
   - **Access Token**: your `RAGLEX_TOKEN`
3. Open **Preferences → Sync & Collab**:
   - **End-to-End Encryption (AES-256-GCM)** is enabled by default with an auto-generated random 256-bit key (`rglx-aes256-...`).
   - Click **Save Key File** (`raglex-e2ee-key-YYYYMMDD.txt`) or **Copy** to back up your key safely.
4. On your **second device**:
   - Enter the same **Server URL** and **Access Token**.
   - Go to **Preferences → Sync & Collab → Load Key File** (or paste your `rglx-aes256-...` key).
   - Click **Sync Now** (or let Smart Auto-Sync pull automatically).

---

## Backup & Upgrading

- **Backup**: Back up the `./data/library` directory (contains `raglex.db`, `sync.db`, and blobs) plus your offline `raglex-e2ee-key-*.txt` recovery file.
- **Upgrade**:
  ```bash
  git pull
  cd deploy
  docker compose up -d --build
  ```
