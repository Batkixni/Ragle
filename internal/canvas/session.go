package canvas

import (
	"errors"
	"math"
	"sort"
	"time"

	"ragle/internal/model"
)

// ErrNeedTwo means Group was asked to wrap fewer than two objects.
var ErrNeedTwo = errors.New("ragle: select two objects to group them")

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
	handle := 8 / s.Zoom
	if s.Zoom == 0 {
		handle = 8
	}
	if len(s.Selected) == 1 {
		for sel := range s.Selected {
			if p, hit := hitHandle(s.Items, sel, wx, wy, handle, s.Zoom); hit {
				return sel, p, true
			}
		}
	}
	order := paintOrder(s.Items)
	for i := len(order) - 1; i >= 0; i-- {
		if ContainsPoint(s.Items, order[i], wx, wy) {
			return order[i], "body", true
		}
	}
	return "", "", false
}

func hitHandle(items map[string]model.Placement, id string, wx, wy, reach, zoom float64) (string, bool) {
	p, ok := items[id]
	if !ok {
		return "", false
	}
	if zoom == 0 {
		zoom = 1
	}
	w := World(items, id)
	scale := w.Scale
	if scale == 0 {
		scale = 1
	}
	// Handles stay a fixed size on screen, so their local offset shrinks as the view zooms in.
	rotateGap := (24 / zoom) / scale
	hw, hh := p.Width/2, p.Height/2
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
