(() => {
  const SUPPORTED_LANGS = ["zh-Hant", "en", "ja"];
  const VERSION = "0.5.1";
  const RELEASE_BASE = `https://github.com/Batkixni/Raglex/releases/download/v${VERSION}`;

  function detectInitialLang() {
    const params = new URLSearchParams(window.location.search);
    const q = params.get("lang");
    if (SUPPORTED_LANGS.includes(q)) return q;
    try {
      const saved = localStorage.getItem("raglex-lang");
      if (SUPPORTED_LANGS.includes(saved)) return saved;
    } catch (_) {}
    const nav = (navigator.language || "").toLowerCase();
    if (nav.startsWith("zh")) return "zh-Hant";
    if (nav.startsWith("ja")) return "ja";
    return "en";
  }

  function applyLang(lang) {
    if (!SUPPORTED_LANGS.includes(lang)) lang = "zh-Hant";
    document.documentElement.lang = lang;
    try {
      localStorage.setItem("raglex-lang", lang);
    } catch (_) {}
    document.querySelectorAll("[data-set-lang]").forEach((btn) => {
      btn.classList.toggle("is-active", btn.getAttribute("data-set-lang") === lang);
    });
    document.querySelectorAll("[data-i18n-zh]").forEach((el) => {
      const attr = lang === "zh-Hant" ? "data-i18n-zh" : lang === "ja" ? "data-i18n-ja" : "data-i18n-en";
      const val = el.getAttribute(attr);
      if (val !== null) el.textContent = val;
    });
    const titles = document.body.dataset;
    if (lang === "zh-Hant" && titles.titleZh) document.title = titles.titleZh;
    if (lang === "en" && titles.titleEn) document.title = titles.titleEn;
    if (lang === "ja" && titles.titleJa) document.title = titles.titleJa;
    updatePrimaryDownloadCTA(lang);
  }

  function detectOS() {
    const ua = (navigator.userAgent || "").toLowerCase();
    const plat = (navigator.platform || "").toLowerCase();
    if (ua.includes("mac") || plat.includes("mac")) return "macos";
    if (ua.includes("linux") && !ua.includes("android")) return "linux";
    return "windows";
  }

  function updatePrimaryDownloadCTA(lang) {
    const os = detectOS();
    document.querySelectorAll("[data-os-card]").forEach((card) => {
      const match = card.getAttribute("data-os-card") === os;
      card.classList.toggle("is-recommended", match);
      const badge = card.querySelector(".rec-badge");
      if (badge) badge.style.display = match ? "inline-flex" : "none";
    });
    const cta = document.getElementById("hero-download-btn");
    if (!cta) return;
    const labels = {
      windows: {
        "zh-Hant": `下載 Windows 版 (v${VERSION})`,
        en: `Download for Windows (v${VERSION})`,
        ja: `Windows 版をダウンロード (v${VERSION})`,
        href: `${RELEASE_BASE}/raglex-setup-${VERSION}-amd64.exe`,
      },
      macos: {
        "zh-Hant": `下載 macOS 版 (v${VERSION})`,
        en: `Download for macOS (v${VERSION})`,
        ja: `macOS 版をダウンロード (v${VERSION})`,
        href: `${RELEASE_BASE}/Raglex-darwin-arm64.zip`,
      },
      linux: {
        "zh-Hant": `下載 Linux 版 (v${VERSION})`,
        en: `Download for Linux (v${VERSION})`,
        ja: `Linux 版をダウンロード (v${VERSION})`,
        href: `${RELEASE_BASE}/raglex_${VERSION}_amd64.deb`,
      },
    };
    const info = labels[os] || labels.windows;
    const lbl = cta.querySelector(".cta-label");
    if (lbl) lbl.textContent = info[lang] || info.en;
    cta.setAttribute("href", info.href);
  }

  function applyTheme(theme) {
    const next = theme === "light" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    try {
      localStorage.setItem("raglex-theme", next);
    } catch (_) {}
    const sun = document.getElementById("theme-icon-sun");
    const moon = document.getElementById("theme-icon-moon");
    if (sun && moon) {
      sun.style.display = next === "dark" ? "block" : "none";
      moon.style.display = next === "light" ? "block" : "none";
    }
  }

  function initStageSwitcher() {
    const tabs = document.querySelectorAll("[data-stage-tab]");
    const imgs = document.querySelectorAll("[data-stage-img]");
    if (!tabs.length || !imgs.length) return;
    tabs.forEach((tab) => {
      tab.addEventListener("click", () => {
        const target = tab.getAttribute("data-stage-tab");
        tabs.forEach((t) => t.classList.toggle("is-active", t === tab));
        imgs.forEach((img) => img.classList.toggle("is-active", img.getAttribute("data-stage-img") === target));
      });
    });
  }

  function initCopyButtons() {
    document.querySelectorAll("[data-copy-target]").forEach((btn) => {
      btn.addEventListener("click", async () => {
        const id = btn.getAttribute("data-copy-target");
        const el = document.getElementById(id);
        if (!el) return;
        const text = el.textContent.trim();
        try {
          await navigator.clipboard.writeText(text);
        } catch (_) {
          const ta = document.createElement("textarea");
          ta.value = text;
          document.body.appendChild(ta);
          ta.select();
          document.execCommand("copy");
          ta.remove();
        }
        const prev = btn.textContent;
        const lang = document.documentElement.lang;
        btn.textContent = lang === "zh-Hant" ? "已複製" : lang === "ja" ? "コピー済" : "Copied";
        btn.classList.add("is-copied");
        setTimeout(() => {
          btn.textContent = prev;
          btn.classList.remove("is-copied");
        }, 1600);
      });
    });
  }

  function initDocsScrollSpy() {
    const links = Array.from(document.querySelectorAll(".docs-nav a[href^='#']"));
    if (!links.length) return;
    const sections = links
      .map((a) => document.querySelector(a.getAttribute("href")))
      .filter(Boolean);
    const update = () => {
      let current = sections[0]?.id || "";
      const offset = 120;
      for (const sec of sections) {
        if (sec.getBoundingClientRect().top <= offset) {
          current = sec.id;
        }
      }
      links.forEach((a) => {
        a.classList.toggle("is-active", a.getAttribute("href") === `#${current}`);
      });
    };
    window.addEventListener("scroll", update, { passive: true });
    update();
  }

  document.addEventListener("DOMContentLoaded", () => {
    let savedTheme = "dark";
    try {
      const t = localStorage.getItem("raglex-theme");
      if (t === "light" || t === "dark") savedTheme = t;
    } catch (_) {}
    applyTheme(savedTheme);
    applyLang(detectInitialLang());

    document.querySelectorAll("[data-set-lang]").forEach((btn) => {
      btn.addEventListener("click", () => applyLang(btn.getAttribute("data-set-lang")));
    });

    const themeBtn = document.getElementById("theme-toggle");
    if (themeBtn) {
      themeBtn.addEventListener("click", () => {
        const cur = document.documentElement.getAttribute("data-theme");
        applyTheme(cur === "dark" ? "light" : "dark");
      });
    }

    initStageSwitcher();
    initCopyButtons();
    initDocsScrollSpy();
  });
})();
