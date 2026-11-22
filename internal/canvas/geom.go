// Package canvas keeps a board in memory: coordinates, hit testing, and
// when those edits are worth writing back. It does not import the UI.
package canvas

import (
	"math"

	"raglex/internal/model"
)

// Xform is an object's center in board space, plus the scale and clockwise
// rotation that map its local box into that space. Y grows downward.
type Xform struct {
	X, Y  float64
	Scale float64
	Rot   float64
}

// World maps id through its parents into board space.
func World(items map[string]model.Placement, id string) Xform {
	chain := chainOf(items, id)
	x, y := 0.0, 0.0
	scale, rot := 1.0, 0.0
	for _, p := range chain {
		ps := p.ScaleOrOne()
		lx, ly := p.X*scale, p.Y*scale
		rx, ry := rotate(lx, ly, rot)
		x += rx
		y += ry
		scale *= ps
		rot += p.Rotation
	}
	return Xform{X: x, Y: y, Scale: scale, Rot: rot}
}

func chainOf(items map[string]model.Placement, id string) []model.Placement {
	var rev []model.Placement
	seen := map[string]bool{}
	for id != "" && !seen[id] {
		p, ok := items[id]
		if !ok {
			break
		}
		seen[id] = true
		rev = append(rev, p)
		id = p.ParentID
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

func parentXform(items map[string]model.Placement, id string) Xform {
	p, ok := items[id]
	if !ok || p.ParentID == "" {
		return Xform{Scale: 1}
	}
	w := World(items, p.ParentID)
	if w.Scale == 0 {
		w.Scale = 1
	}
	return w
}

// WorldToLocal maps a board point into id's unscaled box, centered on the object.
func WorldToLocal(items map[string]model.Placement, id string, wx, wy float64) (float64, float64) {
	w := World(items, id)
	return inverseDelta(wx-w.X, wy-w.Y, w.Scale, w.Rot)
}

func worldPointToParent(items map[string]model.Placement, parentID string, wx, wy float64) (float64, float64) {
	if parentID == "" {
		return wx, wy
	}
	w := World(items, parentID)
	return inverseDelta(wx-w.X, wy-w.Y, w.Scale, w.Rot)
}

func rotate(x, y, deg float64) (float64, float64) {
	rad := deg * math.Pi / 180
	c, s := math.Cos(rad), math.Sin(rad)
	return x*c - y*s, x*s + y*c
}

func inverseDelta(dx, dy, scale, rot float64) (float64, float64) {
	if scale == 0 {
		scale = 1
	}
	x, y := rotate(dx, dy, -rot)
	return x / scale, y / scale
}

// Box is an axis-aligned rectangle in board space.
type Box struct {
	MinX, MinY float64
	MaxX, MaxY float64
}

// Bounds is the axis-aligned box of id after parent transforms.
func Bounds(items map[string]model.Placement, id string) Box {
	p := items[id]
	hw, hh := p.Width/2, p.Height/2
	corners := [4][2]float64{{-hw, -hh}, {hw, -hh}, {hw, hh}, {-hw, hh}}
	w := World(items, id)
	box := Box{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
	for _, c := range corners {
		ox, oy := rotate(c[0]*p.ScaleOrOne(), c[1]*p.ScaleOrOne(), p.Rotation)
		parent := parentXform(items, id)
		rx, ry := rotate(ox*parent.Scale, oy*parent.Scale, parent.Rot)
		x, y := w.X+rx, w.Y+ry
		box.MinX = math.Min(box.MinX, x)
		box.MinY = math.Min(box.MinY, y)
		box.MaxX = math.Max(box.MaxX, x)
		box.MaxY = math.Max(box.MaxY, y)
	}
	return box
}

func (b Box) Contains(x, y float64) bool {
	return x >= b.MinX && x <= b.MaxX && y >= b.MinY && y <= b.MaxY
}

func (b Box) Intersects(o Box) bool {
	return b.MinX <= o.MaxX && b.MaxX >= o.MinX && b.MinY <= o.MaxY && b.MaxY >= o.MinY
}

func (b Box) Empty() bool {
	return b.MaxX < b.MinX || b.MaxY < b.MinY
}

// ContainsPoint reports whether the board point lies in id's box.
func ContainsPoint(items map[string]model.Placement, id string, wx, wy float64) bool {
	p, ok := items[id]
	if !ok {
		return false
	}
	lx, ly := WorldToLocal(items, id, wx, wy)
	return math.Abs(lx) <= p.Width/2 && math.Abs(ly) <= p.Height/2
}
