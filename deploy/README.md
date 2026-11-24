# Self-Hosting Raglex Sync Server (Docker)

Run your own private Raglex Sync, Mobile PWA, Public Share Links, E2E P2P Signaling/Relay, and MCP server in one command — 100% open source and self-hosted.

See the full guide in [`docs/SELF_HOST.md`](../docs/SELF_HOST.md) for reverse-proxy HTTPS (Caddy / Nginx), Cloudflare R2 / S3 storage, and zero-knowledge AES-256-GCM key setup.

## Quick Start

```bash
cd deploy
export RAGLEX_TOKEN="your-strong-secret-token"
docker compose up -d
```

Once running on port `47595`:
- **Desktop Sync URL**: `http://<your-server>:47595` (enter in **Preferences → Account / Sync → Self-Host** with your `RAGLEX_TOKEN`)
- **Mobile PWA**: Open `http://<your-server>:47595/mobile` on iOS Safari or Android Chrome → **Add to Home Screen**
- **Public Share Links**: `http://<your-server>:47595/s/<share-id>`
- **Cross-Network E2E Relay**: `http://<your-server>:47595`
- **MCP AI Server**: `http://<your-server>:47595/mcp`
