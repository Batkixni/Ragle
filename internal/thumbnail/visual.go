package thumbnail

import (
	"fmt"
	"image"
	"math"
	"math/bits"
	"sort"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"

	"ragle/internal/model"
)

// ExtractVisualFromImage computes 64-bit pHash, 64-bit dHash, and a 5-color
// dominant OKLab palette from an already-decoded image.
func ExtractVisualFromImage(img image.Image) (uint64, uint64, []model.PaletteColor) {
	if img == nil {
		return 0, 0, nil
	}
	return ComputePHash(img), ComputeDHash(img), ExtractPalette(img, 5)
}

// ExtractVisualFromPath opens an image file and extracts pHash, dHash, and palette.
func ExtractVisualFromPath(path string) (uint64, uint64, []model.PaletteColor, error) {
	img, err := imaging.Open(path, imaging.AutoOrientation(true))
	if err != nil {
		return 0, 0, nil, err
	}
	ph, dh, pal := ExtractVisualFromImage(img)
	return ph, dh, pal, nil
}

// ComputeDHash computes a 64-bit difference hash (9x8 grayscale horizontal gradient).
func ComputeDHash(img image.Image) uint64 {
	small := imaging.Resize(img, 9, 8, imaging.Linear)
	var gray [8][9]float64
	b := small.Bounds()
	for y := 0; y < 8; y++ {
		for x := 0; x < 9; x++ {
			r, g, bl, _ := small.At(b.Min.X+x, b.Min.Y+y).RGBA()
			gray[y][x] = 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
		}
	}
	var hash uint64
	var bit uint
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if gray[y][x] > gray[y][x+1] {
				hash |= 1 << bit
			}
			bit++
		}
	}
	return hash
}

// ComputePHash computes a 64-bit perceptual DCT hash (32x32 grayscale -> top-left 8x8 DCT).
func ComputePHash(img image.Image) uint64 {
	const N = 32
	small := imaging.Resize(img, N, N, imaging.Linear)
	var lum [N][N]float64
	b := small.Bounds()
	for y := 0; y < N; y++ {
		for x := 0; x < N; x++ {
			r, g, bl, _ := small.At(b.Min.X+x, b.Min.Y+y).RGBA()
			lum[y][x] = 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
		}
	}

	// Precompute 1D DCT cosine basis for low frequencies u=0..7.
	var cosT [8][N]float64
	for u := 0; u < 8; u++ {
		for x := 0; x < N; x++ {
			cosT[u][x] = math.Cos(math.Pi * float64(2*x+1) * float64(u) / float64(2*N))
		}
	}

	// Separable 2D DCT: rows first for v=0..7, then cols for u=0..7.
	var tmp [N][8]float64
	for y := 0; y < N; y++ {
		for v := 0; v < 8; v++ {
			sum := 0.0
			for x := 0; x < N; x++ {
				sum += lum[y][x] * cosT[v][x]
			}
			tmp[y][v] = sum
		}
	}
	vals := make([]float64, 0, 64)
	var coeffs [64]float64
	idx := 0
	for u := 0; u < 8; u++ {
		for v := 0; v < 8; v++ {
			sum := 0.0
			for y := 0; y < N; y++ {
				sum += tmp[y][v] * cosT[u][y]
			}
			coeffs[idx] = sum
			if idx > 0 { // exclude DC term (0,0) from median
				vals = append(vals, sum)
			}
			idx++
		}
	}
	sort.Float64s(vals)
	median := vals[len(vals)/2]

	var hash uint64
	for i := 0; i < 64; i++ {
		if i == 0 {
			continue
		}
		if coeffs[i] > median {
			hash |= 1 << uint(i)
		}
	}
	return hash
}

// HammingDistance returns the number of differing bits between two 64-bit hashes.
func HammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// VisualSimilarity combines pHash and dHash distances into a [0, 1] similarity score.
func VisualSimilarity(pA, dA, pB, dB uint64) float64 {
	if (pA == 0 && dA == 0) || (pB == 0 && dB == 0) {
		return 0
	}
	pSim := 1.0 - float64(HammingDistance(pA, pB))/64.0
	dSim := 1.0 - float64(HammingDistance(dA, dB))/64.0
	return 0.65*pSim + 0.35*dSim
}

type okSample struct {
	l, a, b float64
}

// ExtractPalette extracts up to k dominant colors using deterministic k-means in OKLab space.
func ExtractPalette(img image.Image, k int) []model.PaletteColor {
	if img == nil || k <= 0 {
		return nil
	}
	small := imaging.Fit(img, 48, 48, imaging.Linear)
	bounds := small.Bounds()
	samples := make([]okSample, 0, bounds.Dx()*bounds.Dy())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r16, g16, b16, a16 := small.At(x, y).RGBA()
			if a16 < 0x2000 { // skip mostly transparent pixels
				continue
			}
			l, a, b := srgbToOKLab(float64(r16>>8)/255.0, float64(g16>>8)/255.0, float64(b16>>8)/255.0)
			samples = append(samples, okSample{l: l, a: a, b: b})
		}
	}
	if len(samples) == 0 {
		return nil
	}
	if len(samples) < k {
		k = len(samples)
	}

	// Deterministic farthest-point seeding (k-means++ without RNG).
	centers := make([]okSample, 0, k)
	centers = append(centers, samples[len(samples)/2])
	for len(centers) < k {
		bestIdx := 0
		bestDist := -1.0
		for i, s := range samples {
			minD := math.MaxFloat64
			for _, c := range centers {
				d := okDist2(s, c)
				if d < minD {
					minD = d
				}
			}
			if minD > bestDist {
				bestDist = minD
				bestIdx = i
			}
		}
		if bestDist <= 1e-9 {
			break
		}
		centers = append(centers, samples[bestIdx])
	}

	k = len(centers)
	counts := make([]int, k)
	for iter := 0; iter < 10; iter++ {
		sums := make([]okSample, k)
		for i := range counts {
			counts[i] = 0
		}
		for _, s := range samples {
			best := 0
			bestD := okDist2(s, centers[0])
			for j := 1; j < k; j++ {
				d := okDist2(s, centers[j])
				if d < bestD {
					bestD = d
					best = j
				}
			}
			sums[best].l += s.l
			sums[best].a += s.a
			sums[best].b += s.b
			counts[best]++
		}
		for j := 0; j < k; j++ {
			if counts[j] > 0 {
				inv := 1.0 / float64(counts[j])
				centers[j] = okSample{
					l: sums[j].l * inv,
					a: sums[j].a * inv,
					b: sums[j].b * inv,
				}
			}
		}
	}

	total := float64(len(samples))
	out := make([]model.PaletteColor, 0, k)
	for j := 0; j < k; j++ {
		if counts[j] == 0 {
			continue
		}
		r, g, b := okLabToSRGB(centers[j].l, centers[j].a, centers[j].b)
		out = append(out, model.PaletteColor{
			Hex:   fmt.Sprintf("#%02X%02X%02X", r, g, b),
			L:     centers[j].l,
			A:     centers[j].a,
			B:     centers[j].b,
			Ratio: float64(counts[j]) / total,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ratio > out[j].Ratio })
	return out
}

// ColorDistanceToPalette computes the best perceptual OKLab distance between a
// target hex color and an asset's dominant palette (lower is closer; 0 is exact).
func ColorDistanceToPalette(targetHex string, palette []model.PaletteColor) (float64, bool) {
	if len(palette) == 0 {
		return math.MaxFloat64, false
	}
	r, g, b, ok := ParseHexRGB(targetHex)
	if !ok {
		return math.MaxFloat64, false
	}
	tl, ta, tb := srgbToOKLab(float64(r)/255.0, float64(g)/255.0, float64(b)/255.0)
	target := okSample{l: tl, a: ta, b: tb}

	best := math.MaxFloat64
	for _, c := range palette {
		d := math.Sqrt(okDist2(target, okSample{l: c.L, a: c.A, b: c.B}))
		// Gently favor colors that occupy a larger area of the image.
		weight := 1.0 + (1.0-c.Ratio)*0.22
		score := d * weight
		if score < best {
			best = score
		}
	}
	return best, true
}

// ParseHexRGB parses "#RRGGBB" or "RRGGBB" into 8-bit RGB channels.
func ParseHexRGB(s string) (uint8, uint8, uint8, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8((v >> 16) & 0xFF), uint8((v >> 8) & 0xFF), uint8(v & 0xFF), true
}

func okDist2(x, y okSample) float64 {
	dl := x.l - y.l
	da := x.a - y.a
	db := x.b - y.b
	return dl*dl + da*da + db*db
}

func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func linearToSRGB(c float64) uint8 {
	if c <= 0 {
		return 0
	}
	if c >= 1 {
		return 255
	}
	var s float64
	if c <= 0.0031308 {
		s = 12.92 * c
	} else {
		s = 1.055*math.Pow(c, 1.0/2.4) - 0.055
	}
	v := math.Round(s * 255.0)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// srgbToOKLab implements Björn Ottosson's OKLab perceptual color transform.
func srgbToOKLab(sr, sg, sb float64) (float64, float64, float64) {
	r := srgbToLinear(sr)
	g := srgbToLinear(sg)
	b := srgbToLinear(sb)

	l := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)

	L := 0.2104542553*l + 0.7936177850*m - 0.0040720468*s
	A := 1.9779984951*l - 2.4285922050*m + 0.4505937099*s
	B := 0.0259040371*l + 0.7827717662*m - 0.8086757660*s
	return L, A, B
}

func okLabToSRGB(L, A, B float64) (uint8, uint8, uint8) {
	l_ := L + 0.3963377774*A + 0.2158037573*B
	m_ := L - 0.1055613458*A - 0.0638541728*B
	s_ := L - 0.0894841775*A - 1.2914855480*B

	l := l_ * l_ * l_
	m := m_ * m_ * m_
	s := s_ * s_ * s_

	r := +4.0767416621*l - 3.3077115913*m + 0.2309699292*s
	g := -1.2684380046*l + 2.6097574011*m - 0.3413193965*s
	b := -0.0041960863*l - 0.7034186147*m + 1.7076147010*s
	return linearToSRGB(r), linearToSRGB(g), linearToSRGB(b)
}
