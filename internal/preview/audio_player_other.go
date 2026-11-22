//go:build !windows

package preview

import (
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// AudioPlayer provides lightweight audio playback on macOS/Linux using afplay/ffplay.
type AudioPlayer struct {
	mu       sync.Mutex
	path     string
	cmd      *exec.Cmd
	playing  bool
	offset   float64
	started  time.Time
	duration float64
	volume   float64
	closed   bool
}

func OpenAudioPlayer(path string) (*AudioPlayer, error) {
	wf, _ := ExtractWaveform(path, 32)
	return &AudioPlayer{
		path:     path,
		duration: wf.Duration,
		volume:   1.0,
	}, nil
}

func (p *AudioPlayer) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.stopLocked()
	var cmd *exec.Cmd
	if bin, err := exec.LookPath("ffplay"); err == nil {
		cmd = exec.Command(bin, "-nodisp", "-autoexit", "-loglevel", "quiet", "-ss", fmt.Sprintf("%.2f", p.offset), p.path)
	} else if bin, err := exec.LookPath("afplay"); err == nil {
		cmd = exec.Command(bin, p.path)
	} else {
		return fmt.Errorf("no audio player (ffplay/afplay) found")
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd = cmd
	p.started = time.Now()
	p.playing = true
	go func(c *exec.Cmd) {
		_ = c.Wait()
		p.mu.Lock()
		if p.cmd == c {
			p.playing = false
			p.cmd = nil
		}
		p.mu.Unlock()
	}(cmd)
	return nil
}

func (p *AudioPlayer) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.playing {
		p.offset += time.Since(p.started).Seconds()
	}
	p.stopLocked()
	return nil
}

func (p *AudioPlayer) Toggle() error {
	if p.Playing() {
		return p.Pause()
	}
	return p.Play()
}

func (p *AudioPlayer) Seek(seconds float64) error {
	p.mu.Lock()
	wasPlaying := p.playing
	if seconds < 0 {
		seconds = 0
	}
	p.offset = seconds
	p.stopLocked()
	p.mu.Unlock()
	if wasPlaying {
		return p.Play()
	}
	return nil
}

func (p *AudioPlayer) SetVolume(v float64) {
	p.mu.Lock()
	p.volume = v
	p.mu.Unlock()
}

func (p *AudioPlayer) Playing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

func (p *AudioPlayer) Position() (float64, float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cur := p.offset
	if p.playing {
		cur += time.Since(p.started).Seconds()
	}
	if p.duration > 0 && cur > p.duration {
		cur = p.duration
	}
	return cur, p.duration
}

func (p *AudioPlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.stopLocked()
	return nil
}

func (p *AudioPlayer) stopLocked() {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	p.cmd = nil
	p.playing = false
}
