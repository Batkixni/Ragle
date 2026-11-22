(() => {
  let dropZone = null;
  let draggedImgUrl = "";

  function ensureDropZone() {
    if (dropZone) return dropZone;
    const el = document.createElement("div");
    el.id = "raglex-drop-capsule";
    el.style.cssText = [
      "position:fixed",
      "left:50%",
      "bottom:28px",
      "transform:translateX(-50%) translateY(20px)",
      "opacity:0",
      "pointer-events:none",
      "z-index:2147483647",
      "background:#18181B",
      "color:#FAFAFA",
      "font:600 13px/1 Inter,-apple-system,sans-serif",
      "padding:14px 24px",
      "border-radius:999px",
      "box-shadow:0 16px 40px rgba(0,0,0,0.45)",
      "border:2px solid #FF5C33",
      "display:flex",
      "align-items:center",
      "gap:10px",
      "transition:opacity 180ms ease,transform 220ms cubic-bezier(.16,1,.3,1),background 150ms ease"
    ].join(";");
    el.innerHTML = `<span style="width:10px;height:10px;border-radius:99px;background:#FF5C33;display:inline-block"></span><span id="raglex-drop-label">Drop to save to Raglex</span>`;
    el.addEventListener("dragover", (e) => {
      e.preventDefault();
      el.style.background = "#FF5C33";
    });
    el.addEventListener("draglexave", () => {
      el.style.background = "#18181B";
    });
    el.addEventListener("drop", (e) => {
      e.preventDefault();
      el.style.background = "#18181B";
      const url = draggedImgUrl || e.dataTransfer?.getData("text/uri-list") || "";
      if (!url) {
        hideDropZone();
        return;
      }
      const lbl = el.querySelector("#raglex-drop-label");
      if (lbl) lbl.textContent = "Saving to Raglex…";
      chrome.runtime.sendMessage(
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
            lbl.textContent = resp?.ok ? "Saved to Raglex ✓" : `Failed: ${resp?.error || "Check Raglex Server"}`;
          }
          setTimeout(hideDropZone, 1400);
        }
      );
    });
    document.documentElement.appendChild(el);
    dropZone = el;
    return el;
  }

  function showDropZone() {
    const el = ensureDropZone();
    const lbl = el.querySelector("#raglex-drop-label");
    if (lbl) lbl.textContent = "Drop to save to Raglex";
    el.style.pointerEvents = "auto";
    el.style.opacity = "1";
    el.style.transform = "translateX(-50%) translateY(0)";
  }

  function hideDropZone() {
    if (!dropZone) return;
    dropZone.style.pointerEvents = "none";
    dropZone.style.opacity = "0";
    dropZone.style.transform = "translateX(-50%) translateY(20px)";
    draggedImgUrl = "";
  }

  document.addEventListener("dragstart", (e) => {
    const img = e.target?.tagName === "IMG" ? e.target : e.target?.querySelector?.("img");
    if (img && img.currentSrc && img.currentSrc.startsWith("http")) {
      draggedImgUrl = img.currentSrc;
      showDropZone();
    }
  });

  document.addEventListener("dragend", () => {
    setTimeout(hideDropZone, 120);
  });

  chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
    if (msg.type === "SCAN_IMAGES") {
      const seen = new Set();
      const images = [];
      for (const img of document.querySelectorAll("img")) {
        const src = img.currentSrc || img.src;
        if (!src || !src.startsWith("http") || seen.has(src)) continue;
        const w = img.naturalWidth || img.width || 0;
        const h = img.naturalHeight || img.height || 0;
        if (w < 120 || h < 120) continue;
        seen.add(src);
        images.push({ url: src, width: w, height: h, alt: img.alt || "" });
      }
      sendResponse({ images, title: document.title, pageUrl: location.href });
    }
  });
})();
