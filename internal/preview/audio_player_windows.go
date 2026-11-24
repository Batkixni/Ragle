//go:build windows

package preview

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"raglex/internal/ffmpeg"
)

var (
	winmmDLL       = syscall.NewLazyDLL("winmm.dll")
	mciSendStringW = winmmDLL.NewProc("mciSendStringW")
	mciAliasSeq    atomic.Uint64
)

// AudioPlayer provides direct native audio playback with play/pause/seek/volume
// using Windows MCI (winmm.dll) and zero CGO.
type AudioPlayer struct {
	mu       sync.Mutex
	alias    string
	tempFile string
	playing  bool
	duration float64
	volume   float64
	closed   bool
}

// OpenAudioPlayer opens path for native playback. OGG/FLAC files are
// transparently decoded to a temporary WAV when ffmpeg is available.
func OpenAudioPlayer(path string) (*AudioPlayer, error) {
	playPath := path
	var tempWav string
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".ogg" || ext == ".flac" || ext == ".opus" {
		if ff := ffmpeg.Find(); ff != "" {
			f, ferr := os.CreateTemp("", "raglex-audio-*.wav")
			if ferr == nil {
				tempWav = f.Name()
				_ = f.Close()
				cmd := exec.Command(ff, "-y", "-v", "error", "-i", path, "-ac", "2", "-ar", "44100", tempWav)
				hideCmd(cmd)
				if cmd.Run() == nil {
					playPath = tempWav
				} else {
					_ = os.Remove(tempWav)
					tempWav = ""
				}
			}
		}
	}

	alias := fmt.Sprintf("raglex_aud_%d", mciAliasSeq.Add(1))
	quoted := strings.ReplaceAll(playPath, `"`, "")
	openCmd := fmt.Sprintf(`open "%s" type mpegvideo alias %s`, quoted, alias)
	if strings.EqualFold(filepath.Ext(playPath), ".wav") {
		openCmd = fmt.Sprintf(`open "%s" type waveaudio alias %s`, quoted, alias)
	}
	if _, err := mciSend(openCmd); err != nil {
		// Retry generic MCI auto-detect.
		if _, err2 := mciSend(fmt.Sprintf(`open "%s" alias %s`, quoted, alias)); err2 != nil {
			if tempWav != "" {
				_ = os.Remove(tempWav)
			}
			return nil, fmt.Errorf("open audio: %w", err2)
		}
	}
	_, _ = mciSend(fmt.Sprintf("set %s time format milliseconds", alias))

	var durSec float64
	if out, err := mciSend(fmt.Sprintf("status %s length", alias)); err == nil {
		if ms, perr := strconv.ParseFloat(strings.TrimSpace(out), 64); perr == nil && ms > 0 {
			durSec = ms / 1000.0
		}
	}

	return &AudioPlayer{
		alias:    alias,
		tempFile: tempWav,
		duration: durSec,
		volume:   1.0,
	}, nil
}

func (p *AudioPlayer) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	// If at end, rewind to start before playing.
	cur, tot := p.positionLocked()
	if tot > 0 && cur >= tot-0.15 {
		_, _ = mciSend(fmt.Sprintf("seek %s to 0", p.alias))
	}
	if _, err := mciSend(fmt.Sprintf("play %s", p.alias)); err != nil {
		return err
	}
	p.playing = true
	return nil
}

func (p *AudioPlayer) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	_, err := mciSend(fmt.Sprintf("pause %s", p.alias))
	p.playing = false
	return err
}

func (p *AudioPlayer) Toggle() error {
	if p.Playing() {
		return p.Pause()
	}
	return p.Play()
}

func (p *AudioPlayer) Seek(seconds float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	if seconds < 0 {
		seconds = 0
	}
	if p.duration > 0 && seconds > p.duration {
		seconds = p.duration
	}
	ms := int64(seconds * 1000)
	if p.playing {
		_, err := mciSend(fmt.Sprintf("play %s from %d", p.alias, ms))
		return err
	}
	_, err := mciSend(fmt.Sprintf("seek %s to %d", p.alias, ms))
	return err
}

func (p *AudioPlayer) SetVolume(v float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	p.volume = v
	_, _ = mciSend(fmt.Sprintf("setaudio %s volume to %d", p.alias, int(v*1000)))
}

func (p *AudioPlayer) Playing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	if mode, err := mciSend(fmt.Sprintf("status %s mode", p.alias)); err == nil {
		p.playing = strings.EqualFold(strings.TrimSpace(mode), "playing")
	}
	return p.playing
}

func (p *AudioPlayer) Position() (cur, total float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, p.duration
	}
	return p.positionLocked()
}

func (p *AudioPlayer) positionLocked() (float64, float64) {
	var cur float64
	if out, err := mciSend(fmt.Sprintf("status %s position", p.alias)); err == nil {
		if ms, perr := strconv.ParseFloat(strings.TrimSpace(out), 64); perr == nil {
			cur = ms / 1000.0
		}
	}
	if p.duration <= 0 {
		if out, err := mciSend(fmt.Sprintf("status %s length", p.alias)); err == nil {
			if ms, perr := strconv.ParseFloat(strings.TrimSpace(out), 64); perr == nil && ms > 0 {
				p.duration = ms / 1000.0
			}
		}
	}
	return cur, p.duration
}

func (p *AudioPlayer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	p.playing = false
	_, _ = mciSend(fmt.Sprintf("stop %s", p.alias))
	_, _ = mciSend(fmt.Sprintf("close %s", p.alias))
	if p.tempFile != "" {
		_ = os.Remove(p.tempFile)
		p.tempFile = ""
	}
	return nil
}

func mciSend(command string) (string, error) {
	cmdPtr, err := syscall.UTF16PtrFromString(command)
	if err != nil {
		return "", err
	}
	var retBuf [256]uint16
	r1, _, _ := mciSendStringW.Call(
		uintptr(unsafe.Pointer(cmdPtr)),
		uintptr(unsafe.Pointer(&retBuf[0])),
		uintptr(len(retBuf)),
		0,
	)
	if r1 != 0 {
		return "", fmt.Errorf("mci error %d for %q", r1, command)
	}
	return syscall.UTF16ToString(retBuf[:]), nil
}
