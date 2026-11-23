(() => {
  const api = chrome;
  let dropZone = null;
  let draggedImgUrl = "";
  let dragEnabled = true;

  const isZh = (navigator.language || "").toLowerCase().startsWith("zh");
  const txt = {
    drop: isZh ? "拖放至此存入 Raglex" : "Drop to save to Raglex",
    saving: isZh ? "正在儲存至 Raglex…" : "Saving to Raglex…",
    saved: isZh ? "已存入 Raglex ✓" : "Saved to Raglex ✓",
    failed: isZh ? "儲存失敗，請確認 Raglex 已開啟" : "Failed — check Raglex desktop"
  };

  try {
    api.storage.local.get({ dragEnabled: true }, (cfg) => {
      dragEnabled = cfg?.dragEnabled !== false;
    });
    api.storage.onChanged?.addListener((changes) => {
      if (changes.dragEnabled) {
        dragEnabled = changes.dragEnabled.newValue !== false;
      }
    });
  } catch {}

  function ensureDropZone() {
    if (dropZone) return dropZone;
    const el = document.createElement("div");
    el.id = "raglex-drop-capsule";
    el.style.cssText = [
      "position:fixed",
      "left:50%",
      "bottom:26px",
      "transform:translateX(-50%) translateY(18px)",
      "opacity:0",
      "pointer-events:none",
      "z-index:2147483647",
      "background:rgba(21,21,24,0.92)",
      "backdrop-filter:blur(16px)",
      "-webkit-backdrop-filter:blur(16px)",
      "color:#FAFAFA",
      "font:600 13px/1 -apple-system,BlinkMacSystemFont,'Segoe UI',Inter,sans-serif",
      "padding:12px 20px",
      "border-radius:999px",
      "box-shadow:0 18px 44px rgba(0,0,0,0.48)",
      "border:1px solid rgba(255,255,255,0.14)",
      "display:flex",
      "align-items:center",
      "gap:9px",
      "transition:opacity 160ms ease,transform 200ms cubic-bezier(.16,1,.3,1),background 140ms ease,border-color 140ms ease"
    ].join(";");
    el.innerHTML = `<span style="width:8px;height:8px;border-radius:99px;background:#FF5C33;display:inline-block;flex-shrink:0"></span><span id="raglex-drop-label">${txt.drop}</span>`;
    el.addEventListener("dragover", (e) => {
      e.preventDefault();
      el.style.background = "rgba(255,92,51,0.94)";
      el.style.borderColor = "#FF5C33";
    });
    el.addEventListener("dragleave", () => {
      el.style.background = "rgba(21,21,24,0.92)";
      el.style.borderColor = "rgba(255,255,255,0.14)";
    });
    el.addEventListener("drop", (e) => {
      e.preventDefault();
      el.style.background = "rgba(21,21,24,0.92)";
      el.style.borderColor = "rgba(255,255,255,0.14)";
      const url = draggedImgUrl || e.dataTransfer?.getData("text/uri-list") || "";
      if (!url) {
        hideDropZone();
        return;
      }
      const lbl = el.querySelector("#raglex-drop-label");
      if (lbl) lbl.textContent = txt.saving;
      api.runtime.sendMessage(
        {
          type: "IMPORT_URL",
          payload: {
            url,
            sourceUrl: location.href,
            sourceTitle: document.title
          }
        },
        (resp) => {
          if (lbl) {
            lbl.textContent = resp?.ok ? txt.saved : (resp?.error || txt.failed);
          }
          setTimeout(hideDropZone, 1300);
        }
      );
    });
    document.documentElement.appendChild(el);
    dropZone = el;
    return el;
  }

  function showDropZone() {
    if (!dragEnabled) return;
    const el = ensureDropZone();
    const lbl = el.querySelector("#raglex-drop-label");
    if (lbl) lbl.textContent = txt.drop;
    el.style.pointerEvents = "auto";
    el.style.opacity = "1";
    el.style.transform = "translateX(-50%) translateY(0)";
  }

  function hideDropZone() {
    if (!dropZone) return;
    dropZone.style.pointerEvents = "none";
    dropZone.style.opacity = "0";
    dropZone.style.transform = "translateX(-50%) translateY(18px)";
    draggedImgUrl = "";
  }

  document.addEventListener("dragstart", (e) => {
    if (!dragEnabled) return;
    const img = e.target?.tagName === "IMG" ? e.target : e.target?.querySelector?.("img");
    if (img && (img.currentSrc || img.src || "").startsWith("http")) {
      draggedImgUrl = img.currentSrc || img.src;
      showDropZone();
    }
  });

  document.addEventListener("dragend", () => {
    setTimeout(hideDropZone, 120);
  });

  function scanPageImages() {
    const seen = new Set();
    const images = [];
    const add = (src, w, h, alt) => {
      if (!src || !src.startsWith("http") || seen.has(src)) return;
      if (w < 110 || h < 110) return;
      seen.add(src);
      images.push({ url: src, width: Math.round(w), height: Math.round(h), alt: alt || "" });
    };

    for (const img of document.querySelectorAll("img")) {
      const src = img.currentSrc || img.src;
      const w = img.naturalWidth || img.width || 0;
      const h = img.naturalHeight || img.height || 0;
      add(src, w, h, img.alt);
    }

    // Also inspect visible hero/card CSS background-image elements.
    for (const el of document.querySelectorAll("[style*='background'], figure, article, div")) {
      if (images.length >= 60) break;
      const rect = el.getBoundingClientRect();
      if (rect.width < 140 || rect.height < 140) continue;
      const bg = getComputedStyle(el).backgroundImage;
      if (!bg || !bg.includes("url(")) continue;
      const m = bg.match(/url\(["']?(https?:\/\/[^"')]+)["']?\)/i);
      if (m && m[1]) add(m[1], rect.width, rect.height, "");
    }

    images.sort((a, b) => (b.width * b.height) - (a.width * a.height));
    return images;
  }

  api.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    if (msg.type === "SCAN_IMAGES") {
      sendResponse({ images: scanPageImages(), title: document.title, pageUrl: location.href });
    }
  });
})();
