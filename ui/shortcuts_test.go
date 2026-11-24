package ui

import (
	"testing"

	gui "github.com/egoist/mygo/ui"
)

func TestShortcutFormattingMacAndWindows(t *testing.T) {
	cases := []struct {
		spec string
		mac  string
		win  string
	}{
		{"Cmd+F", "⌘F", "Ctrl+F"},
		{"Cmd+Alt+C", "⌥⌘C", "Ctrl+Alt+C"},
		{"Cmd+Shift+U", "⇧⌘U", "Ctrl+Shift+U"},
		{"Cmd+,", "⌘,", "Ctrl+,"},
		{"Ctrl+Space", "⌃Space", "Ctrl+Space"},
		{"F2", "F2", "F2"},
		{"Delete", "⌫", "Del"},
	}
	for _, tc := range cases {
		if got := FormatShortcutForOS(tc.spec, "darwin"); got != tc.mac {
			t.Errorf("FormatShortcutForOS(%q, darwin) = %q, want %q", tc.spec, got, tc.mac)
		}
		if got := FormatShortcutForOS(tc.spec, "windows"); got != tc.win {
			t.Errorf("FormatShortcutForOS(%q, windows) = %q, want %q", tc.spec, got, tc.win)
		}
	}
}

func TestShortcutRoundTripAndConflictResolution(t *testing.T) {
	// Darwin Super encodes as portable "Cmd"; Ctrl stays "Ctrl".
	if spec, ok := encodeShortcutSpecForOS(gui.Super|gui.Alt, gui.KeyC, "darwin"); !ok || spec != "Cmd+Alt+C" {
		t.Fatalf("darwin encode Super+Alt+C = (%q, %v), want (Cmd+Alt+C, true)", spec, ok)
	}
	if spec, ok := encodeShortcutSpecForOS(gui.Ctrl, gui.KeyK, "darwin"); !ok || spec != "Ctrl+K" {
		t.Fatalf("darwin encode Ctrl+K = (%q, %v), want (Ctrl+K, true)", spec, ok)
	}

	a := NewApp()
	// Rebind focus_mode to Cmd+K.
	if !a.setShortcut("focus_mode", gui.Cmd, gui.KeyK) {
		t.Fatal("setShortcut(focus_mode, Cmd+K) failed")
	}
	if got := a.shortcutSpec("focus_mode"); got != "Cmd+K" {
		t.Fatalf("focus_mode spec = %q, want Cmd+K", got)
	}
	if !a.shortcutIsCustom("focus_mode") {
		t.Fatal("expected focus_mode to be marked custom")
	}

	// Rebinding search to Cmd+K should clear focus_mode conflict automatically.
	if !a.setShortcut("search", gui.Cmd, gui.KeyK) {
		t.Fatal("setShortcut(search, Cmd+K) failed")
	}
	if got := a.shortcutSpec("focus_mode"); got != "none" {
		t.Fatalf("expected conflicting focus_mode to become 'none', got %q", got)
	}

	// Reset all restores defaults cleanly.
	a.resetAllShortcuts()
	if got := a.shortcutSpec("focus_mode"); got != "Cmd+Alt+C" {
		t.Fatalf("after resetAllShortcuts, focus_mode = %q, want Cmd+Alt+C", got)
	}
	if got := a.shortcutSpec("search"); got != "Cmd+F" {
		t.Fatalf("after resetAllShortcuts, search = %q, want Cmd+F", got)
	}
}
