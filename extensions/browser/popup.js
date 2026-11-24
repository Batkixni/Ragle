// Always use chrome.* namespace so callback-based calls work identically in
// BOTH Chromium (Chrome/Edge/Brave) AND Firefox/Zen/Floorp (where browser.*
// ignores callbacks and only returns Promises!).
const api = chrome;
const DEFAULT_URL = "http://127.0.0.1:47595";

const STRINGS = {
  zh: {
    connected: (n) => `已連線 · ${n}`,
    offline: "離線收件模式",
    offlineBanner: "桌面版未開啟 — 存圖將暫存至 Downloads/RaglexInbox，開啟後自動匯入",
    retry: "重試",
    settings: "設定",
    savePage: "儲存此頁（書籤 / 來源）",
    capture: "擷取畫面截圖",
    scan: "掃描本頁所有圖片",
    destSection: "歸類目標（選填）",
    tag: "標籤",
    project: "專案",
    board: "白板",
    note: "備註",
    notePlaceholder: "靈感、用途或出處備註…",
    noneTag: "不指定標籤",
    noneProject: "不指定專案",
    noneBoard: "不指定白板",
    generalSec: "偏好設定",
    language: "介面語言",
    dragCapsule: "拖拉存圖膠囊",
    autoStack: "多圖自動堆疊",
    autoTag: "自動來源標籤",
    offlineDir: "離線資料夾",
    connSec: "進階連線設定",
    localAddr: "連線位址",
    accessKey: "存取金鑰",
    keyPlaceholder: "選填（若桌面版有設定）",
    connHint: "不開軟體也會自動存到「下載 / 離線資料夾」；開啟 Raglex 偏好設定 → 瀏覽器擴充可即時連線同步。",
    back: "返回",
    testConn: "儲存並測試連線",
    openWeb: "開啟網頁圖庫 ↗",
    selectAll: "全選",
    unselectAll: "取消全選",
    saveSelected: "儲存選取",
    selectedCount: (sel, total) => `已選 ${sel} / ${total} 張`,
    saving: "正在儲存至 Raglex…",
    savedPage: (n, s) => s > 0 ? `✓ 已匯入 ${n} 個素材（${s} 組堆疊）` : `✓ 已儲存至 Raglex（${n}）`,
    savedOffline: "✓ 已存至離線收件資料夾（開啟 Raglex 自動匯入）",
    savedShot: "✓ 截圖已存入 Raglex",
    noImages: "本頁找不到適合擷取的圖片。",
    savedBatch: (ok, fail, stacked) => stacked ? `✓ 已存 ${ok} 張並合併堆疊` : `✓ 已存 ${ok} 張${fail ? `，失敗 ${fail}` : ""}`,
    savedBatchOffline: (ok) => `✓ 已存 ${ok} 張至離線收件資料夾`,
    connOK: "✓ 已成功連線至 Raglex",
    connFail: "桌面版未連線，已切換為離線收件模式",
    badgeWeb: "網頁",
    badgeYT: "YouTube",
    badgeFeed: "RSS / Feed",
    badgeImg: "圖片"
  },
  en: {
    connected: (n) => `Connected · ${n}`,
    offline: "Offline Drop",
    offlineBanner: "Desktop closed — clips save to Downloads/RaglexInbox & auto-import on launch",
    retry: "Retry",
    settings: "Settings",
    savePage: "Save Page (Bookmark / Source)",
    capture: "Capture Visible Screenshot",
    scan: "Scan Page Images",
    destSection: "Destination (Optional)",
    tag: "Tag",
    project: "Project",
    board: "Board",
    note: "Note",
    notePlaceholder: "Inspiration, usage, or source note…",
    noneTag: "No tag",
    noneProject: "No project",
    noneBoard: "No board",
    generalSec: "Preferences",
    language: "Language",
    dragCapsule: "Drag-to-Save",
    autoStack: "Auto-Stack",
    autoTag: "Auto Source Tags",
    offlineDir: "Offline Folder",
    connSec: "Advanced Connection",
    localAddr: "Address",
    accessKey: "Access Key",
    keyPlaceholder: "Optional (from Preferences)",
    connHint: "Works even while Raglex is closed via Downloads/RaglexInbox; open Raglex Preferences → Web Clipper for live sync.",
    back: "Back",
    testConn: "Save & Test",
    openWeb: "Open Web Gallery ↗",
    selectAll: "Select All",
    unselectAll: "Clear",
    saveSelected: "Save Selected",
    selectedCount: (sel, total) => `${sel} / ${total} selected`,
    saving: "Saving to Raglex…",
    savedPage: (n, s) => s > 0 ? `✓ Saved ${n} item(s) (${s} stack)` : `✓ Saved to Raglex (${n})`,
    savedOffline: "✓ Saved to Offline Drop Folder (auto-imports on launch)",
    savedShot: "✓ Screenshot saved to Raglex",
    noImages: "No large images found on this page.",
    savedBatch: (ok, fail, stacked) => stacked ? `✓ Saved ${ok} images into 1 stack` : `✓ Saved ${ok}${fail ? `, ${fail} failed` : ""}`,
    savedBatchOffline: (ok) => `✓ Saved ${ok} images to Offline Drop Folder`,
    connOK: "✓ Connected to Raglex",
    connFail: "Desktop offline — using Offline Drop Mode",
    badgeWeb: "Web",
    badgeYT: "YouTube",
    badgeFeed: "RSS / Feed",
    badgeImg: "Image"
  }
};

const $ = (id) => document.getElementById(id);
const urlEl = $("url");
const tokenEl = $("token");
const offlineFolderEl = $("offlineFolder");
const tagEl = $("tag");
const projEl = $("project");
const boardEl = $("board");
const noteEl = $("note");
const dragEl = $("dragEnabled");
const stackEl = $("autoStack");
const autoTagEl = $("autoTag");
const connPill = $("connPill");
const connText = $("connText");
const offlineBar = $("offlineBar");
const msgEl = $("msg");
const gridEl = $("grid");
const batchWrap = $("batchWrap");
const batchCount = $("batchCount");
const mainView = $("mainView");
const settingsView = $("settingsView");
const settingsBtn = $("settingsBtn");

let lang = (navigator.language || "").toLowerCase().startsWith("zh") ? "zh" : "en";
let inSettings = false;
let activeTab = null;
let scanned = [];
let selectedUrls = new Set();
let pageMeta = { pageUrl: "", title: "" };

function tr(key) {
  return (STRINGS[lang] || STRINGS.en)[key];
}

function setMsg(text, kind = "") {
  msgEl.textContent = text || "";
  msgEl.className = "footer-msg" + (kind ? ` ${kind}` : "");
}

function applyI18n() {
  document.querySelectorAll("[data-i18n]").forEach((el) => {
    const k = el.getAttribute("data-i18n");
    const v = tr(k);
    if (typeof v === "string") el.textContent = v;
  });
  noteEl.placeholder = tr("notePlaceholder");
  tokenEl.placeholder = tr("keyPlaceholder");
  $("offlineText").textContent = tr("offlineBanner");
  $("retryBtn").textContent = tr("retry");
  $("openCfgBtn").textContent = tr("settings");
  $("langZh").classList.toggle("active", lang === "zh");
  $("langEn").classList.toggle("active", lang === "en");
  updatePageBadge();
  updateSelectDefaults();
}

function updateSelectDefaults() {
  if (tagEl.options.length === 0) tagEl.innerHTML = `<option value="">${tr("noneTag")}</option>`;
  else tagEl.options[0].textContent = tr("noneTag");
  if (projEl.options.length === 0) projEl.innerHTML = `<option value="">${tr("noneProject")}</option>`;
  else projEl.options[0].textContent = tr("noneProject");
  if (boardEl.options.length === 0) boardEl.innerHTML = `<option value="">${tr("noneBoard")}</option>`;
  else boardEl.options[0].textContent = tr("noneBoard");
}

function detectSourceKind(rawUrl) {
  if (!rawUrl) return "web";
  try {
    const u = new URL(rawUrl);
    const h = u.hostname.toLowerCase().replace(/^www\./, "");
    const p = u.pathname.toLowerCase();
    if (h === "youtube.com" || h === "m.youtube.com" || h === "youtu.be" || h === "music.youtube.com") {
      return p.includes("/feeds/videos.xml") ? "feed" : "yt";
    }
    if (p.endsWith(".xml") || p.endsWith(".rss") || p.endsWith(".atom") || p.endsWith("/feed") || p.endsWith("/rss")) {
      return "feed";
    }
    if (/\.(png|jpe?g|webp|gif|svg|avif|bmp)$/i.test(p)) return "img";
  } catch {}
  return "web";
}

function updatePageBadge() {
  const badge = $("sourceBadge");
  const kind = detectSourceKind(activeTab?.url || "");
  badge.className = "badge";
  if (kind === "yt") {
    badge.textContent = tr("badgeYT");
    badge.classList.add("yt");
    $("savePageMeta").textContent = "HD";
  } else if (kind === "feed") {
    badge.textContent = tr("badgeFeed");
    badge.classList.add("feed");
    $("savePageMeta").textContent = "Feed";
  } else if (kind === "img") {
    badge.textContent = tr("badgeImg");
    $("savePageMeta").textContent = "";
  } else {
    badge.textContent = tr("badgeWeb");
    $("savePageMeta").textContent = "";
  }
}

function toggleSettings(show) {
  inSettings = typeof show === "boolean" ? show : !inSettings;
  mainView.classList.toggle("hidden", inSettings);
  settingsView.classList.toggle("hidden", !inSettings);
  settingsBtn.classList.toggle("active", inSettings);
}

function sendMsg(message) {
  return new Promise((resolve) => {
    api.runtime.sendMessage(message, (resp) => resolve(resp));
  });
}

function sendTabMsg(tabId, message) {
  return new Promise((resolve) => {
    api.tabs.sendMessage(tabId, message, (resp) => {
      if (api.runtime.lastError) resolve(null);
      else resolve(resp);
    });
  });
}

function storageGet(defaults) {
  return new Promise((resolve) => api.storage.local.get(defaults, resolve));
}

function storageSet(vals) {
  return new Promise((resolve) => api.storage.local.set(vals, resolve));
}

async function savePrefs() {
  await storageSet({
    serverUrl: (urlEl.value || DEFAULT_URL).trim().replace(/\/+$/, ""),
    token: tokenEl.value.trim(),
    offlineFolder: (offlineFolderEl.value || "RaglexInbox").trim(),
    defaultTagId: tagEl.value,
    defaultProjectId: projEl.value,
    defaultBoardId: boardEl.value,
    dragEnabled: dragEl.checked,
    autoStack: stackEl.checked,
    autoTag: autoTagEl.checked,
    lang
  });
}

async function probeRaglexHealth(base) {
  const r = await fetch(`${base}/api/v1/health`);
  const d = await r.json();
  if (!r.ok || !d.ok || d.app !== "raglex") throw new Error("not raglex");
  return d;
}

async function checkConnection(showToast = false) {
  let base = (urlEl.value || DEFAULT_URL).trim().replace(/\/+$/, "");
  const headers = {};
  if (tokenEl.value.trim()) headers["X-Raglex-Token"] = tokenEl.value.trim();

  let lastErr = "";
  try {
    let d = null;
    try {
      d = await probeRaglexHealth(base);
    } catch (e) {
      lastErr = e?.message || String(e);
      // Auto-discover Raglex on dedicated ports 47595..47605 (also migrates old 41595 configs).
      if (/^https?:\/\/(127\.0\.0\.1|localhost)(:\d+)?$/i.test(base)) {
        for (let p = 47595; p <= 47605; p++) {
          const cand = `http://127.0.0.1:${p}`;
          if (cand === base) continue;
          try {
            d = await probeRaglexHealth(cand);
            base = cand;
            urlEl.value = cand;
            await storageSet({ serverUrl: cand });
            break;
          } catch {}
        }
      }
      if (!d) throw new Error(lastErr || "offline");
    }

    connPill.className = "conn-pill ok";
    connText.textContent = tr("connected")(d.assets || 0);
    offlineBar.classList.remove("show");

    const [trResp, prResp, brResp] = await Promise.all([
      fetch(`${base}/api/v1/tags`, { headers }).then((x) => x.json()).catch(() => ({})),
      fetch(`${base}/api/v1/projects`, { headers }).then((x) => x.json()).catch(() => ({})),
      fetch(`${base}/api/v1/boards`, { headers }).then((x) => x.json()).catch(() => ({}))
    ]);

    const tags = trResp.tags || [];
    const projects = prResp.projects || [];
    const boards = brResp.boards || [];
    populateSelect(tagEl, tr("noneTag"), tags);
    populateSelect(projEl, tr("noneProject"), projects);
    populateSelect(boardEl, tr("noneBoard"), boards);
    await storageSet({ cachedMeta: { tags, projects, boards } });

    if (showToast) setMsg(tr("connOK"), "ok");
    return true;
  } catch (err) {
    connPill.className = "conn-pill off";
    connText.textContent = tr("offline");
    offlineBar.classList.add("show");
    // Restore last known tags/projects/boards so user can still organize clips offline!
    const st = await storageGet({ cachedMeta: { tags: [], projects: [], boards: [] } });
    if (st.cachedMeta) {
      populateSelect(tagEl, tr("noneTag"), st.cachedMeta.tags || []);
      populateSelect(projEl, tr("noneProject"), st.cachedMeta.projects || []);
      populateSelect(boardEl, tr("noneBoard"), st.cachedMeta.boards || []);
    }
    if (showToast) setMsg(tr("connFail"), "");
    return false;
  }
}

function populateSelect(sel, emptyLabel, list) {
  const cur = sel.value;
  sel.innerHTML = `<option value="">${emptyLabel}</option>`;
  for (const item of list) {
    const opt = document.createElement("option");
    opt.value = item.ID || item.id || "";
    opt.textContent = item.Name || item.name || "";
    if (opt.value) sel.appendChild(opt);
  }
  if (cur) sel.value = cur;
}

function currentDestPayload() {
  return {
    tagIds: tagEl.value ? [tagEl.value] : [],
    projectId: projEl.value || "",
    boardId: boardEl.value || "",
    notes: noteEl.value.trim(),
    autoStack: stackEl.checked,
    autoPlatformTag: autoTagEl.checked
  };
}

// Event bindings
settingsBtn.addEventListener("click", () => toggleSettings());
connPill.addEventListener("click", () => toggleSettings(true));
$("openCfgBtn").addEventListener("click", () => toggleSettings(true));
$("backMainBtn").addEventListener("click", () => toggleSettings(false));
$("retryBtn").addEventListener("click", () => checkConnection(true));

$("langZh").addEventListener("click", async () => { lang = "zh"; applyI18n(); await savePrefs(); });
$("langEn").addEventListener("click", async () => { lang = "en"; applyI18n(); await savePrefs(); });

[tagEl, projEl, boardEl, dragEl, stackEl, autoTagEl].forEach((el) => {
  el.addEventListener("change", savePrefs);
});

$("testConnBtn").addEventListener("click", async () => {
  await savePrefs();
  const ok = await checkConnection(true);
  if (ok) setTimeout(() => toggleSettings(false), 450);
});

$("openGallery").addEventListener("click", () => {
  const base = (urlEl.value || DEFAULT_URL).trim().replace(/\/+$/, "");
  api.tabs.create({ url: base });
});

// 1. Save Current Page (Multi-Source Bookmark / YouTube / oEmbed / JSON-LD / RSS)
$("savePageBtn").addEventListener("click", async () => {
  if (!activeTab?.url) return;
  await savePrefs();
  setMsg(tr("saving"));
  const btn = $("savePageBtn");
  btn.disabled = true;
  const resp = await sendMsg({
    type: "IMPORT_SOURCE",
    payload: {
      url: activeTab.url,
      ...currentDestPayload()
    }
  });
  btn.disabled = false;
  if (resp?.ok) {
    if (resp.res?.offline) {
      setMsg(tr("savedOffline"), "ok");
    } else {
      const added = resp.res?.added ?? 1;
      const stacks = resp.res?.stacks ?? 0;
      setMsg(tr("savedPage")(added, stacks), "ok");
      checkConnection(false);
    }
    noteEl.value = "";
  } else {
    setMsg(resp?.error || tr("connFail"), "err");
  }
});

// 2. Capture Visible Screenshot
$("captureBtn").addEventListener("click", async () => {
  await savePrefs();
  setMsg(tr("saving"));
  const btn = $("captureBtn");
  btn.disabled = true;
  const resp = await sendMsg({
    type: "CAPTURE_TAB",
    payload: currentDestPayload()
  });
  btn.disabled = false;
  if (resp?.ok) {
    setMsg(resp.res?.offline ? tr("savedOffline") : tr("savedShot"), "ok");
    noteEl.value = "";
    if (!resp.res?.offline) checkConnection(false);
  } else {
    setMsg(resp?.error || tr("connFail"), "err");
  }
});

// 3. Scan Page Images (Batch Grabber)
$("scanBtn").addEventListener("click", async () => {
  if (!activeTab?.id) return;
  const resp = await sendTabMsg(activeTab.id, { type: "SCAN_IMAGES" });
  if (!resp?.images?.length) {
    setMsg(tr("noImages"), "err");
    return;
  }
  scanned = resp.images;
  pageMeta = { pageUrl: resp.pageUrl || activeTab.url, title: resp.title || activeTab.title };
  selectedUrls = new Set(scanned.map((i) => i.url));
  $("scanMeta").textContent = `${scanned.length}`;
  renderGrid();
});

$("toggleAllBtn").addEventListener("click", () => {
  if (selectedUrls.size === scanned.length) selectedUrls.clear();
  else selectedUrls = new Set(scanned.map((i) => i.url));
  renderGrid();
});

function renderGrid() {
  batchWrap.classList.add("show");
  batchCount.textContent = tr("selectedCount")(selectedUrls.size, scanned.length);
  $("toggleAllBtn").textContent = selectedUrls.size === scanned.length ? tr("unselectAll") : tr("selectAll");
  $("saveBatchBtn").disabled = selectedUrls.size === 0;
  gridEl.innerHTML = "";
  for (const item of scanned) {
    const tile = document.createElement("div");
    const isSel = selectedUrls.has(item.url);
    tile.className = "tile" + (isSel ? " sel" : "");
    const dim = item.width && item.height ? `<span class="dim">${item.width}×${item.height}</span>` : "";
    tile.innerHTML = `<img src="${item.url}" loading="lazy">${dim}<span class="chk">✓</span>`;
    tile.addEventListener("click", () => {
      if (selectedUrls.has(item.url)) selectedUrls.delete(item.url);
      else selectedUrls.add(item.url);
      renderGrid();
    });
    gridEl.appendChild(tile);
  }
}

$("saveBatchBtn").addEventListener("click", async () => {
  if (selectedUrls.size === 0) return;
  await savePrefs();
  const dest = currentDestPayload();
  const btn = $("saveBatchBtn");
  btn.disabled = true;
  let ok = 0;
  let fail = 0;
  let anyOffline = false;
  const importedIds = [];
  setMsg(tr("saving"));
  for (const url of selectedUrls) {
    const resp = await sendMsg({
      type: "IMPORT_URL",
      payload: {
        url,
        sourceUrl: pageMeta.pageUrl,
        sourceTitle: pageMeta.title,
        ...dest
      }
    });
    if (resp?.ok) {
      ok++;
      if (resp.res?.offline) anyOffline = true;
      const aid = resp.res?.asset?.id || resp.res?.asset?.ID;
      if (aid) importedIds.push(aid);
    } else {
      fail++;
    }
  }
  let stacked = false;
  if (!anyOffline && dest.autoStack && importedIds.length >= 2) {
    const stResp = await sendMsg({
      type: "CREATE_STACK",
      payload: { name: pageMeta.title || "Web Gallery", assetIds: importedIds }
    });
    stacked = !!stResp?.ok;
  }
  btn.disabled = false;
  setMsg(anyOffline ? tr("savedBatchOffline")(ok) : tr("savedBatch")(ok, fail, stacked), ok > 0 ? "ok" : "err");
  if (!anyOffline) checkConnection(false);
});

// Init
(async () => {
  const cfg = await storageGet({
    serverUrl: DEFAULT_URL,
    token: "",
    offlineFolder: "RaglexInbox",
    defaultTagId: "",
    defaultProjectId: "",
    defaultBoardId: "",
    dragEnabled: true,
    autoStack: true,
    autoTag: true,
    lang: ""
  });
  if (cfg.lang === "zh" || cfg.lang === "en") lang = cfg.lang;
  urlEl.value = cfg.serverUrl || DEFAULT_URL;
  tokenEl.value = cfg.token || "";
  offlineFolderEl.value = cfg.offlineFolder || "RaglexInbox";
  dragEl.checked = cfg.dragEnabled !== false;
  stackEl.checked = cfg.autoStack !== false;
  autoTagEl.checked = cfg.autoTag !== false;

  api.tabs.query({ active: true, currentWindow: true }, (tabs) => {
    activeTab = tabs?.[0] || null;
    if (activeTab) {
      $("pageTitle").textContent = activeTab.title || activeTab.url || "";
      if (activeTab.favIconUrl && activeTab.favIconUrl.startsWith("http")) {
        const fav = $("pageFav");
        fav.src = activeTab.favIconUrl;
        fav.style.display = "block";
      }
    }
    applyI18n();
  });

  applyI18n();
  await checkConnection(false);
  if (cfg.defaultTagId) tagEl.value = cfg.defaultTagId;
  if (cfg.defaultProjectId) projEl.value = cfg.defaultProjectId;
  if (cfg.defaultBoardId) boardEl.value = cfg.defaultBoardId;
})();
