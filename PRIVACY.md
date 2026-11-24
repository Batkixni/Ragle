# Privacy Policy / 隱私權說明 / プライバシーポリシー

**Last updated:** 2026-11-24

**Raglex** is built **100% local-first**. You can use the desktop application completely offline without ever creating an account or connecting to any server.

---

## 1. Zero Telemetry & Zero Tracking
- Raglex collects **no analytics, no usage telemetry, no crash trackers, and no advertising identifiers**.
- Outbound network requests only happen for features you explicitly use or enable:
  - **GitHub Update Check** (`api.github.com`) — can be disabled under `Preferences → General → Auto Updates`.
  - **Plugin Marketplace** (`github.com`) — only fetched when opening the Marketplace tab.
  - **Raglex Cloud / Self-Host Sync** — only active if you sign in or configure a sync server URL.

## 2. Local Security & Companion Server
- Local credentials (`AccountToken`, `SyncToken`, `ServerToken`, and optional E2EE passphrase) are stored locally in `config.json` with owner-only permissions (`0600`).
- When the Built-in Server (`Preferences → Sharing` / `Web Clipper`) is enabled, all private library API endpoints (`/api/v1/*`) require your local **Access Key**, Web Gallery / Mobile PWA pages use `HttpOnly; SameSite=Strict` session cookies, and CORS is strictly restricted to browser extensions and localhost origins.

## 3. Raglex Cloud (`cloud.raglex.sorai.tw`)
If you choose to register and sign in to Raglex Cloud (or you may switch anytime to your own **Self-Host** Docker server):
- **Account Data**: We store your email address, display name, a PBKDF2-SHA256 salted password hash (210,000 iterations), and SHA-256 hashes of active session tokens.
- **Synced Library Data**:
  - **Standard Mode (HTTPS)**: Synced metadata and media blobs are encrypted in transit via HTTPS and stored isolated per user on our storage server (strict per-user ACLs prevent any other tenant from probing or accessing your files).
  - **Zero-Knowledge E2EE Mode (Recommended for sensitive studios)**: Set an optional **E2EE Passphrase** under `Preferences → Sync & Collab`. All personal sync metadata (filenames, tags, folders, notes, URLs) and media files are encrypted on your device with **AES-256-GCM** before upload. The passphrase never leaves your device, so even server operators cannot read your files. *(Note: if you lose your E2EE passphrase, encrypted cloud data cannot be recovered.)*
- **Selective Folder Sync**: You can exclude any local folder (and its subfolders) from cloud upload under `Preferences → Sync & Collab`.
- **Public Share Codes (`SHR-XXXX-XXXX`)**: Only the specific Folder or Project you explicitly publish is accessible to people who hold that share code, enforced by a strict per-share `AllowedHashes` allowlist. You can revoke any share code at any time.
- **Collaboration Spaces (`CLB-XXXX-XXXX`)**: Members of a collaboration space can view shared references and each other's boards (each member can only edit their own whiteboards). Other members' email addresses are masked in API responses (`a***@domain.com`).
- **6-Char Cross-Network Pairing Transfers**: End-to-end encrypted (`DTLS` / `X25519 + AES-GCM`). Relay servers only forward opaque ciphertext and store nothing.

## 4. Data Control & Self-Hosting
- All Raglex server components (`raglex-server`, `raglex-cloud`, `raglex-signal`) are open source under MPL-2.0 — you can self-host your own instance at any time under `Preferences → Account → Self-Host`.
- To request permanent deletion of your Raglex Cloud account and all stored blobs, open an issue or contact the repository maintainer via GitHub (`Batkixni/Raglex`).
