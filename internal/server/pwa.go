package server

import (
	"net/http"
)

func (s *Server) registerPWARoutes() {
	s.mux.HandleFunc("/manifest.webmanifest", s.handlePWAManifest)
	s.mux.HandleFunc("/sw.js", s.handlePWAServiceWorker)
	s.mux.HandleFunc("/mobile", s.handleMobilePWA)
}

func (s *Server) handlePWAManifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	_, _ = w.Write([]byte(`{
  "name": "Raglex Mobile",
  "short_name": "Raglex",
  "description": "Local-first visual asset library, moodboards & inspiration clipper",
  "start_url": "/mobile",
  "scope": "/",
  "display": "standalone",
  "background_color": "#0e0e11",
  "theme_color": "#0e0e11"
}`))
}

func (s *Server) handlePWAServiceWorker(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(`const CACHE = 'raglex-pwa-v1';
self.addEventListener('install', (e) => { self.skipWaiting(); });
self.addEventListener('activate', (e) => { e.waitUntil(self.clients.claim()); });
self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);
  if (e.request.method === 'GET' && url.pathname.includes('/thumb')) {
    e.respondWith(
      caches.open(CACHE).then(async (cache) => {
        const hit = await cache.match(e.request);
        if (hit) return hit;
        const res = await fetch(e.request);
        if (res.ok) cache.put(e.request, res.clone());
        return res;
      })
    );
  }
});
`))
}

func (s *Server) handleMobilePWA(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(mobilePWAHTML))
}

const mobilePWAHTML = `<!doctype html>
<html lang="zh-Hant">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,maximum-scale=1,viewport-fit=cover">
<meta name="theme-color" content="#0e0e11">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
<link rel="manifest" href="/manifest.webmanifest">
<title>Raglex Mobile</title>
<style>
:root{--bg:#0e0e11;--surf:#18181d;--muted:rgba(255,255,255,.08);--border:rgba(255,255,255,.09);--ink:#fafafa;--sub:#a1a1aa;--accent:#ededed}
*{box-sizing:border-box;-webkit-tap-highlight-color:transparent}
body{margin:0;background:var(--bg);color:var(--ink);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;padding-bottom:84px}
header{position:sticky;top:0;z-index:20;background:rgba(14,14,17,.86);backdrop-filter:blur(18px);border-bottom:1px solid var(--border);padding:12px 14px 10px}
.top-row{display:flex;align-items:center;gap:10px}
.brand{font-weight:700;font-size:15px;letter-spacing:-.2px}
.search{flex:1;background:var(--muted);border:1px solid var(--border);color:var(--ink);padding:8px 12px;border-radius:999px;font-size:14px;outline:none}
.tags{display:flex;gap:6px;overflow-x:auto;padding-top:10px;scrollbar-width:none}
.tags::-webkit-scrollbar{display:none}
.pill{flex:0 0 auto;padding:5px 11px;border-radius:999px;background:var(--muted);border:1px solid transparent;color:var(--sub);font-size:12px;font-weight:600;cursor:pointer}
.pill.on{background:var(--accent);color:#111}
.grid{padding:12px;column-count:2;column-gap:10px}
@media(min-width:720px){.grid{column-count:4}}
.card{break-inside:avoid;margin-bottom:10px;background:var(--surf);border-radius:12px;overflow:hidden;border:1px solid var(--border);cursor:pointer}
.card img{width:100%;display:block}
.card .cap{padding:7px 9px;font-size:11px;color:var(--sub);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.fab{position:fixed;right:18px;bottom:22px;z-index:30;width:54px;height:54px;border-radius:999px;border:0;background:var(--accent);color:#111;font-size:26px;font-weight:700;box-shadow:0 10px 30px rgba(0,0,0,.5);cursor:pointer}
.sheet-bg{position:fixed;inset:0;z-index:40;background:rgba(0,0,0,.62);display:none;align-items:flex-end;justify-content:center}
.sheet-bg.open{display:flex}
.sheet{background:var(--surf);border:1px solid var(--border);border-radius:20px 20px 0 0;width:100%;max-width:480px;padding:18px 18px 28px}
.sheet h3{margin:0 0 12px;font-size:15px}
.field{width:100%;background:#111115;border:1px solid var(--border);color:var(--ink);padding:10px 12px;border-radius:10px;font-size:14px;margin-bottom:10px}
.btn{width:100%;padding:11px;border-radius:10px;border:0;font-weight:600;font-size:14px;cursor:pointer;margin-bottom:8px}
.btn.pri{background:var(--accent);color:#111}
.btn.sec{background:var(--muted);color:var(--ink)}
.lightbox{position:fixed;inset:0;z-index:50;background:rgba(10,10,12,.95);display:none;flex-direction:column;align-items:center;justify-content:center;padding:20px}
.lightbox.open{display:flex}
.lightbox img{max-width:100%;max-height:76vh;border-radius:10px}
.swatches{display:flex;gap:8px;margin-top:14px}
.sw{width:24px;height:24px;border-radius:999px;border:1px solid rgba(255,255,255,.2)}
.toast{position:fixed;left:50%;bottom:88px;transform:translateX(-50%);background:#23232a;border:1px solid var(--border);padding:8px 14px;border-radius:999px;font-size:12px;display:none;z-index:60}
</style>
</head>
<body>
<header>
  <div class="top-row">
    <div class="brand">Raglex</div>
    <input id="q" class="search" type="search" placeholder="搜尋素材、標籤或備註…">
  </div>
  <div id="tags" class="tags"></div>
</header>
<main id="grid" class="grid"></main>
<button class="fab" id="addBtn" aria-label="新增素材">+</button>

<div class="sheet-bg" id="sheetBg">
  <div class="sheet">
    <h3>快速收藏到 Raglex</h3>
    <input id="tok" class="field" type="password" placeholder="API Token（自架或桌機設定中複製）">
    <input id="urlIn" class="field" type="url" placeholder="貼上網頁或圖片連結 https://…">
    <button class="btn pri" id="saveUrlBtn">收藏網頁書籤</button>
    <input id="fileIn" type="file" accept="image/*,video/*" multiple style="display:none">
    <button class="btn sec" id="pickFileBtn">拍照 / 從手機相簿上傳</button>
    <button class="btn sec" id="closeSheetBtn">取消</button>
  </div>
</div>

<div class="lightbox" id="lb">
  <img id="lbImg" alt="">
  <div id="lbTitle" style="margin-top:10px;font-size:13px;color:#d4d4d8"></div>
  <div id="lbSwatches" class="swatches"></div>
</div>
<div id="toast" class="toast"></div>

<script>
if ('serviceWorker' in navigator) navigator.serviceWorker.register('/sw.js').catch(()=>{});
let activeTag = '', allAssets = [];
const $ = id => document.getElementById(id);
const toast = msg => { const t=$('toast'); t.textContent=msg; t.style.display='block'; setTimeout(()=>t.style.display='none', 2600); };
$('tok').value = localStorage.getItem('raglex_token') || '';
$('tok').addEventListener('change', () => localStorage.setItem('raglex_token', $('tok').value.trim()));

async function loadTags() {
  const r = await fetch('/api/v1/tags'); const d = await r.json();
  const bar = $('tags'); bar.innerHTML = '';
  const mk = (id, name) => {
    const b = document.createElement('button');
    b.className = 'pill' + (activeTag===id ? ' on' : '');
    b.textContent = name;
    b.onclick = () => { activeTag = id; loadTags(); loadAssets(); };
    bar.appendChild(b);
  };
  mk('', '全部素材');
  (d.tags || []).forEach(t => mk(t.ID || t.id, t.Name || t.name));
}

async function loadAssets() {
  const qs = new URLSearchParams();
  if ($('q').value.trim()) qs.set('q', $('q').value.trim());
  if (activeTag) qs.set('tag', activeTag);
  const r = await fetch('/api/v1/assets?' + qs.toString()); const d = await r.json();
  allAssets = d.assets || [];
  const g = $('grid'); g.innerHTML = '';
  allAssets.forEach(a => {
    const card = document.createElement('div'); card.className = 'card';
    card.innerHTML = '<img loading="lazy" src="/api/v1/assets/' + a.id + '/thumb"><div class="cap">' + (a.name || '') + '</div>';
    card.onclick = () => openLB(a);
    g.appendChild(card);
  });
}

function openLB(a) {
  $('lbImg').src = '/api/v1/assets/' + a.id + '/file';
  $('lbTitle').textContent = a.name || '';
  const sw = $('lbSwatches'); sw.innerHTML = '';
  (a.palette || []).forEach(p => {
    const hex = p.Hex || p.hex; if (!hex) return;
    const d = document.createElement('div'); d.className = 'sw'; d.style.background = hex;
    d.onclick = (e) => { e.stopPropagation(); navigator.clipboard?.writeText(hex); toast('已複製色票 ' + hex); };
    sw.appendChild(d);
  });
  $('lb').classList.add('open');
}
$('lb').onclick = () => $('lb').classList.remove('open');
$('q').addEventListener('input', () => { clearTimeout(window._qt); window._qt = setTimeout(loadAssets, 200); });
$('addBtn').onclick = () => $('sheetBg').classList.add('open');
$('closeSheetBtn').onclick = () => $('sheetBg').classList.remove('open');

$('saveUrlBtn').onclick = async () => {
  const u = $('urlIn').value.trim(); if (!u) return;
  const tok = $('tok').value.trim(); localStorage.setItem('raglex_token', tok);
  const r = await fetch('/api/v1/import/url', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Raglex-Token': tok },
    body: JSON.stringify({ url: u })
  });
  if (r.ok) { $('urlIn').value = ''; $('sheetBg').classList.remove('open'); toast('已收藏至 Raglex！'); loadAssets(); }
  else toast('匯入失敗（請檢查 Token）');
};

$('pickFileBtn').onclick = () => $('fileIn').click();
$('fileIn').onchange = async (e) => {
  const tok = $('tok').value.trim(); localStorage.setItem('raglex_token', tok);
  let ok = 0;
  for (const f of e.target.files) {
    const b64 = await new Promise(res => { const fr = new FileReader(); fr.onload = () => res(fr.result.split(',')[1]); fr.readAsDataURL(f); });
    const r = await fetch('/api/v1/import/base64', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Raglex-Token': tok },
      body: JSON.stringify({ filename: f.name, data_base64: b64 })
    });
    if (r.ok) ok++;
  }
  $('sheetBg').classList.remove('open');
  toast(ok > 0 ? '已上傳 ' + ok + ' 個素材！' : '上傳失敗（請檢查 Token）');
  loadAssets();
};

loadTags(); loadAssets();
</script>
</body>
</html>`
