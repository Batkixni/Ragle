package ui

import (
	"math"
	"time"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/canvas"
	"ragle/internal/model"
)

type gesture struct {
	kind       string
	ids        []string
	start      map[string]model.Placement
	x0, y0     float64
	wx0, wy0   float64
	mx1, my1   float64
	panX, panY float64
	cx, cy     float64
	startScale float64
	startRot   float64
	startAngle float64
	startDist  float64
	startWidth float64
	startLocal float64
	sign       float64
}

func (a *App) boardView(c *gui.Context) {
	a.syncSelectionBuffers()
	gui.Column(c).Grow(1).FillHeight().Children(func() {
		a.boardToolbar(c)
		gui.Row(c).Grow(1).AlignItems(gui.Stretch).Children(func() {
			a.boardSurface(c)
			a.boardInspector(c)
		})
		a.boardTray(c)
	})
}

func (a *App) boardToolbar(c *gui.Context) {
	t := c.Theme()
	tools := []string{a.t("board.select"), a.t("board.pan"), a.t("board.text")}
	trays := []string{a.t("board.trayProject"), a.t("board.trayLibrary")}
	gui.Row(c).Padding(8, 12).Gap(8).AlignItems(gui.Center).Background(t.Surface).Children(func() {
		gui.Segmented(c.Key("board-tool"), &a.tool, tools...)
		gui.Button(c, a.t("board.group")).OnClick(func() { a.groupSelection() })
		gui.Button(c, a.t("board.ungroup")).OnClick(func() { a.ungroupSelection() })
		gui.Button(c, a.t("board.front")).OnClick(func() { a.nudgeZ(true) })
		gui.Button(c, a.t("board.back")).OnClick(func() { a.nudgeZ(false) })
		gui.Button(c, a.t("board.delete")).OnClick(func() { a.confirmDeleteSelection() })
		gui.Segmented(c.Key("board-tray"), &a.tray, trays...)
	})
}

func (a *App) boardSurface(c *gui.Context) {
	board := gui.Box(c.Key("board")).Grow(1).Focusable().Background(a.paper)
	if a.focusBoard {
		board = board.Focus()
		a.focusBoard = false
	}
	board = board.HandleInput(a.boardInput).Draw(a.paintBoard)
	board.Children(func() {})
	if drag, ok := gui.Drop[assetDrag](board); ok {
		if x, y, over := board.PointerPosition(); over {
			a.dropAsset(drag.ID, float64(x), float64(y))
		}
	}
}

func (a *App) boardTray(c *gui.Context) {
	t := c.Theme()
	list := a.projectAssets
	if a.tray == 1 {
		list = a.assets
	}
	gui.Scroll(c).Height(92).Background(t.Surface).Children(func() {
		gui.Row(c).Padding(8).Gap(8).Children(func() {
			if len(list) == 0 {
				gui.Text(c, a.t("board.empty")).TextColor(t.TextMuted)
				return
			}
			for _, asset := range list {
				item := asset
				name := a.assetName(item)
				card := gui.Column(c.Key("tray-" + item.ID)).Width(120).Padding(6).Gap(4).Radius(6).Background(t.Background)
				card.Drag(assetDrag{ID: item.ID}).Label(name)
				card.Children(func() {
					if bm := a.bitmaps[item.ID]; bm != nil {
						gui.Image(c, bm).Height(48).Fit(gui.Contain)
					}
					gui.Text(c, name)
				})
			}
		})
	})
}

func (a *App) boardInspector(c *gui.Context) {
	t := c.Theme()
	gui.Column(c).Width(260).Padding(12).Gap(8).Background(t.Surface).Children(func() {
		if a.session == nil {
			return
		}
		ids := a.session.SelectedIDs()
		if len(ids) == 0 {
			gui.Text(c, a.t("board.empty")).TextColor(t.TextMuted)
			return
		}
		if len(ids) != 1 {
			gui.Text(c, a.t("inspector.multi")).TextColor(t.TextMuted)
			return
		}
		item := a.session.Items[ids[0]]
		gui.Text(c, a.t("board.notes")).TextColor(t.TextMuted)
		gui.TextInput(c.Key("notes"), &a.notesBuf).OnChange(func() {
			if a.session == nil || a.boundID == "" {
				return
			}
			a.session.SetNotes(a.boundID, a.notesBuf)
			a.markBoard()
		})
		if item.Kind == model.KindText {
			gui.Text(c, a.t("board.body")).TextColor(t.TextMuted)
			field := gui.TextInput(c.Key("body"), &a.editText).OnChange(func() {
				a.commitEditText()
			})
			if a.focusBody {
				field.Focus()
				a.focusBody = false
			}
		}
	})
}

func (a *App) syncSelectionBuffers() {
	if a.session == nil {
		a.boundID = ""
		return
	}
	ids := a.session.SelectedIDs()
	if len(ids) != 1 {
		a.boundID = ""
		return
	}
	if ids[0] == a.boundID {
		return
	}
	a.boundID = ids[0]
	item := a.session.Items[ids[0]]
	a.notesBuf = item.Notes
	a.editText = item.Text
}

func (a *App) commitEditText() {
	if a.session == nil || a.boundID == "" {
		return
	}
	item, ok := a.session.Items[a.boundID]
	if !ok || item.Text == a.editText {
		return
	}
	a.session.SetText(a.boundID, a.editText)
	a.markBoard()
}

func (a *App) markBoard() {
	if a.session == nil {
		return
	}
	a.session.Touch(time.Now())
	a.armFlush()
}

func (a *App) armFlush() {
	if a.flushArmed || a.win == nil || a.session == nil || !a.session.Dirty() {
		return
	}
	a.flushArmed = true
	win := a.win
	go func() {
		time.Sleep(canvas.FlushDelay)
		win.Update(func() {
			a.flushArmed = false
			a.mu.Lock()
			closed := a.closed
			a.mu.Unlock()
			if closed {
				return
			}
			a.flushBoard(false)
			if a.session != nil && a.session.Dirty() {
				a.armFlush()
			}
		})
	}()
}

func (a *App) flushBoard(force bool) {
	if a.session == nil || a.projects == nil || a.boardID == "" {
		return
	}
	items, ok := a.session.Flush(time.Now(), force)
	if !ok {
		return
	}
	if err := a.projects.SavePlacements(a.boardID, items); err != nil {
		a.status = err.Error()
		a.session.Touch(time.Now())
		a.armFlush()
	}
}

func (a *App) groupSelection() {
	if a.session == nil {
		return
	}
	if _, err := a.session.Group(a.session.SelectedIDs()); err != nil {
		a.status = a.t("board.needTwo")
		return
	}
	a.boundID = ""
	a.markBoard()
	a.flushBoard(true)
}

func (a *App) ungroupSelection() {
	if a.session == nil {
		return
	}
	a.session.Ungroup(a.session.SelectedIDs())
	a.boundID = ""
	a.markBoard()
	a.flushBoard(true)
}

func (a *App) nudgeZ(front bool) {
	if a.session == nil {
		return
	}
	ids := a.session.SelectedIDs()
	if len(ids) != 1 {
		return
	}
	if front {
		a.session.BringFront(ids[0])
	} else {
		a.session.BringBack(ids[0])
	}
	a.markBoard()
	a.flushBoard(true)
}

func (a *App) confirmDeleteSelection() {
	if a.session == nil || len(a.session.SelectedIDs()) == 0 {
		return
	}
	a.ask(a.t("board.delete"), a.t("board.deleteNote"), a.t("common.remove"), func() {
		if a.session == nil {
			return
		}
		a.session.Delete(a.session.SelectedIDs())
		a.boundID = ""
		a.markBoard()
		a.flushBoard(true)
	})
}

func (a *App) dropAsset(id string, x, y float64) {
	if a.session == nil {
		return
	}
	asset, ok := a.assetByID[id]
	if !ok && a.lib != nil {
		loaded, err := a.lib.Get(id)
		if err != nil {
			a.status = err.Error()
			return
		}
		asset = loaded
		a.assetByID[id] = asset
	}
	if asset.ID == "" {
		return
	}
	wx, wy := a.session.ScreenToWorld(x, y)
	a.session.PlaceImage(asset.ID, wx, wy, float64(asset.Width), float64(asset.Height))
	if a.projectID != "" && a.projects != nil {
		_ = a.projects.Reference(a.projectID, asset.ID)
	}
	a.boundID = ""
	a.markBoard()
	a.flushBoard(true)
	a.reload()
}

func (a *App) boardInput(ev gui.InputEvent) bool {
	s := a.session
	if s == nil {
		return false
	}
	switch ev.Kind {
	case gui.InputKeyUp:
		if ev.Key == gui.KeySpace {
			a.spaceDown = false
			return true
		}
		return false
	case gui.InputKeyDown:
		return a.boardKey(ev)
	case gui.InputScroll:
		factor := 1.0
		if ev.Precise {
			factor = math.Exp(float64(-ev.DY) * 0.01)
		} else if ev.DY < 0 {
			factor = 1.1
		} else if ev.DY > 0 {
			factor = 1 / 1.1
		}
		s.ZoomAt(float64(ev.X), float64(ev.Y), factor)
		return true
	case gui.InputPointerDown:
		return a.pointerDown(ev)
	case gui.InputPointerMove:
		return a.pointerMove(ev)
	case gui.InputPointerUp:
		return a.pointerUp(ev)
	default:
		return false
	}
}

func (a *App) boardKey(ev gui.InputEvent) bool {
	s := a.session
	if ev.Key == gui.KeySpace {
		a.spaceDown = true
		return true
	}
	if ev.Repeat {
		return false
	}
	ctrl := ev.Mods&gui.Ctrl != 0
	shift := ev.Mods&gui.Shift != 0
	switch ev.Key {
	case gui.KeyG:
		if !ctrl {
			return false
		}
		if shift {
			a.ungroupSelection()
		} else {
			a.groupSelection()
		}
		return true
	case gui.KeyDelete, gui.KeyBackspace:
		a.confirmDeleteSelection()
		return true
	case gui.KeyBracketRight:
		a.nudgeZ(true)
		return true
	case gui.KeyBracketLeft:
		a.nudgeZ(false)
		return true
	case gui.KeyEscape:
		s.Select("", false)
		a.boundID = ""
		a.gesture = gesture{}
		return true
	default:
		return false
	}
}

func (a *App) pointerDown(ev gui.InputEvent) bool {
	s := a.session
	wx, wy := s.ScreenToWorld(float64(ev.X), float64(ev.Y))
	if ev.Button == 2 || a.spaceDown || (a.tool == 1 && ev.Button == 0) {
		a.gesture = gesture{kind: "pan", x0: float64(ev.X), y0: float64(ev.Y), panX: s.PanX, panY: s.PanY}
		return true
	}
	if ev.Button != 0 {
		return false
	}
	if a.tool == 2 {
		if id, part, ok := s.Hit(wx, wy); ok && part == "body" && s.Items[id].Kind == model.KindText {
			s.Select(id, false)
			a.boundID = ""
			a.focusBody = true
			return true
		}
		if _, _, ok := s.Hit(wx, wy); !ok {
			s.PlaceText(a.t("board.placeholder"), wx, wy)
			a.boundID = ""
			a.focusBody = true
			a.markBoard()
			a.flushBoard(true)
			return true
		}
	}
	id, part, ok := s.Hit(wx, wy)
	if !ok {
		s.Select("", false)
		a.boundID = ""
		a.gesture = gesture{kind: "marquee", wx0: wx, wy0: wy, mx1: wx, my1: wy}
		return true
	}
	if part == "scale" || part == "rotate" || part == "width" {
		item := s.Items[id]
		w := canvas.World(s.Items, id)
		g := gesture{
			kind: part, ids: []string{id}, start: s.Snapshot(),
			wx0: wx, wy0: wy, cx: w.X, cy: w.Y,
			startScale: item.ScaleOrOne(), startRot: item.Rotation, startWidth: item.Width,
		}
		g.startDist = math.Hypot(wx-w.X, wy-w.Y)
		g.startAngle = math.Atan2(wy-w.Y, wx-w.X) * 180 / math.Pi
		lx, _ := canvas.WorldToLocal(s.Items, id, wx, wy)
		g.startLocal = lx
		g.sign = 1
		if lx < 0 {
			g.sign = -1
		}
		a.gesture = g
		return true
	}
	shift := ev.Mods&gui.Shift != 0
	if shift {
		s.Select(id, true)
	} else if !s.Selected[id] {
		s.Select(id, false)
	}
	if ev.Clicks >= 2 && s.Items[id].Kind == model.KindText {
		a.boundID = ""
		a.focusBody = true
		return true
	}
	a.gesture = gesture{
		kind: "move", ids: s.SelectedIDs(), start: s.Snapshot(),
		wx0: wx, wy0: wy,
	}
	return true
}

func (a *App) pointerMove(ev gui.InputEvent) bool {
	s := a.session
	g := a.gesture
	if g.kind == "" {
		return false
	}
	wx, wy := s.ScreenToWorld(float64(ev.X), float64(ev.Y))
	switch g.kind {
	case "pan":
		s.PanX = g.panX + (float64(ev.X) - g.x0)
		s.PanY = g.panY + (float64(ev.Y) - g.y0)
	case "marquee":
		a.gesture.mx1 = wx
		a.gesture.my1 = wy
	case "move":
		s.Move(g.start, g.ids, wx-g.wx0, wy-g.wy0)
		a.markBoard()
	case "scale":
		cur := math.Hypot(wx-g.cx, wy-g.cy)
		if g.startDist > 0.5 && len(g.ids) == 1 {
			s.SetScale(g.ids[0], g.startScale*cur/g.startDist)
			a.markBoard()
		}
	case "rotate":
		if len(g.ids) == 1 {
			ang := math.Atan2(wy-g.cy, wx-g.cx) * 180 / math.Pi
			s.SetRotation(g.ids[0], g.startRot+(ang-g.startAngle))
			a.markBoard()
		}
	case "width":
		if len(g.ids) == 1 {
			lx, _ := canvas.WorldToLocal(s.Items, g.ids[0], wx, wy)
			s.SetWidth(g.ids[0], g.startWidth+g.sign*(lx-g.startLocal)*2)
			a.markBoard()
		}
	}
	return true
}

func (a *App) pointerUp(ev gui.InputEvent) bool {
	if a.gesture.kind == "marquee" {
		a.applyMarquee(ev.Mods&gui.Shift != 0)
	}
	if a.gesture.kind == "move" || a.gesture.kind == "scale" || a.gesture.kind == "rotate" || a.gesture.kind == "width" {
		a.flushBoard(true)
	}
	a.gesture = gesture{}
	return true
}

func (a *App) applyMarquee(shift bool) {
	s := a.session
	g := a.gesture
	box := canvas.Box{
		MinX: math.Min(g.wx0, g.mx1),
		MinY: math.Min(g.wy0, g.my1),
		MaxX: math.Max(g.wx0, g.mx1),
		MaxY: math.Max(g.wy0, g.my1),
	}
	if box.MaxX-box.MinX < 2 && box.MaxY-box.MinY < 2 {
		return
	}
	hit := map[string]bool{}
	for id := range s.Items {
		if canvas.Bounds(s.Items, id).Intersects(box) {
			hit[id] = true
		}
	}
	if !shift {
		s.Selected = map[string]bool{}
	}
	for id := range hit {
		parent := s.Items[id].ParentID
		covered := false
		for parent != "" {
			if hit[parent] {
				covered = true
				break
			}
			next, ok := s.Items[parent]
			if !ok {
				break
			}
			parent = next.ParentID
		}
		if !covered {
			s.Selected[id] = true
		}
	}
	a.boundID = ""
}

func (a *App) paintBoard(p *gui.Painter, r gui.Rect) {
	s := a.session
	if s == nil {
		return
	}
	for _, id := range s.Visible(float64(r.W), float64(r.H)) {
		a.paintItem(p, r, id)
	}
	if a.gesture.kind == "marquee" {
		x0, y0 := s.WorldToScreen(a.gesture.wx0, a.gesture.wy0)
		x1, y1 := s.WorldToScreen(a.gesture.mx1, a.gesture.my1)
		rect := gui.Rect{
			X: r.X + float32(math.Min(x0, x1)),
			Y: r.Y + float32(math.Min(y0, y1)),
			W: float32(math.Abs(x1 - x0)),
			H: float32(math.Abs(y1 - y0)),
		}
		p.Fill(rect, gui.RGBA(a.accent.R, a.accent.G, a.accent.B, 0.18), 0)
		p.Stroke(rect, a.accent, 0, 1)
	}
}

func (a *App) paintItem(p *gui.Painter, origin gui.Rect, id string) {
	s := a.session
	item, ok := s.Items[id]
	if !ok {
		return
	}
	w := canvas.World(s.Items, id)
	scale := w.Scale
	if scale == 0 {
		scale = 1
	}
	zoom := s.Zoom
	if zoom == 0 {
		zoom = 1
	}
	sx, sy := s.WorldToScreen(w.X, w.Y)
	cx := origin.X + float32(sx)
	cy := origin.Y + float32(sy)
	dw := float32(item.Width * scale * zoom)
	dh := float32(item.Height * scale * zoom)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	p.Transform(gui.Rotation(float32(w.Rot)), cx, cy, func() {
		rect := gui.Rect{X: cx - dw/2, Y: cy - dh/2, W: dw, H: dh}
		switch item.Kind {
		case model.KindText:
			p.Fill(rect, gui.RGBA(a.ink.R, a.ink.G, a.ink.B, 0.08), 4)
			size := item.FontSize
			if size <= 0 {
				size = 18
			}
			p.Text(rect.X+6, rect.Y+6, item.Text, float32(size*scale*zoom), a.ink)
		case model.KindGroup:
			p.Stroke(rect, a.accent, 4, 1)
		default:
			if bm := a.bitmaps[item.AssetID]; bm != nil {
				p.Image(bm, rect, gui.Contain)
			} else {
				p.Fill(rect, gui.RGBA(a.ink.R, a.ink.G, a.ink.B, 0.08), 4)
				label := a.t("library.missing")
				if asset, ok := a.assetByID[item.AssetID]; ok && !asset.Missing {
					label = a.assetName(asset)
				}
				p.Text(rect.X+6, rect.Y+6, label, 13, a.ink)
			}
		}
		if !s.Selected[id] {
			return
		}
		p.Stroke(rect, a.accent, 2, 1.5)
		if len(s.Selected) != 1 {
			return
		}
		handle := float32(8)
		corners := []gui.Rect{
			{X: rect.X - handle/2, Y: rect.Y - handle/2, W: handle, H: handle},
			{X: rect.X + rect.W - handle/2, Y: rect.Y - handle/2, W: handle, H: handle},
			{X: rect.X + rect.W - handle/2, Y: rect.Y + rect.H - handle/2, W: handle, H: handle},
			{X: rect.X - handle/2, Y: rect.Y + rect.H - handle/2, W: handle, H: handle},
		}
		for _, h := range corners {
			p.Fill(h, a.accent, 2)
		}
		top := gui.Rect{X: cx - 1, Y: rect.Y - 24, W: 2, H: 24}
		p.Fill(top, a.accent, 0)
		p.Fill(gui.Rect{X: cx - handle/2, Y: rect.Y - 24 - handle/2, W: handle, H: handle}, a.accent, handle/2)
		if item.Kind == model.KindText {
			p.Fill(gui.Rect{X: rect.X - handle/2, Y: cy - handle/2, W: handle, H: handle}, a.accent, 2)
			p.Fill(gui.Rect{X: rect.X + rect.W - handle/2, Y: cy - handle/2, W: handle, H: handle}, a.accent, 2)
		}
	})
}
