// Always use chrome.* namespace for consistent callback support across
// Chromium AND Firefox/Zen MV3.
const api = chrome;
const DEFAULT_URL = "http://127.0.0.1:47595";
const DEFAULT_OFFLINE_FOLDER = "RaglexInbox";

function storageGet(defaults) {
  return new Promise((resolve) => api.storage.local.get(defaults, resolve));
}

async function getConfig() {
  const res = await storageGet({
    serverUrl: DEFAULT_URL,
    token: "",
    offlineFolder: DEFAULT_OFFLINE_FOLDER,
    defaultTagId: "",
    defaultProjectId: "",
    defaultBoardId: "",
    autoStack: true,
    autoTag: true,
    cachedMeta: { tags: [], projects: [], boards: [] }
  });
  res.serverUrl = (res.serverUrl || DEFAULT_URL).replace(/\/+$/, "");
  res.offlineFolder = (res.offlineFolder || DEFAULT_OFFLINE_FOLDER)
    .replace(/[\\/:*?"<>|]+/g, "")
    .trim() || DEFAULT_OFFLINE_FOLDER;
  return res;
}

function authHeaders(cfg) {
  const headers = { "Content-Type": "application/json" };
  if (cfg.token) headers["X-Raglex-Token"] = cfg.token;
  return headers;
}

function sanitizeFileStem(raw, fallback = "clip") {
  const stem = (raw || fallback)
    .replace(/[^a-zA-Z0-9_\u4e00-\u9fff\u3040-\u30ff-]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .slice(0, 48);
  return stem || fallback;
}

function guessImageExt(url, fallback = ".jpg") {
  try {
    const p = new URL(url).pathname.toLowerCase();
    const m = p.match(/\.(png|jpe?g|webp|gif|svg|avif|bmp)$/);
    if (m) return m[0] === ".jpeg" ? ".jpg" : m[0];
  } catch {}
  return fallback;
}

function resolveTagNames(cfg, tagIds) {
  if (!tagIds?.length) return [];
  const list = cfg.cachedMeta?.tags || [];
  const names = [];
  for (const id of tagIds) {
    const hit = list.find((t) => (t.ID || t.id) === id);
    if (hit) names.push(hit.Name || hit.name);
  }
  return names;
}

function getTabPageMeta(tab) {
  const fallback = {
    sourceUrl: tab?.url || "",
    sourceTitle: tab?.title || "",
    canonicalUrl: "",
    siteName: "",
    author: "",
    description: "",
    ogImage: "",
    selectedText: "",
    clippedAt: new Date().toISOString()
  };
  if (!tab?.id) return Promise.resolve(fallback);
  return new Promise((resolve) => {
    try {
      api.tabs.sendMessage(tab.id, { type: "GET_PAGE_META" }, (resp) => {
        if (api.runtime.lastError || !resp) {
          resolve(fallback);
          return;
        }
        resolve({
          sourceUrl: resp.sourceUrl || fallback.sourceUrl,
          sourceTitle: resp.sourceTitle || fallback.sourceTitle,
          canonicalUrl: resp.canonicalUrl || "",
          siteName: resp.siteName || "",
          author: resp.author || "",
          description: resp.description || "",
          ogImage: resp.ogImage || "",
          selectedText: resp.selectedText || "",
          clippedAt: resp.clippedAt || fallback.clippedAt
        });
      });
    } catch {
      resolve(fallback);
    }
  });
}

function extractBasename(filePath) {
  if (!filePath) return "";
  const parts = String(filePath).split(/[\\/]/);
  return parts[parts.length - 1] || "";
}

function dataUrlToBlob(dataUrl) {
  const parts = dataUrl.split(",");
  const mime = (parts[0].match(/:(.*?);/) || [])[1] || "application/octet-stream";
  const bin = atob(parts[1] || "");
  const arr = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
  return new Blob([arr], { type: mime });
}

function downloadFile(url, filename) {
  return new Promise((resolve, reject) => {
    if (!api.downloads || typeof api.downloads.download !== "function") {
      reject(new Error("downloads permission unavailable"));
      return;
    }
    api.downloads.download(
      { url, filename, conflictAction: "uniquify", saveAs: false },
      (downloadId) => {
        if (api.runtime.lastError) {
          reject(new Error(api.runtime.lastError.message));
        } else {
          resolve(downloadId);
        }
      }
    );
  });
}

// Wait for a download to finish so we know the EXACT basename the browser wrote
// (even if Chrome/Firefox adjusted the extension from Content-Type or added " (1)").
function waitForDownloadBasename(downloadId, fallbackBase, timeoutMs = 15000) {
  return new Promise((resolve) => {
    if (!downloadId || !api.downloads?.search) {
      resolve(fallbackBase);
      return;
    }
    let settled = false;
    const finish = (base) => {
      if (settled) return;
      settled = true;
      try { api.downloads.onChanged?.removeListener(onChanged); } catch {}
      clearTimeout(timer);
      resolve(base || fallbackBase);
    };
    const lookupName = () => {
      api.downloads.search({ id: downloadId }, (items) => {
        const item = items && items[0];
        const actual = extractBasename(item?.filename);
        if (item?.state === "complete" || item?.state === "interrupted") {
          finish(actual || fallbackBase);
        }
      });
    };
    const onChanged = (delta) => {
      if (!delta || delta.id !== downloadId) return;
      if (delta.state?.current === "complete" || delta.state?.current === "interrupted") {
        lookupName();
      }
    };
    try {
      api.downloads.onChanged?.addListener(onChanged);
    } catch {}
    const timer = setTimeout(() => {
      api.downloads.search({ id: downloadId }, (items) => {
        const actual = extractBasename(items?.[0]?.filename);
        finish(actual || fallbackBase);
      });
    }, timeoutMs);
    // Check immediately in case the download already completed synchronously.
    lookupName();
  });
}

async function downloadTextOrDataUrl(contentOrDataUrl, filename, mime, isDataUrl = false) {
  // Firefox MV3 rejects data: URLs in downloads.download, while Chrome MV3
  // service workers do not have URL.createObjectURL. Support both cleanly!
  if (typeof URL !== "undefined" && typeof URL.createObjectURL === "function") {
    const blob = isDataUrl
      ? dataUrlToBlob(contentOrDataUrl)
      : new Blob([contentOrDataUrl], { type: mime });
    const objUrl = URL.createObjectURL(blob);
    try {
      return await downloadFile(objUrl, filename);
    } finally {
      setTimeout(() => {
        try { URL.revokeObjectURL(objUrl); } catch {}
      }, 15000);
    }
  }
  if (isDataUrl) {
    return downloadFile(contentOrDataUrl, filename);
  }
  const b64 = btoa(unescape(encodeURIComponent(contentOrDataUrl)));
  return downloadFile(`data:${mime};base64,${b64}`, filename);
}

function buildRichMetadataFields(payload, defaults = {}) {
  const sourceUrl = payload.sourceUrl || defaults.sourceUrl || payload.url || "";
  const sourceTitle = payload.sourceTitle || defaults.sourceTitle || "";
  return {
    source_url: sourceUrl,
    source_title: sourceTitle,
    image_url: payload.imageUrl || payload.ogImage || defaults.imageUrl || "",
    canonical_url: payload.canonicalUrl || defaults.canonicalUrl || "",
    site_name: payload.siteName || defaults.siteName || "",
    author: payload.author || defaults.author || "",
    description: payload.description || defaults.description || "",
    alt: payload.alt || defaults.alt || "",
    selected_text: payload.selectedText || defaults.selectedText || "",
    clipped_at: payload.clippedAt || defaults.clippedAt || new Date().toISOString()
  };
}

async function saveOfflineImage(cfg, payload, tags, projectId, boardId) {
  const ts = Date.now();
  const ext = guessImageExt(payload.url, ".jpg");
  const base = `${sanitizeFileStem(payload.sourceTitle, "image")}_${ts}${ext}`;
  const relMedia = `${cfg.offlineFolder}/${base}`;

  const dlId = await downloadFile(payload.url, relMedia);
  const actualBase = await waitForDownloadBasename(dlId, base);
  const actualRelMedia = `${cfg.offlineFolder}/${actualBase}`;
  const relSidecar = `${actualRelMedia}.raglex-clip.json`;

  const meta = buildRichMetadataFields(payload, { imageUrl: payload.url });
  const sidecar = {
    version: 2,
    kind: "image",
    filename: actualBase,
    url: payload.url,
    ...meta,
    notes: payload.notes || "",
    tag_ids: tags,
    tag_names: resolveTagNames(cfg, tags),
    project_id: projectId || "",
    board_id: boardId || ""
  };
  await downloadTextOrDataUrl(JSON.stringify(sidecar, null, 2), relSidecar, "application/json", false);
  return { ok: true, offline: true, filename: actualRelMedia };
}

async function saveOfflineScreenshot(cfg, dataUrl, tab, payload, tags, projectId, boardId) {
  const ts = Date.now();
  const title = payload.sourceTitle || tab?.title || "";
  const base = `${sanitizeFileStem(title, "capture")}_${ts}.png`;
  const relMedia = `${cfg.offlineFolder}/${base}`;

  const dlId = await downloadTextOrDataUrl(dataUrl, relMedia, "image/png", true);
  const actualBase = await waitForDownloadBasename(dlId, base);
  const actualRelMedia = `${cfg.offlineFolder}/${actualBase}`;
  const relSidecar = `${actualRelMedia}.raglex-clip.json`;

  const meta = buildRichMetadataFields(payload, {
    sourceUrl: tab?.url || "",
    sourceTitle: tab?.title || ""
  });
  const sidecar = {
    version: 2,
    kind: "image",
    filename: actualBase,
    ...meta,
    notes: payload.notes || "",
    tag_ids: tags,
    tag_names: resolveTagNames(cfg, tags),
    project_id: projectId || "",
    board_id: boardId || ""
  };
  await downloadTextOrDataUrl(JSON.stringify(sidecar, null, 2), relSidecar, "application/json", false);
  return { ok: true, offline: true, filename: actualRelMedia };
}

async function saveOfflineSource(cfg, payload, tags, projectId, boardId) {
  const ts = Date.now();
  const stem = sanitizeFileStem(payload.sourceTitle, "source");
  const base = `${stem}_${ts}.raglex-clip.json`;
  const relSidecar = `${cfg.offlineFolder}/${base}`;
  const meta = buildRichMetadataFields(payload, { sourceUrl: payload.url });
  const sidecar = {
    version: 2,
    kind: "source",
    url: payload.url,
    ...meta,
    notes: payload.notes || "",
    tag_ids: tags,
    tag_names: resolveTagNames(cfg, tags),
    project_id: projectId || "",
    board_id: boardId || "",
    auto_platform_tag: payload.autoPlatformTag !== undefined ? payload.autoPlatformTag : cfg.autoTag,
    auto_stack: payload.autoStack !== undefined ? payload.autoStack : cfg.autoStack
  };
  await downloadTextOrDataUrl(JSON.stringify(sidecar, null, 2), relSidecar, "application/json", false);
  return { ok: true, offline: true, added: 1, filename: relSidecar };
}

async function importImageURL(payload = {}) {
  const cfg = await getConfig();
  const tags = payload.tagIds?.length ? payload.tagIds : (cfg.defaultTagId ? [cfg.defaultTagId] : []);
  const projectId = payload.projectId !== undefined ? payload.projectId : cfg.defaultProjectId;
  const boardId = payload.boardId !== undefined ? payload.boardId : cfg.defaultBoardId;
  const meta = buildRichMetadataFields(payload, { imageUrl: payload.url });
  try {
    const resp = await fetch(`${cfg.serverUrl}/api/v1/import/url`, {
      method: "POST",
      headers: authHeaders(cfg),
      body: JSON.stringify({
        url: payload.url,
        ...meta,
        notes: payload.notes || "",
        tag_ids: tags,
        project_id: projectId || "",
        board_id: boardId || ""
      })
    });
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
    return data;
  } catch (err) {
    // Seamless Offline Drop Folder fallback when Raglex desktop is closed!
    if (!String(err?.message || "").startsWith("HTTP 4")) {
      return await saveOfflineImage(cfg, payload, tags, projectId, boardId);
    }
    throw err;
  }
}

async function importSourceURL(payload = {}) {
  const cfg = await getConfig();
  const tags = payload.tagIds?.length ? payload.tagIds : (cfg.defaultTagId ? [cfg.defaultTagId] : []);
  const projectId = payload.projectId !== undefined ? payload.projectId : cfg.defaultProjectId;
  const boardId = payload.boardId !== undefined ? payload.boardId : cfg.defaultBoardId;
  const meta = buildRichMetadataFields(payload, { sourceUrl: payload.url });
  try {
    const resp = await fetch(`${cfg.serverUrl}/api/v1/import/source`, {
      method: "POST",
      headers: authHeaders(cfg),
      body: JSON.stringify({
        url: payload.url,
        ...meta,
        notes: payload.notes || "",
        tag_ids: tags,
        project_id: projectId || "",
        board_id: boardId || "",
        auto_platform_tag: payload.autoPlatformTag !== undefined ? payload.autoPlatformTag : cfg.autoTag,
        auto_stack: payload.autoStack !== undefined ? payload.autoStack : cfg.autoStack
      })
    });
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
    return data;
  } catch (err) {
    if (!String(err?.message || "").startsWith("HTTP 4")) {
      return await saveOfflineSource(cfg, payload, tags, projectId, boardId);
    }
    throw err;
  }
}

async function captureVisibleToRaglex(tab, payload = {}) {
  const cfg = await getConfig();
  const dataUrl = await new Promise((resolve, reject) => {
    api.tabs.captureVisibleTab(tab.windowId, { format: "png" }, (url) => {
      if (api.runtime.lastError) reject(new Error(api.runtime.lastError.message));
      else resolve(url);
    });
  });
  const tabMeta = await getTabPageMeta(tab);
  const enrichedPayload = {
    ...tabMeta,
    ...payload,
    sourceUrl: payload.sourceUrl || tabMeta.sourceUrl || tab.url || "",
    sourceTitle: payload.sourceTitle || tabMeta.sourceTitle || tab.title || ""
  };
  const tags = payload.tagIds?.length ? payload.tagIds : (cfg.defaultTagId ? [cfg.defaultTagId] : []);
  const projectId = payload.projectId !== undefined ? payload.projectId : cfg.defaultProjectId;
  const boardId = payload.boardId !== undefined ? payload.boardId : cfg.defaultBoardId;
  const safeTitle = sanitizeFileStem(enrichedPayload.sourceTitle, "capture");
  const meta = buildRichMetadataFields(enrichedPayload);
  try {
    const resp = await fetch(`${cfg.serverUrl}/api/v1/import/base64`, {
      method: "POST",
      headers: authHeaders(cfg),
      body: JSON.stringify({
        filename: `${safeTitle}.png`,
        data_base64: dataUrl,
        ...meta,
        notes: payload.notes || "",
        tag_ids: tags,
        project_id: projectId || "",
        board_id: boardId || ""
      })
    });
    const data = await resp.json().catch(() => ({}));
    if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
    return data;
  } catch (err) {
    if (!String(err?.message || "").startsWith("HTTP 4")) {
      return await saveOfflineScreenshot(cfg, dataUrl, tab, enrichedPayload, tags, projectId, boardId);
    }
    throw err;
  }
}

async function createStack(payload = {}) {
  const cfg = await getConfig();
  const resp = await fetch(`${cfg.serverUrl}/api/v1/stacks`, {
    method: "POST",
    headers: authHeaders(cfg),
    body: JSON.stringify({
      name: payload.name || "Web Gallery",
      asset_ids: payload.assetIds || []
    })
  });
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
  return data;
}

function registerContextMenus() {
  api.contextMenus.removeAll(() => {
    api.contextMenus.create({
      id: "raglex-save-image",
      title: "Save Image to Raglex",
      contexts: ["image"]
    });
    api.contextMenus.create({
      id: "raglex-save-page",
      title: "Save Page / Source to Raglex",
      contexts: ["page", "link"]
    });
    api.contextMenus.create({
      id: "raglex-capture-tab",
      title: "Capture Screenshot to Raglex",
      contexts: ["page"]
    });
  });
}

api.runtime.onInstalled.addListener(registerContextMenus);

api.contextMenus.onClicked.addListener(async (info, tab) => {
  try {
    const tabMeta = await getTabPageMeta(tab);
    if (info.menuItemId === "raglex-save-image" && info.srcUrl) {
      await importImageURL({
        ...tabMeta,
        url: info.srcUrl,
        sourceUrl: info.pageUrl || tabMeta.sourceUrl || tab?.url || "",
        sourceTitle: tabMeta.sourceTitle || tab?.title || ""
      });
    } else if (info.menuItemId === "raglex-save-page") {
      const isLinkClick = !!info.linkUrl && info.linkUrl !== (info.pageUrl || tab?.url);
      const target = info.linkUrl || info.pageUrl || tabMeta.sourceUrl || tab?.url || "";
      if (target) {
        await importSourceURL(
          isLinkClick
            ? {
                url: target,
                sourceUrl: target,
                sourceTitle: info.selectionText || tabMeta.sourceTitle || "",
                clippedAt: new Date().toISOString()
              }
            : {
                ...tabMeta,
                url: target,
                sourceUrl: target,
                sourceTitle: tabMeta.sourceTitle || tab?.title || ""
              }
        );
      }
    } else if (info.menuItemId === "raglex-capture-tab" && tab) {
      await captureVisibleToRaglex(tab, tabMeta);
    }
  } catch (err) {
    console.error("[Raglex Clipper]", err);
  }
});

api.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (!msg || !msg.type) return false;
  (async () => {
    try {
      if (msg.type === "IMPORT_URL") {
        const res = await importImageURL(msg.payload || {});
        sendResponse({ ok: true, res });
      } else if (msg.type === "IMPORT_SOURCE") {
        const res = await importSourceURL(msg.payload || {});
        sendResponse({ ok: true, res });
      } else if (msg.type === "CAPTURE_TAB") {
        const tabs = await new Promise((r) => api.tabs.query({ active: true, currentWindow: true }, r));
        if (!tabs || !tabs[0]) throw new Error("No active tab");
        const res = await captureVisibleToRaglex(tabs[0], msg.payload || {});
        sendResponse({ ok: true, res });
      } else if (msg.type === "CREATE_STACK") {
        const res = await createStack(msg.payload || {});
        sendResponse({ ok: true, res });
      } else {
        sendResponse({ ok: false, error: "unknown message" });
      }
    } catch (err) {
      sendResponse({ ok: false, error: err?.message || String(err) });
    }
  })();
  return true;
});
