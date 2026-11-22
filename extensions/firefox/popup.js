const DEFAULT_URL = "http://127.0.0.1:41595";
const urlEl = document.getElementById("url");
const tokenEl = document.getElementById("token");
const tagEl = document.getElementById("tag");
const connEl = document.getElementById("conn");
const msgEl = document.getElementById("msg");
const gridEl = document.getElementById("grid");
const batchCard = document.getElementById("batchCard");
const batchCount = document.getElementById("batchCount");

let scanned = [];
let selectedUrls = new Set();
let pageMeta = { pageUrl: "", title: "" };

async function savePrefs() {
  await chrome.storage.local.set({
    serverUrl: (urlEl.value || DEFAULT_URL).trim().replace(/\/+$/, ""),
    token: tokenEl.value.trim(),
    defaultTagId: tagEl.value
  });
}

async function checkServer() {
  const base = (urlEl.value || DEFAULT_URL).trim().replace(/\/+$/, "");
  try {
    const r = await fetch(`${base}/api/v1/health`);
    const d = await r.json();
    connEl.textContent = d.ok ? `Connected (${d.assets || 0})` : "Offline";
    connEl.style.color = d.ok ? "#FF5C33" : "#A1A1AA";
    const tr = await fetch(`${base}/api/v1/tags`);
    const td = await tr.json();
    const cur = tagEl.value;
    tagEl.innerHTML = `<option value="">No default tag</option>`;
    for (const tg of td.tags || []) {
      const opt = document.createElement("option");
      opt.value = tg.ID || tg.id || "";
      opt.textContent = tg.Name || tg.name || "";
      tagEl.appendChild(opt);
    }
    if (cur) tagEl.value = cur;
  } catch {
    connEl.textContent = "Server Offline";
    connEl.style.color = "#A1A1AA";
  }
}

urlEl.addEventListener("change", async () => { await savePrefs(); checkServer(); });
tokenEl.addEventListener("change", savePrefs);
tagEl.addEventListener("change", savePrefs);

document.getElementById("capture").addEventListener("click", async () => {
  await savePrefs();
  msgEl.textContent = "Capturing…";
  const tagIds = tagEl.value ? [tagEl.value] : [];
  chrome.runtime.sendMessage({ type: "CAPTURE_TAB", tagIds }, (resp) => {
    msgEl.textContent = resp?.ok ? "Screenshot saved to Ragle ✓" : `Error: ${resp?.error}`;
  });
});

document.getElementById("scan").addEventListener("click", async () => {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (!tab?.id) return;
  chrome.tabs.sendMessage(tab.id, { type: "SCAN_IMAGES" }, (resp) => {
    if (!resp?.images?.length) {
      msgEl.textContent = "No large images found on this page.";
      return;
    }
    scanned = resp.images;
    pageMeta = { pageUrl: resp.pageUrl, title: resp.title };
    selectedUrls = new Set(scanned.map((i) => i.url));
    renderGrid();
  });
});

function renderGrid() {
  batchCard.style.display = "flex";
  batchCount.textContent = `${selectedUrls.size} / ${scanned.length} selected`;
  gridEl.innerHTML = "";
  for (const item of scanned) {
    const tile = document.createElement("div");
    tile.className = "tile" + (selectedUrls.has(item.url) ? " sel" : "");
    tile.innerHTML = `<img src="${item.url}" loading="lazy">`;
    tile.addEventListener("click", () => {
      if (selectedUrls.has(item.url)) selectedUrls.delete(item.url);
      else selectedUrls.add(item.url);
      renderGrid();
    });
    gridEl.appendChild(tile);
  }
}

document.getElementById("saveBatch").addEventListener("click", async () => {
  await savePrefs();
  const tagIds = tagEl.value ? [tagEl.value] : [];
  let ok = 0;
  let fail = 0;
  msgEl.textContent = `Saving ${selectedUrls.size} images…`;
  for (const url of selectedUrls) {
    const resp = await new Promise((resolve) =>
      chrome.runtime.sendMessage(
        { type: "IMPORT_URL", payload: { url, sourceUrl: pageMeta.pageUrl, sourceTitle: pageMeta.title, tagIds } },
        resolve
      )
    );
    if (resp?.ok) ok++;
    else fail++;
  }
  msgEl.textContent = `Done: ${ok} saved, ${fail} failed.`;
});

chrome.storage.local.get({ serverUrl: DEFAULT_URL, token: "", defaultTagId: "" }, (cfg) => {
  urlEl.value = cfg.serverUrl || DEFAULT_URL;
  tokenEl.value = cfg.token || "";
  checkServer().then(() => {
    if (cfg.defaultTagId) tagEl.value = cfg.defaultTagId;
  });
});
