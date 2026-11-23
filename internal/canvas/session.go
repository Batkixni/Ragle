package canvas

import (
	"errors"
	"math"
	"sort"
	"time"

	"raglex/internal/model"
)

// ErrNeedTwo means Group was asked to wrap fewer than two objects.
var ErrNeedTwo = errors.New("raglex: select two objects to group them")

// FlushDelay is the longest a dirty board waits before it should be written.
const FlushDelay = 400 * time.Millisecond

// Session is one open board. Coordinates change here; the caller writes
// them back when Flush says so.
type Session struct {
	Items    map[string]model.Placement
	Selected map[string]bool
	PanX     float64
	PanY     float64
	Zoom     float64

	dirty      bool
	dirtySince time.Time

	undoStack []map[string]model.Placement
	redoStack []map[string]model.Placement
}

const maxUndoDepth = 80

func cloneItems(src map[string]model.Placement) map[string]model.Placement {
	out := make(map[string]model.Placement, len(src))
	for k, v := range src {
		if len(v.Points) > 0 {
			pts := make([]float64, len(v.Points))
			copy(pts, v.Points)
			v.Points = pts
		}
		out[k] = v
	}
	return out
}

// RecordUndo saves a deep snapshot of the current Items before a mutation.
func (s *Session) RecordUndo() {
	s.PushUndo(s.Items)
}

// PushUndo pushes a previously captured snapshot onto the undo stack and clears redo.
func (s *Session) PushUndo(snap map[string]model.Placement) {
	if snap == nil {
		return
	}
	s.undoStack = append(s.undoStack, cloneItems(snap))
	if len(s.undoStack) > maxUndoDepth {
		s.undoStack = s.undoStack[len(s.undoStack)-maxUndoDepth:]
	}
	s.redoStack = nil
}

// CanUndo reports whether there is at least one state to restore.
func (s *Session) CanUndo() bool { return len(s.undoStack) > 0 }

// CanRedo reports whether there is at least one undone state to reapply.
func (s *Session) CanRedo() bool { return len(s.redoStack) > 0 }

// Undo restores the previous board state and pushes the current state to redo.
func (s *Session) Undo() bool {
	if len(s.undoStack) == 0 {
		return false
	}
	s.redoStack = append(s.redoStack, cloneItems(s.Items))
	prev := s.undoStack[len(s.undoStack)-1]
	s.undoStack = s.undoStack[:len(s.undoStack)-1]
	s.Items = prev
	for id := range s.Selected {
		if _, ok := s.Items[id]; !ok {
			delete(s.Selected, id)
		}
	}
	return true
}

// Redo reapplies the most recently undone board state.
func (s *Session) Redo() bool {
	if len(s.redoStack) == 0 {
		return false
	}
	s.undoStack = append(s.undoStack, cloneItems(s.Items))
	next := s.redoStack[len(s.redoStack)-1]
	s.redoStack = s.redoStack[:len(s.redoStack)-1]
	s.Items = next
	for id := range s.Selected {
		if _, ok := s.Items[id]; !ok {
			delete(s.Selected, id)
		}
	}
	return true
}

// NewSession copies items into a board opened at a small margin from the origin.
func NewSession(items []model.Placement) *Session {
	s := &Session{
		Items:    make(map[string]model.Placement, len(items)),
		Selected: map[string]bool{},
		PanX:     40,
		PanY:     40,
		Zoom:     1,
	}
	for _, item := range items {
		if item.Scale == 0 {
			item.Scale = 1
		}
		s.Items[item.ID] = item
	}
	return s
}

// Touch marks the board dirty. The first call starts the flush deadline.
func (s *Session) Touch(now time.Time) {
	if !s.dirty {
		s.dirtySince = now
	}
	s.dirty = true
}

// Dirty reports an unwritten edit.
func (s *Session) Dirty() bool { return s.dirty }

// Flush returns every placement when force is set or the deadline has
// passed. A second call returns nothing until Touch runs again.
func (s *Session) Flush(now time.Time, force bool) ([]model.Placement, bool) {
	if !s.dirty {
		return nil, false
	}
	if !force && now.Sub(s.dirtySince) < FlushDelay {
		return nil, false
	}
	s.dirty = false
	return s.Slice(), true
}

// Slice returns the placements in a stable order.
func (s *Session) Slice() []model.Placement {
	out := make([]model.Placement, 0, len(s.Items))
	for _, item := range s.Items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ScreenToWorld maps a point in the board widget, in DIPs, into board space.
func (s *Session) ScreenToWorld(x, y float64) (float64, float64) {
	z := s.Zoom
	if z == 0 {
		z = 1
	}
	return (x - s.PanX) / z, (y - s.PanY) / z
}

// WorldToScreen maps a board point into the board widget.
func (s *Session) WorldToScreen(x, y float64) (float64, float64) {
	z := s.Zoom
	if z == 0 {
		z = 1
	}
	return x*z + s.PanX, y*z + s.PanY
}

// ZoomAt multiplies the zoom, keeping the board point under x, y in place.
func (s *Session) ZoomAt(x, y, factor float64) {
	if factor <= 0 {
		return
	}
	wx, wy := s.ScreenToWorld(x, y)
	s.Zoom *= factor
	if s.Zoom < 0.05 {
		s.Zoom = 0.05
	}
	if s.Zoom > 16 {
		s.Zoom = 16
	}
	s.PanX = x - wx*s.Zoom
	s.PanY = y - wy*s.Zoom
}

// Pan moves the camera by a screen delta.
func (s *Session) Pan(dx, dy float64) {
	s.PanX += dx
	s.PanY += dy
}

// ViewWorld is the board rectangle currently inside a widget of the given size.
func (s *Session) ViewWorld(width, height float64) Box {
	x0, y0 := s.ScreenToWorld(0, 0)
	x1, y1 := s.ScreenToWorld(width, height)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	return Box{MinX: x0, MinY: y0, MaxX: x1, MaxY: y1}
}

// Visible returns objects whose boxes meet the widget, in paint order.
func (s *Session) Visible(width, height float64) []string {
	view := s.ViewWorld(width, height)
	var out []string
	for _, id := range paintOrder(s.Items) {
		if Bounds(s.Items, id).Intersects(view) {
			out = append(out, id)
		}
	}
	return out
}

// Hit finds the front-most object under a board point.
// A single selection's handles win over the body so they can be grabbed.
func (s *Session) Hit(wx, wy float64) (id, part string, ok bool) {
	zoom := s.Zoom
	if zoom == 0 {
		zoom = 1
	}
	handle := 9 / zoom
	if len(s.Selected) == 1 {
		for sel := range s.Selected {
			if p, hit := hitHandle(s.Items, sel, wx, wy, handle, zoom); hit {
				return sel, p, true
			}
		}
	}
	order := paintOrder(s.Items)
	for i := len(order) - 1; i >= 0; i-- {
		cid := order[i]
		if hitBody(s.Items, cid, wx, wy, zoom) {
			return cid, "body", true
		}
	}
	return "", "", false
}

func hitBody(items map[string]model.Placement, id string, wx, wy, zoom float64) bool {
	p, ok := items[id]
	if !ok {
		return false
	}
	tol := math.Max(10/zoom, p.StrokeWidth/2+4/zoom)
	switch p.Kind {
	case model.KindArrow:
		if len(p.Points) < 4 {
			return ContainsPoint(items, id, wx, wy)
		}
		x0, y0 := localToWorld(items, id, p.Points[0], p.Points[1])
		x1, y1 := localToWorld(items, id, p.Points[2], p.Points[3])
		return distPointToSeg(wx, wy, x0, y0, x1, y1) <= tol
	case model.KindStroke, model.KindHighlight:
		if len(p.Points) < 4 {
			return ContainsPoint(items, id, wx, wy)
		}
		for i := 0; i+3 < len(p.Points); i += 2 {
			x0, y0 := localToWorld(items, id, p.Points[i], p.Points[i+1])
			x1, y1 := localToWorld(items, id, p.Points[i+2], p.Points[i+3])
			if distPointToSeg(wx, wy, x0, y0, x1, y1) <= tol {
				return true
			}
		}
		return false
	case model.KindFrame:
		lx, ly := WorldToLocal(items, id, wx, wy)
		hw, hh := p.Width/2, p.Height/2
		// Title label bar above top-left corner of Frame.
		labelH := 26 / zoom
		labelW := math.Min(math.Max(110/zoom, p.Width*0.65), p.Width)
		if lx >= -hw-4/zoom && lx <= -hw+labelW && ly >= -hh-labelH && ly <= -hh+tol {
			return true
		}
		// Frame border ring (interior stays click-through so items inside can be marquee-selected!).
		if math.Abs(lx) <= hw+tol && math.Abs(ly) <= hh+tol {
			if math.Abs(lx) >= hw-tol || math.Abs(ly) >= hh-tol {
				return true
			}
		}
		return false
	default:
		return ContainsPoint(items, id, wx, wy)
	}
}

func distPointToSeg(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	if l2 < 1e-9 {
		return math.Hypot(px-x0, py-y0)
	}
	t := ((px-x0)*dx + (py-y0)*dy) / l2
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}

func hitHandle(items map[string]model.Placement, id string, wx, wy, reach, zoom float64) (string, bool) {
	p, ok := items[id]
	if !ok {
		return "", false
	}
	if zoom == 0 {
		zoom = 1
	}
	// Vector Arrow: two draggable endpoint handles (start & tip).
	if p.Kind == model.KindArrow && len(p.Points) >= 4 {
		x0, y0 := localToWorld(items, id, p.Points[0], p.Points[1])
		if dist(x0, y0, wx, wy) <= reach*1.25 {
			return "arrow-p0", true
		}
		x1, y1 := localToWorld(items, id, p.Points[2], p.Points[3])
		if dist(x1, y1, wx, wy) <= reach*1.25 {
			return "arrow-p1", true
		}
		return "", false
	}
	w := World(items, id)
	scale := w.Scale
	if scale == 0 {
		scale = 1
	}
	// Handles stay a fixed size on screen, so their local offset shrinks as the view zooms in.
	rotateGap := (24 / zoom) / scale
	hw, hh := p.Width/2, p.Height/2
	if p.Kind == model.KindFrame || p.Kind == model.KindRect {
		locals := []struct {
			x, y float64
			part string
		}{
			{-hw, -hh, "resize-tl"},
			{hw, -hh, "resize-tr"},
			{hw, hh, "resize-br"},
			{-hw, hh, "resize-bl"},
		}
		if p.Kind == model.KindRect {
			locals = append(locals, struct {
				x, y float64
				part string
			}{0, -hh - rotateGap, "rotate"})
		}
		for _, h := range locals {
			sx, sy := localToWorld(items, id, h.x, h.y)
			if dist(sx, sy, wx, wy) <= reach {
				return h.part, true
			}
		}
		return "", false
	}
	locals := []struct {
		x, y float64
		part string
	}{
		{-hw, -hh, "scale"},
		{hw, -hh, "scale"},
		{hw, hh, "scale"},
		{-hw, hh, "scale"},
		{0, -hh - rotateGap, "rotate"},
	}
	if p.Kind == model.KindText {
		locals = append(locals,
			struct {
				x, y float64
				part string
			}{hw, 0, "width"},
			struct {
				x, y float64
				part string
			}{-hw, 0, "width"},
		)
	}
	for _, h := range locals {
		sx, sy := localToWorld(items, id, h.x, h.y)
		if dist(sx, sy, wx, wy) <= reach {
			return h.part, true
		}
	}
	return "", false
}

// ResizeCorner resizes a Frame or Rectangle by moving one corner handle while
// keeping the opposite corner fixed, preserving crisp stroke width.
func (s *Session) ResizeCorner(start map[string]model.Placement, id, part string, wx, wy float64) {
	orig, ok := start[id]
	if !ok {
		return
	}
	cur, ok := s.Items[id]
	if !ok {
		return
	}
	left := orig.X - orig.Width/2
	right := orig.X + orig.Width/2
	top := orig.Y - orig.Height/2
	bottom := orig.Y + orig.Height/2
	switch part {
	case "resize-tl":
		left, top = wx, wy
	case "resize-tr":
		right, top = wx, wy
	case "resize-br":
		right, bottom = wx, wy
	case "resize-bl":
		left, bottom = wx, wy
	default:
		return
	}
	if right < left {
		left, right = right, left
	}
	if bottom < top {
		top, bottom = bottom, top
	}
	w := math.Max(24, right-left)
	h := math.Max(24, bottom-top)
	cur.X = (left + right) / 2
	cur.Y = (top + bottom) / 2
	cur.Width = w
	cur.Height = h
	cur.Scale = 1
	s.Items[id] = cur
}

// SetArrowEndpoint repositions either the start (idx=0) or tip (idx=1) of an
// arrow in world coordinates and normalizes its center & local Points.
func (s *Session) SetArrowEndpoint(id string, idx int, wx, wy float64) {
	item, ok := s.Items[id]
	if !ok || item.Kind != model.KindArrow || len(item.Points) < 4 {
		return
	}
	x0, y0 := item.X+item.Points[0], item.Y+item.Points[1]
	x1, y1 := item.X+item.Points[2], item.Y+item.Points[3]
	if idx == 0 {
		x0, y0 = wx, wy
	} else {
		x1, y1 = wx, wy
	}
	cx := (x0 + x1) / 2
	cy := (y0 + y1) / 2
	item.X = cx
	item.Y = cy
	item.Width = math.Max(16, math.Abs(x1-x0))
	item.Height = math.Max(16, math.Abs(y1-y0))
	item.Scale = 1
	item.Rotation = 0
	item.Points = []float64{x0 - cx, y0 - cy, x1 - cx, y1 - cy}
	s.Items[id] = item
}

// FrameContainedIDs returns all top-level non-frame item IDs whose center lies
// inside the given Frame so dragging a Section Frame moves its contents together.
func (s *Session) FrameContainedIDs(frameID string) []string {
	fr, ok := s.Items[frameID]
	if !ok || fr.Kind != model.KindFrame {
		return nil
	}
	minX := fr.X - fr.Width/2
	maxX := fr.X + fr.Width/2
	minY := fr.Y - fr.Height/2
	maxY := fr.Y + fr.Height/2
	var out []string
	for id, it := range s.Items {
		if id == frameID || it.ParentID != "" || it.Kind == model.KindFrame {
			continue
		}
		if it.X >= minX && it.X <= maxX && it.Y >= minY && it.Y <= maxY {
			out = append(out, id)
		}
	}
	return out
}

func localToWorld(items map[string]model.Placement, id string, lx, ly float64) (float64, float64) {
	p := items[id]
	ox, oy := rotate(lx*p.ScaleOrOne(), ly*p.ScaleOrOne(), p.Rotation)
	parent := parentXform(items, id)
	rx, ry := rotate(ox*parent.Scale, oy*parent.Scale, parent.Rot)
	w := World(items, id)
	return w.X + rx, w.Y + ry
}

func dist(x0, y0, x1, y1 float64) float64 {
	return math.Hypot(x1-x0, y1-y0)
}

func paintOrder(items map[string]model.Placement) []string {
	var out []string
	var walk func(parent string)
	walk = func(parent string) {
		kids := children(items, parent)
		for _, id := range kids {
			out = append(out, id)
			walk(id)
		}
	}
	walk("")
	return out
}

func children(items map[string]model.Placement, parent string) []string {
	var ids []string
	for id, item := range items {
		if item.ParentID == parent {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		if items[ids[i]].ZIndex == items[ids[j]].ZIndex {
			return ids[i] < ids[j]
		}
		return items[ids[i]].ZIndex < items[ids[j]].ZIndex
	})
	return ids
}

func rootsOf(items map[string]model.Placement, ids []string) []string {
	sel := map[string]bool{}
	for _, id := range ids {
		sel[id] = true
	}
	var out []string
	for _, id := range ids {
		if _, ok := items[id]; !ok || ancestorSelected(items, id, sel) {
			continue
		}
		out = append(out, id)
	}
	return out
}

func ancestorSelected(items map[string]model.Placement, id string, sel map[string]bool) bool {
	seen := map[string]bool{}
	parent := items[id].ParentID
	for parent != "" && !seen[parent] {
		if sel[parent] {
			return true
		}
		seen[parent] = true
		next, ok := items[parent]
		if !ok {
			break
		}
		parent = next.ParentID
	}
	return false
}

// Move shifts the roots of ids by a board delta, using positions from start
// so repeated calls during one drag do not accumulate error.
func (s *Session) Move(start map[string]model.Placement, ids []string, dx, dy float64) {
	for _, id := range rootsOf(start, ids) {
		base := start[id]
		parent := parentXform(start, id)
		lx, ly := inverseDelta(dx, dy, parent.Scale, parent.Rot)
		cur := s.Items[id]
		cur.X = base.X + lx
		cur.Y = base.Y + ly
		s.Items[id] = cur
	}
}

// Snapshot copies the placements for a gesture.
func (s *Session) Snapshot() map[string]model.Placement {
	out := make(map[string]model.Placement, len(s.Items))
	for id, item := range s.Items {
		out[id] = item
	}
	return out
}

// SetScale sets one object's local scale.
func (s *Session) SetScale(id string, scale float64) {
	item, ok := s.Items[id]
	if !ok {
		return
	}
	if scale < 0.05 {
		scale = 0.05
	}
	item.Scale = scale
	s.Items[id] = item
}

// SetRotation sets one object's local clockwise rotation in degrees.
func (s *Session) SetRotation(id string, deg float64) {
	item, ok := s.Items[id]
	if !ok {
		return
	}
	item.Rotation = deg
	s.Items[id] = item
}

// SetWidth changes a text box width. The center stays put.
func (s *Session) SetWidth(id string, width float64) {
	item, ok := s.Items[id]
	if !ok {
		return
	}
	if width < 40 {
		width = 40
	}
	item.Width = width
	s.Items[id] = item
}

// PlaceImage drops an image centered at a board point. The long edge is 320.
func (s *Session) PlaceImage(assetID string, x, y, pixelW, pixelH float64) string {
	w, h := fitBox(pixelW, pixelH)
	return s.add(model.Placement{
		Kind: model.KindImage, AssetID: assetID,
		X: x, Y: y, Width: w, Height: h, Scale: 1,
	})
}

// PlaceText drops a text box centered at a board point.
func (s *Session) PlaceText(text string, x, y float64) string {
	return s.add(model.Placement{
		Kind: model.KindText, Text: text,
		X: x, Y: y, Width: 240, Height: 72, Scale: 1, FontSize: 18,
	})
}

// PlaceAnnotation adds a drawn stroke, highlighter, rectangle, arrow, or section frame.
// Frames automatically sit behind images so artwork inside them stays interactive.
func (s *Session) PlaceAnnotation(p model.Placement) string {
	id := s.add(p)
	if p.Kind == model.KindFrame {
		s.BringBack(id)
	}
	return id
}

func fitBox(w, h float64) (float64, float64) {
	if w <= 0 || h <= 0 {
		return 320, 240
	}
	if w >= h {
		return 320, 320 * h / w
	}
	return 320 * w / h, 320
}

func (s *Session) add(p model.Placement) string {
	p.ID = model.NewID()
	p.ZIndex = s.maxZ(p.ParentID) + 1
	if p.Scale == 0 {
		p.Scale = 1
	}
	s.Items[p.ID] = p
	s.Selected = map[string]bool{p.ID: true}
	return p.ID
}

func (s *Session) maxZ(parent string) int {
	max := 0
	first := true
	for _, item := range s.Items {
		if item.ParentID != parent {
			continue
		}
		if first || item.ZIndex > max {
			max = item.ZIndex
			first = false
		}
	}
	return max
}

// Delete removes ids and their descendants.
func (s *Session) Delete(ids []string) {
	drop := map[string]bool{}
	for _, id := range ids {
		drop[id] = true
	}
	changed := true
	for changed {
		changed = false
		for id, item := range s.Items {
			if item.ParentID != "" && drop[item.ParentID] && !drop[id] {
				drop[id] = true
				changed = true
			}
		}
	}
	for id := range drop {
		delete(s.Items, id)
		delete(s.Selected, id)
	}
}

// Group wraps the roots of ids in a new group. They must be two or more.
func (s *Session) Group(ids []string) (string, error) {
	roots := rootsOf(s.Items, ids)
	if len(roots) < 2 {
		return "", ErrNeedTwo
	}
	parent := s.Items[roots[0]].ParentID
	for _, id := range roots[1:] {
		if s.Items[id].ParentID != parent {
			parent = ""
			break
		}
	}
	type posed struct {
		id     string
		wx, wy float64
		scale  float64
		rot    float64
	}
	posedItems := make([]posed, 0, len(roots))
	box := Box{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
	for _, id := range roots {
		w := World(s.Items, id)
		b := Bounds(s.Items, id)
		box.MinX = math.Min(box.MinX, b.MinX)
		box.MinY = math.Min(box.MinY, b.MinY)
		box.MaxX = math.Max(box.MaxX, b.MaxX)
		box.MaxY = math.Max(box.MaxY, b.MaxY)
		posedItems = append(posedItems, posed{id, w.X, w.Y, w.Scale, w.Rot})
	}
	cx, cy := (box.MinX+box.MaxX)/2, (box.MinY+box.MaxY)/2
	gx, gy := worldPointToParent(s.Items, parent, cx, cy)
	g := model.Placement{
		ID: model.NewID(), Kind: model.KindGroup, ParentID: parent,
		X: gx, Y: gy, Width: box.MaxX - box.MinX, Height: box.MaxY - box.MinY,
		Scale: 1, ZIndex: s.maxZ(parent) + 1,
	}
	s.Items[g.ID] = g
	gw := World(s.Items, g.ID)
	for _, posedItem := range posedItems {
		child := s.Items[posedItem.id]
		child.ParentID = g.ID
		child.X, child.Y = inverseDelta(posedItem.wx-gw.X, posedItem.wy-gw.Y, gw.Scale, gw.Rot)
		if gw.Scale == 0 {
			gw.Scale = 1
		}
		child.Scale = posedItem.scale / gw.Scale
		child.Rotation = posedItem.rot - gw.Rot
		s.Items[posedItem.id] = child
	}
	s.Selected = map[string]bool{g.ID: true}
	return g.ID, nil
}

// Ungroup lifts the children of each selected group onto that group's parent.
func (s *Session) Ungroup(ids []string) {
	for _, id := range ids {
		g, ok := s.Items[id]
		if !ok || g.Kind != model.KindGroup {
			continue
		}
		kids := children(s.Items, id)
		parentX := parentXform(s.Items, id)
		for _, kid := range kids {
			w := World(s.Items, kid)
			child := s.Items[kid]
			child.ParentID = g.ParentID
			child.X, child.Y = worldPointToParent(s.Items, g.ParentID, w.X, w.Y)
			ps := parentX.Scale
			if ps == 0 {
				ps = 1
			}
			child.Scale = w.Scale / ps
			child.Rotation = w.Rot - parentX.Rot
			s.Items[kid] = child
		}
		delete(s.Items, id)
		delete(s.Selected, id)
	}
}

// NudgeZ moves id forward (dir > 0) or backward among its siblings.
func (s *Session) NudgeZ(id string, dir int) {
	item, ok := s.Items[id]
	if !ok || dir == 0 {
		return
	}
	sibs := children(s.Items, item.ParentID)
	idx := -1
	for i, sib := range sibs {
		if sib == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	swap := idx + 1
	if dir < 0 {
		swap = idx - 1
	}
	if swap < 0 || swap >= len(sibs) {
		return
	}
	sibs[idx], sibs[swap] = sibs[swap], sibs[idx]
	for i, sib := range sibs {
		cur := s.Items[sib]
		cur.ZIndex = i
		s.Items[sib] = cur
	}
}

// BringFront places id above its siblings. BringBack places it below them.
func (s *Session) BringFront(id string) { s.extremeZ(id, true) }

// BringBack places id under its siblings.
func (s *Session) BringBack(id string) { s.extremeZ(id, false) }

func (s *Session) extremeZ(id string, front bool) {
	item, ok := s.Items[id]
	if !ok {
		return
	}
	sibs := children(s.Items, item.ParentID)
	rest := make([]string, 0, len(sibs))
	for _, sib := range sibs {
		if sib != id {
			rest = append(rest, sib)
		}
	}
	if front {
		rest = append(rest, id)
	} else {
		rest = append([]string{id}, rest...)
	}
	for i, sib := range rest {
		cur := s.Items[sib]
		cur.ZIndex = i
		s.Items[sib] = cur
	}
}

// SetNotes sets the annotation on one object.
func (s *Session) SetNotes(id, notes string) {
	item, ok := s.Items[id]
	if !ok {
		return
	}
	item.Notes = notes
	s.Items[id] = item
}

// SetText sets a text object's body.
func (s *Session) SetText(id, text string) {
	item, ok := s.Items[id]
	if !ok {
		return
	}
	item.Text = text
	s.Items[id] = item
}

// Select replaces the selection. An empty id clears it.
func (s *Session) Select(id string, add bool) {
	if id == "" && !add {
		s.Selected = map[string]bool{}
		return
	}
	if !add {
		s.Selected = map[string]bool{}
	}
	if id == "" {
		return
	}
	if add && s.Selected[id] {
		delete(s.Selected, id)
		return
	}
	s.Selected[id] = true
}

// SelectedIDs returns the selection.
func (s *Session) SelectedIDs() []string {
	out := make([]string, 0, len(s.Selected))
	for id := range s.Selected {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
