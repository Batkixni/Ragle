# Raglex Plugin SDK & Community Registry Guide

Raglex keeps its core desktop binary fast and focused (~21 MB) while allowing anyone to extend Raglex in **any language** (Go, Python, Node.js, Rust, PowerShell, Bash) through a simple stdin/stdout JSON protocol.

## 1. How a Raglex Plugin Works

Every installed plugin lives in `<LibraryPath>/plugins/<plugin-id>/` and contains a `manifest.json`:

```json
{
  "id": "yourname.my-plugin",
  "name": "My Awesome Plugin",
  "version": "1.0.0",
  "author": "Your Name",
  "description": "What your plugin does in one clear sentence.",
  "command": ["python3", "./main.py"],
  "hooks": ["context_menu", "batch_action"],
  "permissions": ["read_library", "write_tags"]
}
```

When the user triggers your plugin from a right-click menu or batch action, Raglex executes `command` with:
- **Working directory**: `<LibraryPath>/plugins/<plugin-id>/`
- **stdin (JSON)**:
```json
{
  "hook": "context_menu",
  "library_path": "/path/to/Library",
  "server_url": "http://127.0.0.1:41595",
  "token": "<optional-api-token>",
  "assets": [
    {
      "id": "ast_...",
      "name": "poster.png",
      "path": "/path/to/poster.png",
      "mime": "image/png",
      "width": 1920,
      "height": 2560,
      "rating": 5,
      "notes": "...",
      "source_url": "https://...",
      "palette": [{ "Hex": "#FF5C33", "Weight": 0.42 }]
    }
  ]
}
```
- **stdout (JSON)**:
```json
{
  "ok": true,
  "toast": "Processed 3 assets!",
  "reload": true,
  "add_tags": {
    "ast_123": ["Portrait", "Poster"]
  }
}
```

---

## 2. Publishing to the Official Community Marketplace (GitHub PR)

Raglex fetches its in-app Marketplace directory from:
`https://raw.githubusercontent.com/Batkixni/raglex-plugins/main/registry.json`

To publish your plugin so every Raglex user can install it in 1 click:

1. Create a GitHub Release on your plugin repo containing a `.zip` archive with `manifest.json` at its root (or inside a single top-level folder).
2. Compute its SHA-256 checksum (`sha256sum plugin.zip`).
3. Fork `Batkixni/raglex-plugins` and open a Pull Request adding `plugins/<your-plugin-id>/plugin.json`:

```json
{
  "id": "yourname.my-plugin",
  "name": "My Awesome Plugin",
  "version": "1.0.0",
  "author": "Your Name",
  "description": "Export color palettes or inspect EXIF metadata.",
  "category": "export",
  "repository": "https://github.com/yourname/raglex-my-plugin",
  "download_url": "https://github.com/yourname/raglex-my-plugin/releases/download/v1.0.0/plugin.zip",
  "sha256": "<64-hex-sha256-of-zip>",
  "min_raglex_version": "0.2.1",
  "hooks": ["context_menu", "batch_action"],
  "permissions": ["read_library", "disk"]
}
```

4. GitHub Actions validates your JSON, HTTPS download URL, and SHA-256 automatically. Once merged, `registry.json` rebuilds and your plugin appears directly inside **Raglex → Preferences → Plugins → Marketplace**!
