package ui

import (
	"fmt"
	"runtime"
	"strings"

	gui "github.com/egoist/mygo/ui"
)

type shortcutDef struct {
	ID       string
	Category string // "nav", "edit", "view"
	LabelKey string
	Default  string // portable canonical spec, e.g. "Cmd+Alt+C", "F2", "Space"
}

var shortcutDefs = []shortcutDef{
	// Navigation
	{ID: "quick_goto", Category: "nav", LabelKey: "shortcuts.act.quick_goto", Default: "Cmd+T"},
	{ID: "search", Category: "nav", LabelKey: "shortcuts.act.search", Default: "Cmd+F"},
	{ID: "toggle_sidebar", Category: "nav", LabelKey: "shortcuts.act.toggle_sidebar", Default: "Cmd+B"},
	{ID: "open_settings", Category: "nav", LabelKey: "shortcuts.act.open_settings", Default: "Cmd+,"},
	{ID: "quick_look", Category: "nav", LabelKey: "shortcuts.act.quick_look", Default: "Space"},
	// Import & Organize
	{ID: "import_files", Category: "edit", LabelKey: "shortcuts.act.import_files", Default: "Cmd+I"},
	{ID: "web_import", Category: "edit", LabelKey: "shortcuts.act.web_import", Default: "Cmd+Shift+U"},
	{ID: "select_all", Category: "edit", LabelKey: "shortcuts.act.select_all", Default: "Cmd+A"},
	{ID: "stack_assets", Category: "edit", LabelKey: "shortcuts.act.stack_assets", Default: "Cmd+G"},
	{ID: "batch_rename", Category: "edit", LabelKey: "shortcuts.act.batch_rename", Default: "F2"},
	{ID: "copy_files", Category: "edit", LabelKey: "shortcuts.act.copy_files", Default: "Cmd+C"},
	{ID: "paste_files", Category: "edit", LabelKey: "shortcuts.act.paste_files", Default: "Cmd+V"},
	{ID: "delete_asset", Category: "edit", LabelKey: "shortcuts.act.delete_asset", Default: "Delete"},
	// View & Edit
	{ID: "focus_mode", Category: "view", LabelKey: "shortcuts.act.focus_mode", Default: "Cmd+Alt+C"},
	{ID: "grayscale", Category: "view", LabelKey: "shortcuts.act.grayscale", Default: "Cmd+Alt+G"},
	{ID: "undo", Category: "view", LabelKey: "shortcuts.act.undo", Default: "Cmd+Z"},
	{ID: "redo", Category: "view", LabelKey: "shortcuts.act.redo", Default: "Cmd+Shift+Z"},
}

var shortcutDefByID = func() map[string]shortcutDef {
	m := make(map[string]shortcutDef, len(shortcutDefs))
	for _, d := range shortcutDefs {
		m[d.ID] = d
	}
	return m
}()

var keyToCanonical = map[gui.Key]string{
	gui.KeyA: "A", gui.KeyB: "B", gui.KeyC: "C", gui.KeyD: "D", gui.KeyE: "E",
	gui.KeyF: "F", gui.KeyG: "G", gui.KeyH: "H", gui.KeyI: "I", gui.KeyJ: "J",
	gui.KeyK: "K", gui.KeyL: "L", gui.KeyM: "M", gui.KeyN: "N", gui.KeyO: "O",
	gui.KeyP: "P", gui.KeyQ: "Q", gui.KeyR: "R", gui.KeyS: "S", gui.KeyT: "T",
	gui.KeyU: "U", gui.KeyV: "V", gui.KeyW: "W", gui.KeyX: "X", gui.KeyY: "Y",
	gui.KeyZ: "Z",
	gui.Key0: "0", gui.Key1: "1", gui.Key2: "2", gui.Key3: "3", gui.Key4: "4",
	gui.Key5: "5", gui.Key6: "6", gui.Key7: "7", gui.Key8: "8", gui.Key9: "9",
	gui.KeyF1: "F1", gui.KeyF2: "F2", gui.KeyF3: "F3", gui.KeyF4: "F4",
	gui.KeyF5: "F5", gui.KeyF6: "F6", gui.KeyF7: "F7", gui.KeyF8: "F8",
	gui.KeyF9: "F9", gui.KeyF10: "F10", gui.KeyF11: "F11", gui.KeyF12: "F12",
	gui.KeySpace:        "Space",
	gui.KeyEnter:        "Enter",
	gui.KeyTab:          "Tab",
	gui.KeyDelete:       "Delete",
	gui.KeyBackspace:    "Backspace",
	gui.KeyInsert:       "Insert",
	gui.KeyHome:         "Home",
	gui.KeyEnd:          "End",
	gui.KeyPageUp:       "PageUp",
	gui.KeyPageDown:     "PageDown",
	gui.KeyLeft:         "Left",
	gui.KeyRight:        "Right",
	gui.KeyUp:           "Up",
	gui.KeyDown:         "Down",
	gui.KeyComma:        ",",
	gui.KeyPeriod:       ".",
	gui.KeySlash:        "/",
	gui.KeySemicolon:    ";",
	gui.KeyQuote:        "'",
	gui.KeyBracketLeft:  "[",
	gui.KeyBracketRight: "]",
	gui.KeyBackslash:    `\`,
	gui.KeyBackquote:    "`",
	gui.KeyMinus:        "-",
	gui.KeyEqual:        "=",
}

var canonicalToKey = func() map[string]gui.Key {
	m := make(map[string]gui.Key, len(keyToCanonical)+4)
	for k, name := range keyToCanonical {
		m[strings.ToLower(name)] = k
	}
	m["del"] = gui.KeyDelete
	m["return"] = gui.KeyEnter
	m["comma"] = gui.KeyComma
	m["period"] = gui.KeyPeriod
	return m
}()

// ParseShortcutSpec parses a canonical shortcut string (e.g. "Cmd+Alt+C", "F2")
// into MyGo modifier flags and Key for the current platform.
func ParseShortcutSpec(spec string) (gui.Modifiers, gui.Key, bool) {
	return parseShortcutSpecForOS(spec, runtime.GOOS)
}

func parseShortcutSpecForOS(spec, goos string) (gui.Modifiers, gui.Key, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "none") {
		return 0, gui.KeyUnknown, false
	}
	var mods gui.Modifiers
	var key gui.Key
	parts := strings.Split(spec, "+")
	for _, raw := range parts {
		tok := strings.TrimSpace(raw)
		if tok == "" {
			continue
		}
		switch strings.ToLower(tok) {
		case "cmd", "command", "primary":
			if goos == "darwin" {
				mods |= gui.Super
			} else {
				mods |= gui.Ctrl
			}
		case "ctrl", "control":
			mods |= gui.Ctrl
		case "alt", "opt", "option":
			mods |= gui.Alt
		case "shift":
			mods |= gui.Shift
		case "super", "meta", "win":
			mods |= gui.Super
		default:
			if k, ok := canonicalToKey[strings.ToLower(tok)]; ok {
				key = k
			} else {
				return 0, gui.KeyUnknown, false
			}
		}
	}
	if key == gui.KeyUnknown {
		return 0, gui.KeyUnknown, false
	}
	return mods, key, true
}

// EncodeShortcutSpec encodes a recorded MyGo modifier+key pair into a portable
// canonical string stored in config.json.
func EncodeShortcutSpec(mods gui.Modifiers, key gui.Key) (string, bool) {
	return encodeShortcutSpecForOS(mods, key, runtime.GOOS)
}

func encodeShortcutSpecForOS(mods gui.Modifiers, key gui.Key, goos string) (string, bool) {
	keyName, ok := keyToCanonical[key]
	if !ok {
		return "", false
	}
	var parts []string
	if goos == "darwin" {
		if mods&gui.Super != 0 {
			parts = append(parts, "Cmd")
		}
		if mods&gui.Ctrl != 0 {
			parts = append(parts, "Ctrl")
		}
	} else {
		if mods&gui.Ctrl != 0 {
			parts = append(parts, "Cmd")
		}
		if mods&gui.Super != 0 {
			parts = append(parts, "Super")
		}
	}
	if mods&gui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&gui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	parts = append(parts, keyName)
	return strings.Join(parts, "+"), true
}

// FormatShortcutForOS formats a canonical shortcut spec using native macOS
// modifier symbols (⌘ ⌥ ⌃ ⇧) on darwin, or standard Ctrl/Alt/Shift labels
// on Windows and Linux.
func FormatShortcutForOS(spec, goos string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "none") {
		return ""
	}
	mods, key, ok := parseShortcutSpecForOS(spec, goos)
	if !ok {
		return spec
	}
	keyName := keyToCanonical[key]
	if goos == "darwin" {
		var b strings.Builder
		if mods&gui.Ctrl != 0 {
			b.WriteString("⌃")
		}
		if mods&gui.Alt != 0 {
			b.WriteString("⌥")
		}
		if mods&gui.Shift != 0 {
			b.WriteString("⇧")
		}
		if mods&gui.Super != 0 {
			b.WriteString("⌘")
		}
		switch key {
		case gui.KeyDelete, gui.KeyBackspace:
			b.WriteString("⌫")
		case gui.KeyEnter:
			b.WriteString("↩")
		case gui.KeyLeft:
			b.WriteString("←")
		case gui.KeyRight:
			b.WriteString("→")
		case gui.KeyUp:
			b.WriteString("↑")
		case gui.KeyDown:
			b.WriteString("↓")
		default:
			b.WriteString(keyName)
		}
		return b.String()
	}

	var parts []string
	if mods&gui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&gui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&gui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	if mods&gui.Super != 0 {
		parts = append(parts, "Win")
	}
	if key == gui.KeyDelete {
		keyName = "Del"
	}
	parts = append(parts, keyName)
	return strings.Join(parts, "+")
}

func (a *App) shortcutSpec(id string) string {
	if a.shortcuts != nil {
		if custom, ok := a.shortcuts[id]; ok {
			return custom
		}
	}
	if def, ok := shortcutDefByID[id]; ok {
		return def.Default
	}
	return ""
}

func (a *App) shortcutLabel(id string) string {
	return FormatShortcutForOS(a.shortcutSpec(id), runtime.GOOS)
}

func (a *App) shortcutBinding(id string) (gui.Modifiers, gui.Key, bool) {
	return ParseShortcutSpec(a.shortcutSpec(id))
}

func (a *App) shortcutIsCustom(id string) bool {
	def, ok := shortcutDefByID[id]
	if !ok {
		return false
	}
	return a.shortcutSpec(id) != def.Default
}

func (a *App) setShortcut(id string, mods gui.Modifiers, key gui.Key) bool {
	if _, ok := shortcutDefByID[id]; !ok {
		return false
	}
	spec, ok := EncodeShortcutSpec(mods, key)
	if !ok {
		return false
	}
	if a.shortcuts == nil {
		a.shortcuts = map[string]string{}
	}
	// Detect and clear any conflicting action bound to the exact same key combo.
	var conflictLabel string
	for _, other := range shortcutDefs {
		if other.ID == id {
			continue
		}
		if strings.EqualFold(a.shortcutSpec(other.ID), spec) {
			a.shortcuts[other.ID] = "none"
			conflictLabel = a.t(other.LabelKey)
		}
	}
	if def := shortcutDefByID[id]; strings.EqualFold(spec, def.Default) {
		delete(a.shortcuts, id)
	} else {
		a.shortcuts[id] = spec
	}
	a.syncConfigFromState()
	if conflictLabel != "" {
		a.notify(fmt.Sprintf(a.t("shortcuts.conflictReplaced"), FormatShortcutForOS(spec, runtime.GOOS), conflictLabel))
	}
	return true
}

func (a *App) resetShortcut(id string) {
	if a.shortcuts != nil {
		delete(a.shortcuts, id)
	}
	a.syncConfigFromState()
}

func (a *App) resetAllShortcuts() {
	a.shortcuts = map[string]string{}
	a.recordingShortcutID = ""
	a.syncConfigFromState()
	a.notify(a.t("shortcuts.resetDone"))
}

func (a *App) settingsOpenTooltip() string {
	base := a.t("settings.title")
	if lbl := a.shortcutLabel("open_settings"); lbl != "" {
		return fmt.Sprintf("%s (%s)", base, lbl)
	}
	return base
}

func (a *App) sidebarToggleTooltip() string {
	base := a.t("shortcuts.act.toggle_sidebar")
	if lbl := a.shortcutLabel("toggle_sidebar"); lbl != "" {
		return fmt.Sprintf("%s (%s)", base, lbl)
	}
	return base
}
