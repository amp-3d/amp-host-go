package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

type TexEval interface {
	URI() string

	Reset(width, height int) error

	// EvalAt returns the L and A values for the given u,v coordinates.
	// The u,v coordinates are in the range 0..1 where (0,0) is the bottom-left corner.
	EvalAt(u, v float32) (L, A float32)
}

func main() {

	RenderToFile(64, 64, &Tex{
		NameURI: "panel-stroke-bravo.png",
		At:      wyvillDot,
		ScaleL:  1,
		ScaleA:  1,
	})
	RenderToFile(64, 64, &Tex{
		NameURI: "dot-black.png",
		At:      wyvillDot,
		ScaleL:  0,
		ScaleA:  1,
	})
	RenderToFile(16, 16, &Tex{
		NameURI: "panel-fill-bg.png",
		At: func(u, v float32) (L, A float32) {
			L = .3 + .2*v // lighter at top
			A = 1
			return
		},
		ScaleL: 1,
		ScaleA: 1,
	})
	RenderToFile(32, 32, &Tex{
		NameURI: "panel-fill-tapered.png",
		At: func(u, v float32) (L, A float32) {
			L, A = wyvillDot(u, v)
			L *= (0.3 - .2*v) // lighter at top
			A = float32(math.Pow(float64(A), .4))
			return
		},
		ScaleL: 1,
		ScaleA: 1,
	})
}

func toByte(x float32) uint8 {
	if x <= 0.001 {
		return 0
	}
	if x >= 0.999 {
		return 0xFF
	}
	return uint8(x * 255.49)
}

func clamp01(x float32) float32 {
	if x < 0 {
		return 0
	} else if x >= 1 {
		return 1
	}
	return x
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func sqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	return float32(math.Sqrt(float64(x)))
}

func RenderToFile(
	dx, dy int,
	tex TexEval,
) error {

	err := tex.Reset(dx, dy)
	if err != nil {
		return err
	}

	// Create a colored image of the given width and height.
	img := image.NewNRGBA(image.Rect(0, 0, dx, dy))

	scaleX := 1.0 / (float32(dx) - 0.499)
	scaleY := 1.0 / (float32(dy) - 0.499)
	for y := 0; y < dy; y++ {
		v := 1 - scaleY * float32(y) // v=0 corresponds to bottom
		for x := 0; x < dx; x++ {
			u := scaleX * float32(x)
			L, A := tex.EvalAt(u, v)
			L8 := toByte(L)
			img.Set(x, y, color.NRGBA{
				R: L8,
				G: L8,
				B: L8,
				A: toByte(A),
			})
		}
	}

	imgFile, err := os.Create(tex.URI())
	if err != nil {
		return err
	}
	defer imgFile.Close()

	if err := png.Encode(imgFile, img); err != nil {
		imgFile.Close()
		return err
	}

	if err := imgFile.Close(); err != nil {
		return err
	}
	return nil
}

type Tex struct {
	NameURI string
	ScaleL  float32
	ScaleA  float32
	At      func(u, v float32) (L, A float32)
}

func (tex *Tex) URI() string {
	return tex.NameURI
}

func (tex *Tex) Reset(width, height int) error {
	return nil
}

func (tex *Tex) EvalAt(u, v float32) (L, A float32) {
	L1, A1 := tex.At(u, v)
	L = tex.ScaleL * L1
	A = tex.ScaleA * A1
	return
}

var wyvillDot = func(u, v float32) (L, A float32) {

	// Remap 0..1 to -1..1, scale y into/out of x
	x := 2*u - 1
	y := 2*v - 1

	r := clamp01(sqrt(x*x + y*y))
	r2 := r * r

	// xscale := v*2
	// if xscale > 1 {
	//     xscale = 2 - xscale
	// }

	// rscale := u*2
	// if rscale > 1 {
	//     rscale = 2 - rscale
	// }
	// rscale = clamp01(rscale + tex.scaleNudge)

	// // r := 1 - nv * xscale
	// r := 1 - nv // * rscale * tex.Sigma
	// r2 := r*r
	A = r2*(r2*(1.888888888-0.444444444*r2)-2.444444444) + 1
	L = A
	return
}
