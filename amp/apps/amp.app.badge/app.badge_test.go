package badge

// import (
// 	"os"

// 	svg "github.com/ajstarks/svgo"
// )

// func main() {

// 	width := 500
// 	height := 500
// 	canvas := svg.New(os.Stdout)
// 	canvas.Start(width, height)
// 	canvas.Circle(width/2, height/2, 100)
// 	canvas.Text(width/2, height/2, "Hello, SVG", "text-anchor:middle;font-size:30px;fill:white")
// 	canvas.End()
// }

// package main

import (
	"image"
	"math"
	"testing"

	"github.com/llgcode/draw2d/draw2dimg"
)

func hexagon(centerX, centerY, size float64) []float64 {
	points := make([]float64, 0)
	for i := 0; i < 6; i++ {
		angle := float64(i) * math.Pi / 3
		x := centerX + size*math.Cos(angle)
		y := centerY + size*math.Sin(angle)
		points = append(points, x, y)
	}
	points = append(points, points[0], points[1]) // Close the hexagon
	return points
}

func spiralHexagons(centerX, centerY, size float64, numRings int) [][]float64 {
	hexagons := [][]float64{}
	directions := [6][2]float64{
		{size * 3 / 2, size * math.Sqrt(3) / 2},
		{0, size * math.Sqrt(3)},
		{-size * 3 / 2, size * math.Sqrt(3) / 2},
		{-size * 3 / 2, -size * math.Sqrt(3) / 2},
		{0, -size * math.Sqrt(3)},
		{size * 3 / 2, -size * math.Sqrt(3) / 2},
	}

	hexagons = append(hexagons, hexagon(centerX, centerY, size))

	for ring := 1; ring <= numRings; ring++ {
		x, y := centerX+float64(ring)*directions[5][0], centerY+float64(ring)*directions[5][1]
		for _, direction := range directions {
			for step := 0; step < ring; step++ {
				hexagons = append(hexagons, hexagon(x, y, size))
				x += direction[0]
				y += direction[1]
			}
		}
	}
	return hexagons
}

func TestThing(*testing.T) {
	size := 20.0
	numRings := 8
	centerX, centerY := 400.0, 400.0

	img := image.NewRGBA(image.Rect(0, 0, 800, 800))
	dest := draw2dimg.NewGraphicContext(img)
	dest.SetStrokeColor(image.Black)
	dest.SetLineWidth(1)

	hexagons := spiralHexagons(centerX, centerY, size, numRings)
	for _, hex := range hexagons {
		dest.BeginPath()
		dest.MoveTo(hex[0], hex[1])
		for i := 2; i < len(hex); i += 2 {
			dest.LineTo(hex[i], hex[i+1])
		}
		dest.Close()
		dest.Stroke()
	}

	draw2dimg.SaveToPngFile("spiral_hexagons.png", img)
}
