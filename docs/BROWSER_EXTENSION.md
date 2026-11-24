# Raglex Browser Companion Extension

The **Raglex Web Clipper** lets you save inspiration from any webpage directly into your Raglex library — complete with source URL, page title, tags, folders, projects, and boards — and even works **offline** when Raglex is closed.

Supported browsers:
- **Chromium**: Google Chrome, Microsoft Edge, Brave, Arc, Vivaldi, Opera (`raglex-browser-extension.zip`)
- **Gecko**: Firefox, Zen Browser, LibreWolf (`raglex-firefox-extension.xpi`)

---

## Features

- **Right-Click Save Image**: Right-click any image on a webpage to clip it straight into Raglex with its original URL and page title.
- **Batch Page Collector**: Open the extension popup to scan and batch-import images from the current page into a chosen tag, project, or board.
- **Webpage Bookmarks & Multi-Source Capture**: Save full webpage bookmarks, YouTube/oEmbed metadata, and design references in one click.
- **Offline Clipper Inbox**: Even when Raglex desktop is closed, clipped items are downloaded to your configured Inbox folder and automatically imported the next time Raglex opens.

---

## Step 1 — Enable Raglex Built-in Local Server & Copy Access Key

1. Open **Raglex → Preferences → Sharing / Built-in Server** (or **Advanced**).
2. Enable **Built-in Local Server** (default port `47595`, bound to `127.0.0.1` localhost only).
   - Recommended: enable **Auto-start server on launch**.
3. Click **Copy Access Key** (`ServerToken`).
   - All `/api/v1/*` endpoints require this token, and CORS is strictly locked to browser extensions and localhost for your privacy.

---

## Step 2 — Install the Extension

Download the latest extension package from [GitHub Releases](https://github.com/Batkixni/Raglex/releases):

### Chrome / Edge / Brave / Arc (Chromium)

1. Download `raglex-browser-extension.zip` from the latest Release and **extract** it to a permanent folder (e.g. `~/Raglex-Extension`).
2. Open your browser's Extensions page:
   - Chrome / Brave / Arc: `chrome://extensions`
   - Microsoft Edge: `edge://extensions`
3. Turn on **Developer mode** (top-right toggle).
4. Click **Load unpacked** and select the extracted folder (containing `manifest.json`).
5. Pin **Raglex Web Clipper** to your toolbar.

### Firefox / Zen Browser

1. Download `raglex-firefox-extension.xpi` from the latest Release (or unzip `extensions/firefox/` from the repository).
2. For permanent sideloading on Firefox Developer Edition / Nightly / Zen / LibreWolf (with `xpinstall.signatures.required = false` in `about:config`), drag and drop `raglex-firefox-extension.xpi` directly onto the browser window.
3. Alternatively, for temporary testing in standard Firefox:
   - Navigate to `about:debugging#/runtime/this-firefox`
   - Click **Load Temporary Add-on...** and select `manifest.json` from the extracted extension folder.

---

## Step 3 — Pair with Raglex Desktop

1. Click the **Raglex Web Clipper** icon in your browser toolbar.
2. Verify the local server address is `http://127.0.0.1:47595`.
3. Paste the **Access Key** you copied in Step 1.
4. Once the status indicator turns green (**Connected**), your tags, projects, and boards sync into the popup dropdown immediately.

---

## Optional — Set Up Offline Clipper Inbox

If you want to clip images even while Raglex is closed:
1. In **Raglex → Preferences → Import / Offline Clipper**, choose an **Offline Clipper Inbox Folder** (e.g. `~/Downloads/RaglexInbox`).
2. In the browser extension popup, set the matching download subfolder name (default `RaglexInbox`).
3. Whenever Raglex is offline, the extension automatically falls back to saving images + `.raglex.json` sidecar metadata into that folder, and Raglex ingests them automatically on next launch!
