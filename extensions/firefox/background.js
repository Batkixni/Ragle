const DEFAULT_URL = "http://127.0.0.1:41595";

async function getConfig() {
  const res = await chrome.storage.local.get({
    serverUrl: DEFAULT_URL,
    token: "",
    defaultTagId: ""
  });
  res.serverUrl = (res.serverUrl || DEFAULT_URL).replace(/\/+$/, "");
  return res;
}

async function importImageURL({ url, sourceUrl, sourceTitle, tagIds }) {
  const cfg = await getConfig();
  const headers = { "Content-Type": "application/json" };
  if (cfg.token) headers["X-Raglex-Token"] = cfg.token;
  const tags = tagIds && tagIds.length ? tagIds : (cfg.defaultTagId ? [cfg.defaultTagId] : []);
  const resp = await fetch(`${cfg.serverUrl}/api/v1/import/url`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      url,
      source_url: sourceUrl || url,
      source_title: sourceTitle || "",
      tag_ids: tags
    })
  });
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
  return data;
}

async function captureVisibleToRaglex(tab, tagIds) {
  const cfg = await getConfig();
  const dataUrl = await chrome.tabs.captureVisibleTab(tab.windowId, { format: "png" });
  const headers = { "Content-Type": "application/json" };
  if (cfg.token) headers["X-Raglex-Token"] = cfg.token;
  const tags = tagIds && tagIds.length ? tagIds : (cfg.defaultTagId ? [cfg.defaultTagId] : []);
  const safeTitle = (tab.title || "capture").replace(/[^a-zA-Z0-9_\u4e00-\u9fff-]+/g, "_").slice(0, 48);
  const resp = await fetch(`${cfg.serverUrl}/api/v1/import/base64`, {
    method: "POST",
    headers,
    body: JSON.stringify({
      filename: `${safeTitle || "capture"}.png`,
      data_base64: dataUrl,
      source_url: tab.url || "",
      source_title: tab.title || "",
      tag_ids: tags
    })
  });
  const data = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new Error(data.error || `HTTP ${resp.status}`);
  return data;
}

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: "raglex-save-image",
    title: "Save Image to Raglex",
    contexts: ["image"]
  });
  chrome.contextMenus.create({
    id: "raglex-capture-tab",
    title: "Capture Screenshot to Raglex",
    contexts: ["page"]
  });
});

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  try {
    if (info.menuItemId === "raglex-save-image" && info.srcUrl) {
      await importImageURL({
        url: info.srcUrl,
        sourceUrl: info.pageUrl || tab?.url || "",
        sourceTitle: tab?.title || ""
      });
    } else if (info.menuItemId === "raglex-capture-tab" && tab) {
      await captureVisibleToRaglex(tab, []);
    }
  } catch (err) {
    console.error("[Raglex Clipper]", err);
  }
});

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (msg.type === "IMPORT_URL") {
    importImageURL(msg.payload)
      .then((res) => sendResponse({ ok: true, res }))
      .catch((err) => sendResponse({ ok: false, error: err.message }));
    return true;
  }
  if (msg.type === "CAPTURE_TAB") {
    chrome.tabs.query({ active: true, currentWindow: true }, async (tabs) => {
      try {
        if (!tabs[0]) throw new Error("No active tab");
        const res = await captureVisibleToRaglex(tabs[0], msg.tagIds || []);
        sendResponse({ ok: true, res });
      } catch (err) {
        sendResponse({ ok: false, error: err.message });
      }
    });
    return true;
  }
});
