package canvas

import (
	"math"
	"testing"
	"time"

	"ragle/internal/model"
)

func item(id string, x, y, w, h, scale, rot float64, parent string) model.Placement {
	return model.Placement{
		ID: id, Kind: model.KindImage, X: x, Y: y, Width: w, Height: h,
		Scale: scale, Rotation: rot, ParentID: parent,
	}
}

func TestWorldParentScaleAndRotation(t *testing.T) {
	items := map[string]model.Placement{
		"p": item("p", 100, 50, 20, 10, 2, 90, ""),
		"c": item("c", 10, 0, 4, 4, 1, 0, "p"),
	}
	w := World(items, "c")
	if math.Abs(w.X-100) > 1e-6 || math.Abs(w.Y-70) > 1e-6 {
		t.Fatalf("center %v %v", w.X, w.Y)
	}
	if math.Abs(w.Scale-2) > 1e-6 || math.Abs(w.Rot-90) > 1e-6 {
		t.Fatalf("scale %v rot %v", w.Scale, w.Rot)
	}
	if !ContainsPoint(items, "c", w.X, w.Y) {
		t.Fatal("center should hit the child")
	}
	if ContainsPoint(items, "c", 0, 0) {
		t.Fatal("origin should miss the child")
	}
}

func TestZoomKeepsPointUnderCursor(t *testing.T) {
	s := NewSession(nil)
	wx, wy := s.ScreenToWorld(100, 80)
	s.ZoomAt(100, 80, 2)
	gotX, gotY := s.ScreenToWorld(100, 80)
	if math.Abs(gotX-wx) > 1e-6 || math.Abs(gotY-wy) > 1e-6 {
		t.Fatalf("cursor world moved from %v,%v to %v,%v", wx, wy, gotX, gotY)
	}
}

func TestCullSkipsOffscreen(t *testing.T) {
	s := NewSession([]model.Placement{
		item("near", 10, 10, 20, 20, 1, 0, ""),
		item("far", 10000, 10000, 20, 20, 1, 0, ""),
	})
	got := s.Visible(200, 200)
	if len(got) != 1 || got[0] != "near" {
		t.Fatalf("visible %v", got)
	}
}

func TestMoveUsesParentSpace(t *testing.T) {
	s := NewSession([]model.Placement{
		item("p", 0, 0, 10, 10, 2, 0, ""),
		item("c", 5, 0, 4, 4, 1, 0, "p"),
	})
	start := s.Snapshot()
	s.Move(start, []string{"c"}, 4, 0)
	if math.Abs(s.Items["c"].X-7) > 1e-6 || math.Abs(s.Items["c"].Y) > 1e-6 {
		t.Fatalf("child local %+v", s.Items["c"])
	}
	start = s.Snapshot()
	s.Move(start, []string{"p", "c"}, 3, 0)
	if math.Abs(s.Items["p"].X-3) > 1e-6 {
		t.Fatalf("parent %v", s.Items["p"].X)
	}
	if math.Abs(s.Items["c"].X-7) > 1e-6 {
		t.Fatalf("child should ride with the parent, local x %v", s.Items["c"].X)
	}
}

func TestGroupPreservesWorldCenterAndDeleteDropsChildren(t *testing.T) {
	s := NewSession([]model.Placement{
		item("a", 0, 0, 20, 10, 1, 0, ""),
		item("b", 40, 0, 20, 10, 1, 0, ""),
	})
	before := World(s.Items, "b")
	gid, err := s.Group([]string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	after := World(s.Items, "b")
	if math.Abs(after.X-before.X) > 1e-6 || math.Abs(after.Y-before.Y) > 1e-6 {
		t.Fatalf("world moved from %v to %v", before, after)
	}
	if s.Items["b"].ParentID != gid {
		t.Fatalf("parent %s", s.Items["b"].ParentID)
	}
	s.Delete([]string{gid})
	if _, ok := s.Items["a"]; ok {
		t.Fatal("child a survived the group")
	}
	if _, ok := s.Items["b"]; ok {
		t.Fatal("child b survived the group")
	}
}

func TestUngroupRestoresParent(t *testing.T) {
	s := NewSession([]model.Placement{
		item("a", 10, 20, 8, 8, 1, 0, ""),
		item("b", 30, 20, 8, 8, 1, 0, ""),
	})
	gid, err := s.Group([]string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	s.Ungroup([]string{gid})
	if _, ok := s.Items[gid]; ok {
		t.Fatal("group remains")
	}
	if s.Items["a"].ParentID != "" || s.Items["b"].ParentID != "" {
		t.Fatalf("parents %q %q", s.Items["a"].ParentID, s.Items["b"].ParentID)
	}
}

func TestFlushWaitsThenWritesOnce(t *testing.T) {
	s := NewSession([]model.Placement{item("a", 0, 0, 10, 10, 1, 0, "")})
	now := time.Now()
	if _, ok := s.Flush(now, false); ok {
		t.Fatal("clean board flushed")
	}
	s.Touch(now)
	s.Move(s.Snapshot(), []string{"a"}, 1, 0)
	if _, ok := s.Flush(now.Add(100*time.Millisecond), false); ok {
		t.Fatal("flushed during the drag window")
	}
	items, ok := s.Flush(now.Add(FlushDelay), false)
	if !ok || len(items) != 1 {
		t.Fatalf("deadline flush %v %d", ok, len(items))
	}
	if _, ok := s.Flush(now.Add(time.Second), true); ok {
		t.Fatal("flushed again without a new edit")
	}
	s.Touch(now)
	if _, ok := s.Flush(now, true); !ok {
		t.Fatal("pointer-up should flush immediately")
	}
}

func TestHitPrefersFrontSibling(t *testing.T) {
	s := NewSession([]model.Placement{
		{ID: "back", Kind: model.KindImage, X: 0, Y: 0, Width: 40, Height: 40, Scale: 1, ZIndex: 0},
		{ID: "front", Kind: model.KindImage, X: 0, Y: 0, Width: 40, Height: 40, Scale: 1, ZIndex: 2},
	})
	id, part, ok := s.Hit(0, 0)
	if !ok || id != "front" || part != "body" {
		t.Fatalf("hit %s %s %v", id, part, ok)
	}
}
