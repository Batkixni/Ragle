package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	pathOverride = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { pathOverride = "" })

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.LibraryPath != "" || got.Language != "" {
		t.Fatalf("missing file: %+v", got)
	}
	want := Config{LibraryPath: `D:\libs\one`, Language: "zh-Hant"}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}
