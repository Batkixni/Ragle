package preview

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"raglex/internal/ffmpeg"
)

// Waveform holds normalized amplitude peaks (0..1) and duration for an audio file.
type Waveform struct {
	Peaks      []float32
	Duration   float64 // seconds
	SampleRate int
	Channels   int
}

// ExtractWaveform computes normalized peak bars for WAV (pure Go) or any other
// audio format via ffmpeg (falling back gracefully when ffmpeg is unavailable).
func ExtractWaveform(path string, bars int) (Waveform, error) {
	if bars < 16 {
		bars = 96
	}
	if strings.EqualFold(filepath.Ext(path), ".wav") {
		if wf, err := extractWAVWaveform(path, bars); err == nil {
			return wf, nil
		}
	}
	if wf, err := extractFFmpegWaveform(path, bars); err == nil {
		return wf, nil
	}
	// Graceful deterministic fallback so audio UI still looks crisp without ffmpeg.
	return fallbackWaveform(path, bars), nil
}

func extractWAVWaveform(path string, bars int) (Waveform, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Waveform{}, err
	}
	if len(raw) < 44 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return Waveform{}, errors.New("not a RIFF WAVE file")
	}
	var (
		audioFormat   uint16
		channels      uint16
		sampleRate    uint32
		bitsPerSample uint16
		dataChunk     []byte
	)
	off := 12
	for off+8 <= len(raw) {
		id := string(raw[off : off+4])
		size := int(binary.LittleEndian.Uint32(raw[off+4 : off+8]))
		off += 8
		if size < 0 || off+size > len(raw) {
			size = len(raw) - off
		}
		chunk := raw[off : off+size]
		switch id {
		case "fmt ":
			if len(chunk) >= 16 {
				audioFormat = binary.LittleEndian.Uint16(chunk[0:2])
				channels = binary.LittleEndian.Uint16(chunk[2:4])
				sampleRate = binary.LittleEndian.Uint32(chunk[4:8])
				bitsPerSample = binary.LittleEndian.Uint16(chunk[14:16])
			}
		case "data":
			dataChunk = chunk
		}
		off += size
		if size%2 == 1 {
			off++
		}
	}
	if channels == 0 || sampleRate == 0 || len(dataChunk) == 0 {
		return Waveform{}, errors.New("incomplete WAV header")
	}
	bytesPerSample := int(bitsPerSample) / 8
	frameSize := bytesPerSample * int(channels)
	if frameSize <= 0 {
		return Waveform{}, errors.New("invalid WAV frame size")
	}
	totalFrames := len(dataChunk) / frameSize
	if totalFrames == 0 {
		return Waveform{}, errors.New("empty WAV data")
	}

	peaks := make([]float32, bars)
	var maxPeak float32
	for b := 0; b < bars; b++ {
		start := b * totalFrames / bars
		end := (b + 1) * totalFrames / bars
		if end <= start {
			end = min(start+1, totalFrames)
		}
		step := max(1, (end-start)/128)
		var localMax float32
		for f := start; f < end; f += step {
			base := f * frameSize
			if base+bytesPerSample > len(dataChunk) {
				break
			}
			var amp float32
			switch {
			case audioFormat == 1 && bitsPerSample == 8:
				v := int(dataChunk[base]) - 128
				amp = float32(math.Abs(float64(v))) / 128.0
			case audioFormat == 1 && bitsPerSample == 16:
				v := int16(binary.LittleEndian.Uint16(dataChunk[base : base+2]))
				amp = float32(math.Abs(float64(v))) / 32768.0
			case audioFormat == 1 && bitsPerSample == 24:
				v := int32(dataChunk[base]) | int32(dataChunk[base+1])<<8 | int32(int8(dataChunk[base+2]))<<16
				amp = float32(math.Abs(float64(v))) / 8388608.0
			case audioFormat == 3 && bitsPerSample == 32:
				v := math.Float32frombits(binary.LittleEndian.Uint32(dataChunk[base : base+4]))
				amp = float32(math.Abs(float64(v)))
			default:
				v := int16(binary.LittleEndian.Uint16(dataChunk[base : base+2]))
				amp = float32(math.Abs(float64(v))) / 32768.0
			}
			if amp > localMax {
				localMax = amp
			}
		}
		peaks[b] = localMax
		if localMax > maxPeak {
			maxPeak = localMax
		}
	}
	if maxPeak > 0.001 {
		for i := range peaks {
			peaks[i] = clamp01(peaks[i] / maxPeak)
		}
	}
	return Waveform{
		Peaks:      peaks,
		Duration:   float64(totalFrames) / float64(sampleRate),
		SampleRate: int(sampleRate),
		Channels:   int(channels),
	}, nil
}

func extractFFmpegWaveform(path string, bars int) (Waveform, error) {
	ff := ffmpeg.Find()
	if ff == "" {
		return Waveform{}, exec.ErrNotFound
	}
	const rate = 8000
	cmd := exec.Command(ff,
		"-v", "error",
		"-i", path,
		"-ac", "1",
		"-ar", "8000",
		"-f", "s16le",
		"pipe:1",
	)
	hideCmd(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil && out.Len() == 0 {
		return Waveform{}, err
	}
	pcm := out.Bytes()
	samples := len(pcm) / 2
	if samples == 0 {
		return Waveform{}, io.ErrUnexpectedEOF
	}
	peaks := make([]float32, bars)
	var maxPeak float32
	for b := 0; b < bars; b++ {
		start := b * samples / bars
		end := (b + 1) * samples / bars
		if end <= start {
			end = min(start+1, samples)
		}
		step := max(1, (end-start)/128)
		var localMax float32
		for s := start; s < end; s += step {
			v := int16(binary.LittleEndian.Uint16(pcm[s*2 : s*2+2]))
			amp := float32(math.Abs(float64(v))) / 32768.0
			if amp > localMax {
				localMax = amp
			}
		}
		peaks[b] = localMax
		if localMax > maxPeak {
			maxPeak = localMax
		}
	}
	if maxPeak > 0.001 {
		for i := range peaks {
			peaks[i] = clamp01(peaks[i] / maxPeak)
		}
	}
	return Waveform{
		Peaks:      peaks,
		Duration:   float64(samples) / float64(rate),
		SampleRate: rate,
		Channels:   1,
	}, nil
}

func fallbackWaveform(path string, bars int) Waveform {
	h := fnv.New64a()
	_, _ = h.Write([]byte(filepath.Base(path)))
	seed := h.Sum64()
	peaks := make([]float32, bars)
	for i := 0; i < bars; i++ {
		t := float64(i) / float64(max(1, bars-1))
		env := math.Sin(t * math.Pi)
		wave := 0.35 + 0.55*math.Abs(math.Sin(float64(i)*0.42+float64(seed%17)))
		peaks[i] = clamp01(float32(0.15 + 0.85*env*wave))
	}
	return Waveform{Peaks: peaks}
}

// RenderWaveformThumbnail draws a sleek monochrome audio waveform card thumbnail.
func RenderWaveformThumbnail(wf Waveform, fg, bg color.RGBA) *image.RGBA {
	const w, h = 440, 300
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)

	peaks := wf.Peaks
	if len(peaks) == 0 {
		peaks = fallbackWaveform("audio", 48).Peaks
	}
	bars := 48
	if len(peaks) < bars {
		bars = len(peaks)
	}
	padX := 34
	usableW := w - padX*2
	barStep := float64(usableW) / float64(bars)
	barW := max(3, int(barStep*0.56))
	midY := h / 2
	maxHalfH := (h - 88) / 2

	for i := 0; i < bars; i++ {
		idx := i * len(peaks) / bars
		p := peaks[idx]
		if p < 0.08 {
			p = 0.08
		}
		halfH := max(4, int(float64(maxHalfH)*float64(p)))
		x0 := padX + int(float64(i)*barStep)
		draw.Draw(img, image.Rect(x0, midY-halfH, x0+barW, midY+halfH), &image.Uniform{C: fg}, image.Point{}, draw.Src)
	}
	return img
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
