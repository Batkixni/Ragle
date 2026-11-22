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
  if (cfg.token) headers["X-Ragle-Token"] = cfg.token;
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

async function captureVisibleToRagle(tab, tagIds) {
  const cfg = await getConfig();
  const dataUrl = await chrome.tabs.captureVisibleTab(tab.windowId, { format: "png" });
  const headers = { "Content-Type": "application/json" };
  if (cfg.token) headers["X-Ragle-Token"] = cfg.token;
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
    id: "ragle-save-image",
    title: "Save Image to Ragle",
    contexts: ["image"]
  });
  chrome.contextMenus.create({
    id: "ragle-capture-tab",
    title: "Capture Screenshot to Ragle",
    contexts: ["page"]
  });
});

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  try {
    if (info.menuItemId === "ragle-save-image" && info.srcUrl) {
      await importImageURL({
        url: info.srcUrl,
        sourceUrl: info.pageUrl || tab?.url || "",
        sourceTitle: tab?.title || ""
      });
    } else if (info.menuItemId === "ragle-capture-tab" && tab) {
      await captureVisibleToRagle(tab, []);
    }
  } catch (err) {
    console.error("[Ragle Clipper]", err);
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
        const res = await captureVisibleToRagle(tabs[0], msg.tagIds || []);
        sendResponse({ ok: true, res });
      } catch (err) {
        sendResponse({ ok: false, error: err.message });
      }
    });
    return true;
  }
});
