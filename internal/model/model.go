// Package model holds the records stored in the library database.
package model

import "time"

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
	MediaOther = "other"
)

// SchemaVersion is the meta bucket value written by this build.
const SchemaVersion = "1"

// Asset is one item in the library. The file at Path is the user's when
// StorageMode is index or symlink; copy and move own the file inside the
// library.
type Asset struct {
	ID            string    `msgpack:"id"`
	Path          string    `msgpack:"path"`
	StorageMode   string    `msgpack:"storage_mode"`
	Hash          string    `msgpack:"hash"`
	Mime          string    `msgpack:"mime"`
	Width         int       `msgpack:"width"`
	Height        int       `msgpack:"height"`
	Duration      float64   `msgpack:"duration"`
	ThumbnailPath string    `msgpack:"thumbnail_path"`
	Rating        int       `msgpack:"rating"`
	Missing       bool      `msgpack:"missing"`
	CreatedAt     time.Time `msgpack:"created_at"`
}

// Tag is a label on assets and projects. ParentID nests child tags under a
// parent tag (empty string for root tags).
type Tag struct {
	ID        string    `msgpack:"id"`
	Name      string    `msgpack:"name"`
	ParentID  string    `msgpack:"parent_id,omitempty"`
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
