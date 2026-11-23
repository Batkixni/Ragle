package canvas

import (
	"testing"

	"raglex/internal/model"
)

func TestSessionUndoRedo(t *testing.T) {
	s := NewSession(nil)
	s.RecordUndo()
	id := s.PlaceText("Hello", 100, 100)
	if len(s.Items) != 1 || !s.CanUndo() {
		t.Fatalf("expected 1 item and CanUndo=true")
	}
	if !s.Undo() || len(s.Items) != 0 {
		t.Fatalf("Undo should remove placed item, got len=%d", len(s.Items))
	}
	if !s.CanRedo() || !s.Redo() || len(s.Items) != 1 || s.Items[id].Text != "Hello" {
		t.Fatalf("Redo should restore item, got len=%d", len(s.Items))
	}
}

func TestArrowHitAndEndpoints(t *testing.T) {
	s := NewSession(nil)
	s.Zoom = 1
	id := s.PlaceAnnotation(model.Placement{
		Kind:        model.KindArrow,
		X:           100,
		Y:           100,
		Width:       200,
		Height:      200,
		Scale:       1,
		StrokeWidth: 3,
		Points:      []float64{-100, -100, 100, 100}, // diagonal from (0,0) to (200,200)
	})
	// Hit on diagonal midpoint (100,100) MUST succeed.
	if hid, part, ok := s.Hit(100, 100); !ok || hid != id || part != "body" {
		t.Fatalf("expected arrow body hit at midpoint, got ok=%v id=%q part=%q", ok, hid, part)
	}
	// Empty corner of bounding box (0,200) MUST NOT hit the diagonal arrow!
	if _, _, ok := s.Hit(0, 200); ok {
		t.Fatalf("empty corner of diagonal arrow bounding box should not hit arrow")
	}
	// Endpoint handles MUST hit arrow-p0 and arrow-p1!
	if _, part, ok := s.Hit(0, 0); !ok || part != "arrow-p0" {
		t.Fatalf("expected arrow-p0 at (0,0), got ok=%v part=%q", ok, part)
	}
	if _, part, ok := s.Hit(200, 200); !ok || part != "arrow-p1" {
		t.Fatalf("expected arrow-p1 at (200,200), got ok=%v part=%q", ok, part)
	}
	s.SetArrowEndpoint(id, 1, 300, 100)
	got := s.Items[id]
	if got.X != 150 || got.Y != 50 {
		t.Fatalf("expected recentered arrow (150,50), got (%.1f,%.1f)", got.X, got.Y)
	}
}

func TestFrameBorderHitAndContainedMove(t *testing.T) {
	s := NewSession(nil)
	s.Zoom = 1
	fid := s.PlaceAnnotation(model.Placement{
		Kind:   model.KindFrame,
		Text:   "Mood",
		X:      200,
		Y:      200,
		Width:  600,
		Height: 500,
		Scale:  1,
	})
	imgID := s.PlaceImage("a1", 200, 200, 100, 100)
	s.Select("", false)

	// Clicking empty interior of frame (-50, 200) outside the 320x320 image
	// should NOT select frame (click-through interior!).
	if hid, _, ok := s.Hit(-50, 200); ok && hid == fid {
		t.Fatalf("clicking empty interior of frame should not swallow click")
	}
	// Clicking top-left title label above (-100, -50) MUST hit frame!
	if hid, _, ok := s.Hit(-60, -60); !ok || hid != fid {
		t.Fatalf("expected frame title hit, got ok=%v id=%q", ok, hid)
	}
	contained := s.FrameContainedIDs(fid)
	if len(contained) != 1 || contained[0] != imgID {
		t.Fatalf("FrameContainedIDs=%v, want [%s]", contained, imgID)
	}
}
