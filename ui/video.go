package ui

import (
	"errors"
	"fmt"
	"image"
	"io"
	"os/exec"
	"sync"
	"time"

	gui "github.com/egoist/mygo/ui"

	"ragle/internal/thumbnail"
)

const (
	videoPreviewFPS     = 24
	videoPreviewMaxEdge = 960
)

// ffmpegVideoDecoder streams decoded RGBA frames from ffmpeg into MyGo's
// native ui.VideoPlayer so MP4/MOV/MKV/WEBM assets play smoothly in-app.
type ffmpegVideoDecoder struct {
	ffmpeg string
	path   string
	w, h   int

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdout io.ReadCloser
	buf    []byte
	closed bool
}

func newFFmpegVideoDecoder(path string, hintW, hintH int) (*ffmpegVideoDecoder, error) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, exec.ErrNotFound
	}
	w, h := hintW, hintH
	if w <= 0 || h <= 0 {
		pw, ph, _, perr := thumbnail.Probe(path)
		if perr == nil && pw > 0 && ph > 0 {
			w, h = pw, ph
		}
	}
	if w <= 0 || h <= 0 {
		w, h = 640, 360
	}
	if w > videoPreviewMaxEdge || h > videoPreviewMaxEdge {
		if w >= h {
			h = h * videoPreviewMaxEdge / w
			w = videoPreviewMaxEdge
		} else {
			w = w * videoPreviewMaxEdge / h
			h = videoPreviewMaxEdge
		}
	}
	// Ensure even dimensions >= 2 for ffmpeg rawvideo scaling.
	if w < 2 {
		w = 2
	}
	if h < 2 {
		h = 2
	}
	w &^= 1
	h &^= 1

	d := &ffmpegVideoDecoder{
		ffmpeg: ffmpeg,
		path:   path,
		w:      w,
		h:      h,
		buf:    make([]byte, w*h*4),
	}
	if err := d.start(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *ffmpegVideoDecoder) start() error {
	d.stopLocked()
	if d.closed {
		return io.EOF
	}
	scale := fmt.Sprintf("scale=%d:%d:flags=bilinear", d.w, d.h)
	cmd := exec.Command(d.ffmpeg,
		"-v", "error",
		"-i", d.path,
		"-an",
		"-vf", scale,
		"-r", fmt.Sprintf("%d", videoPreviewFPS),
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"pipe:1",
	)
	hideCmdWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	d.cmd = cmd
	d.stdout = stdout
	return nil
}

func (d *ffmpegVideoDecoder) stopLocked() {
	if d.stdout != nil {
		_ = d.stdout.Close()
		d.stdout = nil
	}
	if d.cmd != nil {
		if d.cmd.Process != nil {
			_ = d.cmd.Process.Kill()
		}
		_ = d.cmd.Wait()
		d.cmd = nil
	}
}

func (d *ffmpegVideoDecoder) Next() (gui.VideoFrame, error) {
	d.mu.Lock()
	stdout := d.stdout
	closed := d.closed
	d.mu.Unlock()
	if closed || stdout == nil {
		return gui.VideoFrame{}, io.EOF
	}
	if _, err := io.ReadFull(stdout, d.buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return gui.VideoFrame{}, io.EOF
		}
		return gui.VideoFrame{}, err
	}
	img := &image.RGBA{
		Pix:    d.buf,
		Stride: d.w * 4,
		Rect:   image.Rect(0, 0, d.w, d.h),
	}
	return gui.VideoFrame{
		Image:    img,
		Duration: time.Second / videoPreviewFPS,
	}, nil
}

func (d *ffmpegVideoDecoder) Rewind() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return io.EOF
	}
	return d.start()
}

func (d *ffmpegVideoDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	d.stopLocked()
	return nil
}
