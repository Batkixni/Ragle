package thumbnail

import (
	"bytes"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"raglex/internal/ffmpeg"
)

// Available reports whether ffmpeg can be started (bundled, managed, or PATH).
func Available() bool { return ffmpeg.Available() }

// Probe reads the first video stream's size and duration.
// It returns an error when neither ffprobe nor ffmpeg is available.
func Probe(path string) (width, height int, duration float64, err error) {
	if probe := ffmpeg.FindFFprobe(); probe != "" {
		return probeWith(probe, path)
	}
	ff := ffmpeg.Find()
	if ff == "" {
		return 0, 0, 0, exec.ErrNotFound
	}
	return probeFFmpeg(ff, path)
}

func probeWith(ffprobe, path string) (int, int, float64, error) {
	cmd := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=width,height", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=0", path)
	hideCmd(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, 0, err
	}
	var w, h int
	var dur float64
	for _, line := range strings.Split(string(out), "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "width":
			w, _ = strconv.Atoi(val)
		case "height":
			h, _ = strconv.Atoi(val)
		case "duration":
			dur, _ = strconv.ParseFloat(val, 64)
		}
	}
	return w, h, dur, nil
}

var (
	durationRE = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)
	sizeRE     = regexp.MustCompile(`Video:.*?(\d{2,5})x(\d{2,5})`)
)

func probeFFmpeg(ffmpeg, path string) (int, int, float64, error) {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-i", path)
	hideCmd(cmd)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run() // ffmpeg exits non-zero when no output file is given
	text := buf.String()
	var dur float64
	if m := durationRE.FindStringSubmatch(text); m != nil {
		hh, _ := strconv.Atoi(m[1])
		mm, _ := strconv.Atoi(m[2])
		ss, _ := strconv.ParseFloat(m[3], 64)
		dur = float64(hh*3600+mm*60) + ss
	}
	var w, h int
	if m := sizeRE.FindStringSubmatch(text); m != nil {
		w, _ = strconv.Atoi(m[1])
		h, _ = strconv.Atoi(m[2])
	}
	if w == 0 && h == 0 && dur == 0 {
		return 0, 0, 0, exec.ErrNotFound
	}
	return w, h, dur, nil
}

// ExtractFrame writes one JPEG frame from a video. It tries one second in,
// then the first frame.
func ExtractFrame(path, dest string) error {
	ff := ffmpeg.Find()
	if ff == "" {
		return exec.ErrNotFound
	}
	err := runFrame(ff, path, dest, "1")
	if err != nil {
		return runFrame(ff, path, dest, "0")
	}
	return nil
}

func runFrame(ff, path, dest, ss string) error {
	cmd := exec.Command(ff, "-y", "-ss", ss, "-i", path, "-frames:v", "1", dest)
	hideCmd(cmd)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	return cmd.Run()
}
