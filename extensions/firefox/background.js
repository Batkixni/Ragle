// Always use chrome.* namespace for consistent callback support across
// Chromium AND Firefox/Zen MV3.
const api = chrome;
const DEFAULT_URL = "http://127.0.0.1:47595";

function storageGet(defaults) {
  return new Promise((resolve) => api.storage.local.get(defaults, resolve));
}

async function getConfig() {
  const res = await storageGet({
    serverUrl: DEFAULT_URL,
    token: "",
    defaultTagId: "",
    defaultProjectId: "",
    defaultBoardId: "",
    autoStack: true,
    autoTag: true
  });
  res.serverUrl = (res.serverUrl || DEFAULT_URL).replace(/\/+$/, "");
  return res;
}

function authHeaders(cfg) {
  const headers = { "Content-Type": "application/json" };
  if (cfg.token) headers["X-Raglex-Token"] = cfg.token;
  return headers;
}

async function importImageURL(payload = {}) {
  const cfg = await getConfig();
  const tags = payload.tagIds?.length ? payload.tagIds : (cfg.defaultTagId ? [cfg.defaultTagId] : []);
  const projectId = payload.projectId !== undefined ? payload.projectId : cfg.defaultProjectId;
  const boardId = payload.boardId !== undefined ? payload.boardId : cfg.defaultBoardId;
  const resp = await fetch(`${cfg.serverUrl}/api/v1/import/url`, {
    method: "POST",
    headers: authHeaders(cfg),
    body: JSON.stringify({
      url: payload.url,
      source_url: payload.sourceUrl || payload.url,
      source_title: payload.sourceTitle || "",
      notes: payload.notes || "",
      tag_ids: tags,
      project_id: projectId || "",
      board_id: boardId || ""
    })
  });
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
  return data;
}

async function importSourceURL(payload = {}) {
  const cfg = await getConfig();
  const tags = payload.tagIds?.length ? payload.tagIds : (cfg.defaultTagId ? [cfg.defaultTagId] : []);
  const projectId = payload.projectId !== undefined ? payload.projectId : cfg.defaultProjectId;
  const boardId = payload.boardId !== undefined ? payload.boardId : cfg.defaultBoardId;
  const resp = await fetch(`${cfg.serverUrl}/api/v1/import/source`, {
    method: "POST",
    headers: authHeaders(cfg),
    body: JSON.stringify({
      url: payload.url,
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
}

async function captureVisibleToRaglex(tab, payload = {}) {
  const cfg = await getConfig();
  const dataUrl = await new Promise((resolve, reject) => {
    api.tabs.captureVisibleTab(tab.windowId, { format: "png" }, (url) => {
      if (api.runtime.lastError) reject(new Error(api.runtime.lastError.message));
      else resolve(url);
    });
  });
  const tags = payload.tagIds?.length ? payload.tagIds : (cfg.defaultTagId ? [cfg.defaultTagId] : []);
  const projectId = payload.projectId !== undefined ? payload.projectId : cfg.defaultProjectId;
  const boardId = payload.boardId !== undefined ? payload.boardId : cfg.defaultBoardId;
  const safeTitle = (tab.title || "capture").replace(/[^a-zA-Z0-9_\u4e00-\u9fff-]+/g, "_").slice(0, 48);
  const resp = await fetch(`${cfg.serverUrl}/api/v1/import/base64`, {
    method: "POST",
    headers: authHeaders(cfg),
    body: JSON.stringify({
      filename: `${safeTitle || "capture"}.png`,
      data_base64: dataUrl,
      source_url: tab.url || "",
      source_title: tab.title || "",
      notes: payload.notes || "",
      tag_ids: tags,
      project_id: projectId || "",
      board_id: boardId || ""
    })
  });
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
  return data;
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
    if (info.menuItemId === "raglex-save-image" && info.srcUrl) {
      await importImageURL({
        url: info.srcUrl,
        sourceUrl: info.pageUrl || tab?.url || "",
        sourceTitle: tab?.title || ""
      });
    } else if (info.menuItemId === "raglex-save-page") {
      const target = info.linkUrl || info.pageUrl || tab?.url || "";
      if (target) await importSourceURL({ url: target });
    } else if (info.menuItemId === "raglex-capture-tab" && tab) {
      await captureVisibleToRaglex(tab, {});
    }
  } catch (err) {
    console.error("[Raglex Clipper]", err);
  }
});

api.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg.type === "IMPORT_URL") {
    importImageURL(msg.payload)
      .then((res) => sendResponse({ ok: true, res }))
      .catch((err) => sendResponse({ ok: false, error: err.message }));
    return true;
  }
  if (msg.type === "IMPORT_SOURCE") {
    importSourceURL(msg.payload)
      .then((res) => sendResponse({ ok: true, res }))
      .catch((err) => sendResponse({ ok: false, error: err.message }));
    return true;
  }
  if (msg.type === "CREATE_STACK") {
    createStack(msg.payload)
      .then((res) => sendResponse({ ok: true, res }))
      .catch((err) => sendResponse({ ok: false, error: err.message }));
    return true;
  }
  if (msg.type === "CAPTURE_TAB") {
    api.tabs.query({ active: true, currentWindow: true }, async (tabs) => {
      try {
        if (!tabs?.[0]) throw new Error("No active tab");
        const res = await captureVisibleToRaglex(tabs[0], msg.payload || { tagIds: msg.tagIds || [] });
        sendResponse({ ok: true, res });
      } catch (err) {
        sendResponse({ ok: false, error: err.message });
      }
    });
    return true;
  }
});
