# Self-Hosting Raglex Sync Server (Docker)

Run your own private Raglex Sync, Mobile PWA, Public Share Links, E2E P2P Signaling/Relay, and MCP server in one command — 100% open source and feature-identical to Raglex Cloud.

## Quick Start

```bash
cd deploy
export RAGLEX_TOKEN="your-strong-secret-token"
docker compose up -d
```

Once running on port `41595`:
- **Desktop Sync URL**: `http://<your-server>:41595` (enter in **Preferences → Sharing & Sync** with your `RAGLEX_TOKEN`)
- **Mobile PWA**: Open `http://<your-server>:41595/mobile` on iOS Safari or Android Chrome → **Add to Home Screen**
- **Public Share Links**: `http://<your-server>:41595/s/<share-id>`
- **Cross-Network E2E Relay**: `http://<your-server>:41595` (enter as Relay Address in Preferences → Sharing → Advanced Network)
- **MCP AI Server**: `http://<your-server>:41595/mcp`

## Optional: Cloudflare R2 / S3 Storage (Zero Egress)

Uncomment the `RAGLEX_S3_*` environment variables in `docker-compose.yml` to enable direct AWS SigV4 presigned URLs for Cloudflare R2, Amazon S3, or MinIO.
