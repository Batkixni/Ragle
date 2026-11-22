// Package embed implements Raglex's built-in, zero-dependency local visual
// embedding engine (128-dim L2-normalized OKLab spatial + gradient + color
// histogram + frequency descriptor). It runs completely inside the app binary
// with no external server, GPU, or Python runtime required.
package embed

import (
	"image"
	"math"

	"github.com/disintegration/imaging"
)

const Dim = 128

// ExtractFromPath decodes an image file and returns its 128-dim L2-normalized embedding.
func ExtractFromPath(path string) ([]float32, error) {
	img, err := imaging.Open(path, imaging.AutoOrientation(true))
	if err != nil {
		return nil, err
	}
	return ExtractFromImage(img), nil
}

// ExtractFromImage computes a 128-dim L2-normalized visual descriptor:
//   - [0..47]   4x4 spatial OKLab mean color (composition & color layout)
//   - [48..79]  4x4 spatial horizontal & vertical edge energy (framing & layout)
//   - [80..111] 32-bin global OKLab color & lightness distribution (mood & palette)
//   - [112..127] 4x4 low-frequency DCT texture signature (minimal vs busy)
func ExtractFromImage(img image.Image) []float32 {
	if img == nil {
		return nil
	}
	const sz = 64
	small := imaging.Resize(img, sz, sz, imaging.Linear)
	b := small.Bounds()

	var L [sz][sz]float64
	var A [sz][sz]float64
	var B [sz][sz]float64
	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			r16, g16, b16, _ := small.At(b.Min.X+x, b.Min.Y+y).RGBA()
			l, a, bl := srgbToOKLab(float64(r16>>8)/255.0, float64(g16>>8)/255.0, float64(b16>>8)/255.0)
			L[y][x] = l
			A[y][x] = a
			B[y][x] = bl
		}
	}

	vec := make([]float64, Dim)

	// 1) 4x4 spatial OKLab mean color (48 dims).
	const cell = sz / 4
	const invCell = 1.0 / float64(cell*cell)
	idx := 0
	for gy := 0; gy < 4; gy++ {
		for gx := 0; gx < 4; gx++ {
			var sl, sa, sb float64
			for dy := 0; dy < cell; dy++ {
				y := gy*cell + dy
				for dx := 0; dx < cell; dx++ {
					x := gx*cell + dx
					sl += L[y][x]
					sa += A[y][x]
					sb += B[y][x]
				}
			}
			vec[idx] = sl * invCell * 1.1
			vec[idx+1] = sa * invCell * 3.0
			vec[idx+2] = sb * invCell * 3.0
			idx += 3
		}
	}

	// 2) 4x4 spatial edge gradients (32 dims).
	for gy := 0; gy < 4; gy++ {
		for gx := 0; gx < 4; gx++ {
			var gxSum, gySum float64
			for dy := 0; dy < cell; dy++ {
				y := gy*cell + dy
				yNext := min(y+1, sz-1)
				for dx := 0; dx < cell; dx++ {
					x := gx*cell + dx
					xNext := min(x+1, sz-1)
					gxSum += math.Abs(L[y][xNext] - L[y][x])
					gySum += math.Abs(L[yNext][x] - L[y][x])
				}
			}
			vec[idx] = gxSum * invCell * 2.5
			vec[idx+1] = gySum * invCell * 2.5
			idx += 2
		}
	}

	// 3) 32-bin global OKLab distribution: 8 lightness + 12 hue + 12 chroma-weighted hue.
	const invPix = 1.0 / float64(sz*sz)
	lHist := vec[80:88]
	hHist := vec[88:100]
	cHist := vec[100:112]
	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			lb := int(L[y][x] * 8.0)
			if lb < 0 {
				lb = 0
			} else if lb > 7 {
				lb = 7
			}
			lHist[lb] += invPix * 1.4

			a, bl := A[y][x], B[y][x]
			chroma := math.Sqrt(a*a + bl*bl)
			angle := math.Atan2(bl, a) // -pi..pi
			hb := int(((angle + math.Pi) / (2 * math.Pi)) * 12.0)
			if hb < 0 {
				hb = 0
			} else if hb > 11 {
				hb = 11
			}
			if chroma > 0.03 {
				hHist[hb] += invPix * 1.6
				cHist[hb] += invPix * chroma * 8.0
			}
		}
	}

	// 4) 4x4 low-frequency DCT on downsampled 16x16 luminance (16 dims).
	const dN = 16
	var sub [dN][dN]float64
	for y := 0; y < dN; y++ {
		for x := 0; x < dN; x++ {
			sub[y][x] = L[y*4+2][x*4+2]
		}
	}
	dIdx := 112
	for u := 0; u < 4; u++ {
		for v := 0; v < 4; v++ {
			sum := 0.0
			for y := 0; y < dN; y++ {
				cy := math.Cos(math.Pi * float64(2*y+1) * float64(u) / float64(2*dN))
				for x := 0; x < dN; x++ {
					cx := math.Cos(math.Pi * float64(2*x+1) * float64(v) / float64(2*dN))
					sum += sub[y][x] * cy * cx
				}
			}
			vec[dIdx] = (sum / float64(dN*dN)) * 1.5
			dIdx++
		}
	}

	return normalize(vec)
}

// CosineSimilarity returns the cosine similarity in [0, 1] between two normalized vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	if dot < 0 {
		return 0
	}
	if dot > 1 {
		return 1
	}
	return dot
}

// MeanCentroid computes the L2-normalized centroid of a set of vectors.
func MeanCentroid(vecs [][]float32) []float32 {
	if len(vecs) == 0 {
		return nil
	}
	dim := len(vecs[0])
	sum := make([]float64, dim)
	n := 0
	for _, v := range vecs {
		if len(v) != dim {
			continue
		}
		for i, x := range v {
			sum[i] += float64(x)
		}
		n++
	}
	if n == 0 {
		return nil
	}
	return normalize(sum)
}

func normalize(v []float64) []float32 {
	var norm2 float64
	for _, x := range v {
		norm2 += x * x
	}
	out := make([]float32, len(v))
	if norm2 <= 1e-12 {
		return out
	}
	inv := 1.0 / math.Sqrt(norm2)
	for i, x := range v {
		out[i] = float32(x * inv)
	}
	return out
}

func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

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
