// Package model holds the records stored in the library database.
package model

import (
	"path/filepath"
	"strings"
	"time"
)

// Storage modes choose what happens to a file at import.
const (
	StorageIndex   = "index"
	StorageSymlink = "symlink"
	StorageMove    = "move"
	StorageCopy    = "copy"
)

// Placement kinds.
const (
	KindImage = "image"
	KindText  = "text"
	KindGroup = "group"
)

// Media kinds used by the library filter.
const (
	MediaImage = "image"
	MediaVideo = "video"
	MediaAudio = "audio"
	MediaFont  = "font"
	MediaPDF   = "pdf"
	MediaOther = "other"
)

// SchemaVersion is the meta bucket value written by this build.
const SchemaVersion = "1"

// PaletteColor is one extracted dominant color in sRGB hex and OKLab space.
type PaletteColor struct {
	Hex   string  `msgpack:"hex"`
	L     float64 `msgpack:"l"`
	A     float64 `msgpack:"a"`
	B     float64 `msgpack:"b"`
	Ratio float64 `msgpack:"ratio"`
}

// Asset is one item in the library. The file at Path is the user's when
// StorageMode is index or symlink; copy and move own the file inside the
// library.
type Asset struct {
	ID            string         `msgpack:"id"`
	Path          string         `msgpack:"path"`
	DisplayName   string         `msgpack:"display_name,omitempty"`
	StorageMode   string         `msgpack:"storage_mode"`
	Hash          string         `msgpack:"hash"`
	Mime          string         `msgpack:"mime"`
	Width         int            `msgpack:"width"`
	Height        int            `msgpack:"height"`
	Duration      float64        `msgpack:"duration"`
	ThumbnailPath string         `msgpack:"thumbnail_path"`
	Rating        int            `msgpack:"rating"`
	Missing       bool           `msgpack:"missing"`
	SourceURL     string         `msgpack:"source_url,omitempty"`
	SourceTitle   string         `msgpack:"source_title,omitempty"`
	Notes         string         `msgpack:"notes,omitempty"`
	Palette       []PaletteColor `msgpack:"palette,omitempty"`
	PHash         uint64         `msgpack:"phash,omitempty"`
	DHash         uint64         `msgpack:"dhash,omitempty"`
	StackID       string         `msgpack:"stack_id,omitempty"`
	CreatedAt     time.Time      `msgpack:"created_at"`
}

// Stack groups related versions or angles of an asset into one collapsible card.
type Stack struct {
	ID        string    `msgpack:"id"`
	Name      string    `msgpack:"name"`
	CoverID   string    `msgpack:"cover_id"`
	AssetIDs  []string  `msgpack:"asset_ids"`
	CreatedAt time.Time `msgpack:"created_at"`
}

// Label returns DisplayName if set, falling back to the base filename of Path.
func (a Asset) Label() string {
	if s := strings.TrimSpace(a.DisplayName); s != "" {
		return s
	}
	return filepath.Base(a.Path)
}

// RenameEntry records one asset change inside a batch rename for Undo.
type RenameEntry struct {
	AssetID        string `msgpack:"asset_id"`
	OldDisplayName string `msgpack:"old_display_name"`
	NewDisplayName string `msgpack:"new_display_name"`
	OldPath        string `msgpack:"old_path"`
	NewPath        string `msgpack:"new_path"`
	RenamedDisk    bool   `msgpack:"renamed_disk"`
}

// RenameBatch stores one reversible batch rename operation.
type RenameBatch struct {
	ID        string        `msgpack:"id"`
	CreatedAt time.Time     `msgpack:"created_at"`
	Entries   []RenameEntry `msgpack:"entries"`
}

// Tag is a label on assets and projects. ParentID nests child tags under a
// parent tag (empty string for root tags). Color is an optional #RRGGBB hex.
type Tag struct {
	ID        string    `msgpack:"id"`
	Name      string    `msgpack:"name"`
	ParentID  string    `msgpack:"parent_id,omitempty"`
	Color     string    `msgpack:"color,omitempty"`
	CreatedAt time.Time `msgpack:"created_at"`
}

// Project is a working context. It references library assets and does not
// own their files.
type Project struct {
	ID          string    `msgpack:"id"`
	Name        string    `msgpack:"name"`
	Description string    `msgpack:"description"`
	CreatedAt   time.Time `msgpack:"created_at"`
}

// Board is one canvas inside a project.
type Board struct {
	ID        string    `msgpack:"id"`
	ProjectID string    `msgpack:"project_id"`
	Name      string    `msgpack:"name"`
	CreatedAt time.Time `msgpack:"created_at"`
}

// Placement is one object on a board.
//
// X and Y are the object's center in its parent's space. Width and Height
// are the unscaled box. Scale and Rotation (degrees, clockwise, Y down)
// are also relative to the parent. A parent scales and rotates its children.
// Notes is an annotation. Text is the body of a text object.
type Placement struct {
	ID       string  `msgpack:"id"`
	BoardID  string  `msgpack:"board_id"`
	Kind     string  `msgpack:"kind"`
	AssetID  string  `msgpack:"asset_id"`
	ParentID string  `msgpack:"parent_id"`
	X        float64 `msgpack:"x"`
	Y        float64 `msgpack:"y"`
	Width    float64 `msgpack:"width"`
	Height   float64 `msgpack:"height"`
	Scale    float64 `msgpack:"scale"`
	Rotation float64 `msgpack:"rotation"`
	ZIndex   int     `msgpack:"z_index"`
	Notes    string  `msgpack:"notes"`
	Text     string  `msgpack:"text"`
	FontSize float64 `msgpack:"font_size"`
}

// ScaleOrOne returns Scale, or 1 when it was never set.
func (p Placement) ScaleOrOne() float64 {
	if p.Scale == 0 {
		return 1
	}
	return p.Scale
}

// MediaKind maps a MIME type to image, video, or other.
// Note: SVG is kept under MediaImage for filtering, while IsSVG/IsFont/IsAudio/IsPDF
// identify specialized native designer preview formats.
func MediaKind(mime string) string {
	switch {
	case len(mime) >= 6 && mime[:6] == "image/":
		return MediaImage
	case len(mime) >= 6 && mime[:6] == "video/":
		return MediaVideo
	default:
		return MediaOther
	}
}

// IsSVG reports whether the asset is an SVG vector graphic.
func IsSVG(mime, path string) bool {
	if strings.EqualFold(mime, "image/svg+xml") {
		return true
	}
	return strings.EqualFold(filepath.Ext(path), ".svg")
}

// IsFont reports whether the asset is a TrueType or OpenType font file.
func IsFont(mime, path string) bool {
	low := strings.ToLower(mime)
	if strings.HasPrefix(low, "font/") || low == "application/x-font-ttf" || low == "application/vnd.ms-opentype" {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".otf":
		return true
	}
	return false
}

// IsAudio reports whether the asset is a previewable audio file.
func IsAudio(mime, path string) bool {
	if strings.HasPrefix(strings.ToLower(mime), "audio/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".wav", ".flac", ".ogg", ".m4a", ".aac", ".opus", ".wma":
		return true
	}
	return false
}

// IsPDF reports whether the asset is a PDF document.
func IsPDF(mime, path string) bool {
	if strings.EqualFold(mime, "application/pdf") {
		return true
	}
	return strings.EqualFold(filepath.Ext(path), ".pdf")
}
