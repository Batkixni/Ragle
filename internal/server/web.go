package server

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"raglex/internal/library"
	"raglex/internal/model"
)

const webBaseCSS = `
:root { --bg:#111111; --surface:#18181B; --raised:#1F1F23; --muted:#27272A; --ink:#FAFAFA; --sub:#A1A1AA; --accent:#FF5C33; }
* { box-sizing:border-box; margin:0; padding:0; }
body { background:var(--bg); color:var(--ink); font-family:Inter,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; -webkit-font-smoothing:antialiased; }
a { color:inherit; text-decoration:none; }
header { position:sticky; top:0; z-index:20; backdrop-filter:blur(12px); background:rgba(17,17,17,.86); padding:14px 28px; display:flex; align-items:center; gap:16px; }
.brand { display:flex; align-items:center; gap:10px; font-weight:700; font-size:15px; letter-spacing:-.02em; }
.dot { width:10px; height:10px; border-radius:999px; background:var(--accent); }
.pill { background:var(--surface); padding:6px 12px; border-radius:999px; font-size:12px; color:var(--sub); }
.pill:hover { background:var(--muted); color:var(--ink); }
.search { margin-left:auto; background:var(--surface); border:none; color:var(--ink); padding:8px 14px; border-radius:12px; width:240px; font-size:13px; outline:none; }
main { max-width:1380px; margin:0 auto; padding:24px 28px 64px; }
h1 { font-size:28px; font-weight:700; letter-spacing:-.03em; margin-bottom:8px; }
h2 { font-size:12px; text-transform:uppercase; letter-spacing:.08em; color:var(--sub); margin:28px 0 12px; }
.sub { color:var(--sub); font-size:13px; margin-bottom:20px; }
.chips { display:flex; flex-wrap:wrap; gap:8px; margin-bottom:20px; }
.chip { background:var(--surface); padding:7px 14px; border-radius:12px; font-size:13px; font-weight:600; transition:background .15s; }
.chip:hover { background:var(--accent); color:#fff; }
.masonry { column-count:4; column-gap:16px; }
@media (max-width:1100px){ .masonry{ column-count:3; } }
@media (max-width:740px){ .masonry{ column-count:2; } }
.card { break-inside:avoid; margin-bottom:16px; background:var(--surface); border-radius:14px; overflow:hidden; transition:transform .2s ease; }
.card:hover { transform:translateY(-3px); }
.card img { width:100%; display:block; background:var(--muted); }
.cap { padding:10px 12px; display:flex; align-items:center; justify-content:space-between; gap:8px; }
.cap .name { font-size:12px; font-weight:600; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
.cap .meta { font-size:11px; color:var(--sub); white-space:nowrap; }
.pal { display:flex; gap:4px; padding:0 12px 10px; }
.sw { width:12px; height:12px; border-radius:999px; }
`

func (s *Server) handleWebIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if s.lib == nil {
		fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>Raglex Server</title><style>%s</style></head><body><header><div class="brand"><span class="dot"></span>Raglex Signaling Server</div></header><main><h1>Signaling Active</h1><p class="sub">Running in headless WebRTC signaling-only mode. Point Raglex clients to this URL for pairing-code E2E sharing.</p></main></body></html>`, webBaseCSS)
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	assets, _ := s.lib.List(library.Filter{})
	var projs []model.Project
	var sboards []model.Board
	if s.projects != nil {
		projs, _ = s.projects.List()
		sboards, _ = s.projects.StandaloneBoards()
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Raglex · Web Library</title><style>%s</style></head><body>`, webBaseCSS)
	fmt.Fprintf(&b, `<header><a class="brand" href="/"><span class="dot"></span>Raglex</a><span class="pill">%d assets</span><form method="get" action="/"><input class="search" name="q" value="%s" placeholder="Search assets…"></form></header><main>`, len(assets), html.EscapeString(q))

	if len(projs) > 0 {
		b.WriteString(`<h2>Projects</h2><div class="chips">`)
		for _, p := range projs {
			fmt.Fprintf(&b, `<a class="chip" href="/project/%s">%s</a>`, html.EscapeString(p.ID), html.EscapeString(p.Name))
		}
		b.WriteString(`</div>`)
	}
	if len(sboards) > 0 {
		b.WriteString(`<h2>Standalone Boards</h2><div class="chips">`)
		for _, sb := range sboards {
			fmt.Fprintf(&b, `<a class="chip" href="/board/%s">%s</a>`, html.EscapeString(sb.ID), html.EscapeString(sb.Name))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`<h2>Library</h2><div class="masonry">`)
	for _, a := range assets {
		if q != "" && !strings.Contains(strings.ToLower(a.Label()+" "+a.Notes), q) {
			continue
		}
		writeWebAssetCard(&b, a)
	}
	b.WriteString(`</div></main></body></html>`)
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) handleWebProject(w http.ResponseWriter, r *http.Request) {
	if s.lib == nil || s.projects == nil {
		http.NotFound(w, r)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/project/"), "/")
	projs, _ := s.projects.List()
	var proj model.Project
	found := false
	for _, p := range projs {
		if p.ID == id {
			proj = p
			found = true
			break
		}
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	boards, _ := s.projects.Boards(id)
	assets, _ := s.projects.Assets(id)

	var b strings.Builder
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s · Raglex</title><style>%s</style></head><body>`, html.EscapeString(proj.Name), webBaseCSS)
	fmt.Fprintf(&b, `<header><a class="brand" href="/"><span class="dot"></span>Raglex</a><a class="pill" href="/">← Library</a></header><main><h1>%s</h1>`, html.EscapeString(proj.Name))
	if proj.Description != "" {
		fmt.Fprintf(&b, `<p class="sub">%s</p>`, html.EscapeString(proj.Description))
	}
	if len(boards) > 0 {
		b.WriteString(`<h2>Moodboards</h2><div class="chips">`)
		for _, bd := range boards {
			fmt.Fprintf(&b, `<a class="chip" href="/board/%s">%s</a>`, html.EscapeString(bd.ID), html.EscapeString(bd.Name))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`<h2>Referenced Assets</h2><div class="masonry">`)
	for _, a := range assets {
		writeWebAssetCard(&b, a)
	}
	b.WriteString(`</div></main></body></html>`)
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) handleWebBoard(w http.ResponseWriter, r *http.Request) {
	if s.projects == nil {
		http.NotFound(w, r)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/board/"), "/")
	board, err := s.projects.GetBoard(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	placements, _ := s.projects.LoadPlacements(id)

	var b strings.Builder
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s · Board · Raglex</title><style>%s
	#stage{position:relative;width:100vw;height:calc(100vh - 56px);overflow:hidden;background:#141416;background-image:radial-gradient(rgba(255,255,255,.08) 1px,transparent 1px);background-size:24px 24px;cursor:grab;}
	#world{position:absolute;left:0;top:0;transform-origin:0 0;}
	.pl{position:absolute;transform-origin:center center;border-radius:10px;overflow:hidden;box-shadow:0 12px 32px rgba(0,0,0,.4);}
	.pl img{width:100%%;height:100%%;object-fit:cover;display:block;pointer-events:none;}
	.pl-txt{background:#1F1F23;color:#FAFAFA;padding:12px 16px;font-weight:600;line-height:1.45;}
	</style></head><body>`, html.EscapeString(board.Name), webBaseCSS)
	backHref := "/"
	if board.ProjectID != "" {
		backHref = "/project/" + html.EscapeString(board.ProjectID)
	}
	fmt.Fprintf(&b, `<header><a class="brand" href="/"><span class="dot"></span>Raglex</a><a class="pill" href="%s">← Back</a><span class="pill">%s · %d items</span></header>`, backHref, html.EscapeString(board.Name), len(placements))
	b.WriteString(`<div id="stage"><div id="world">`)
	for _, pl := range placements {
		sc := pl.ScaleOrOne()
		wPx := pl.Width * sc
		hPx := pl.Height * sc
		if wPx <= 0 {
			wPx = 220
		}
		if hPx <= 0 {
			hPx = 160
		}
		left := pl.X - wPx/2
		top := pl.Y - hPx/2
		if pl.Kind == model.KindImage && pl.AssetID != "" {
			fmt.Fprintf(&b, `<a class="pl" href="/api/v1/assets/%s/file" target="_blank" style="left:%.1fpx;top:%.1fpx;width:%.1fpx;height:%.1fpx;z-index:%d;transform:rotate(%.1fdeg)"><img src="/api/v1/assets/%s/thumb" loading="lazy"></a>`,
				html.EscapeString(pl.AssetID), left, top, wPx, hPx, pl.ZIndex, pl.Rotation, html.EscapeString(pl.AssetID))
		} else if pl.Kind == model.KindText {
			fmt.Fprintf(&b, `<div class="pl pl-txt" style="left:%.1fpx;top:%.1fpx;width:%.1fpx;z-index:%d;transform:rotate(%.1fdeg)">%s</div>`,
				left, top, wPx, pl.ZIndex, pl.Rotation, html.EscapeString(pl.Text))
		}
	}
	b.WriteString(`</div></div><script>
	let panX=40,panY=40,zoom=1,drag=false,sx=0,sy=0;
	const st=document.getElementById('stage'),wd=document.getElementById('world');
	const upd=()=>{wd.style.transform='translate('+panX+'px,'+panY+'px) scale('+zoom+')';};
	upd();
	st.addEventListener('pointerdown',e=>{drag=true;sx=e.clientX-panX;sy=e.clientY-panY;st.style.cursor='grabbing';});
	window.addEventListener('pointermove',e=>{if(!drag)return;panX=e.clientX-sx;panY=e.clientY-sy;upd();});
	window.addEventListener('pointerup',()=>{drag=false;st.style.cursor='grab';});
	st.addEventListener('wheel',e=>{e.preventDefault();const f=e.deltaY<0?1.1:0.9;zoom=Math.max(0.2,Math.min(4,zoom*f));upd();},{passive:false});
	</script></body></html>`)
	_, _ = w.Write([]byte(b.String()))
}

func writeWebAssetCard(b *strings.Builder, a model.Asset) {
	fmt.Fprintf(b, `<div class="card"><a href="/api/v1/assets/%s/file" target="_blank"><img src="/api/v1/assets/%s/thumb" alt="%s" loading="lazy"></a><div class="cap"><span class="name">%s</span>`,
		html.EscapeString(a.ID), html.EscapeString(a.ID), html.EscapeString(a.Label()), html.EscapeString(a.Label()))
	if a.Width > 0 && a.Height > 0 {
		fmt.Fprintf(b, `<span class="meta">%d×%d</span>`, a.Width, a.Height)
	}
	b.WriteString(`</div>`)
	if len(a.Palette) > 0 {
		b.WriteString(`<div class="pal">`)
		for _, pc := range a.Palette {
			fmt.Fprintf(b, `<span class="sw" style="background:%s" title="%s"></span>`, html.EscapeString(pc.Hex), html.EscapeString(pc.Hex))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
}
