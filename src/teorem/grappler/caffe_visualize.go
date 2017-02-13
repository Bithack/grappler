package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"

	"math"

	"github.com/disintegration/imaging"
)

func caffeVisualize(m *caffeMessage) {
	switch m.T {
	case "BlobProto":
		shape := m.BlobProto.Shape
		dims := shape.GetDim()
		if len(dims) == 4 {

			max := 0.0
			min := math.MaxFloat64
			for _, f := range m.BlobProto.Data {
				max = math.Max(max, float64(f))
				min = math.Min(min, float64(f))
			}

			di := 0
			var imgNum, gridW, gridH int

			maxF := int(dims[0] - 1)
			maxD := int(dims[1] - 1)
			padding := 5
			imageSize := int(dims[3] * dims[2] * 2)
			spacing := imageSize / 10
			var extraW, extraH int

			if dims[1] == 3 {
				imgNum = int(dims[0])
				gridW = int(math.Sqrt(float64(imgNum)))
				gridH = int(dims[0]) / gridW
			} else {
				imgNum = int(dims[0] * dims[1])
				gridW = int(dims[0] + 1) // filters
				gridH = int(dims[1] + 1) // depth
				maxF++
				maxD++
				extraW = spacing
				extraH = spacing
			}

			montage := imaging.New(padding*2+(gridW-1)*spacing+gridW*imageSize+extraW, padding*2+(gridH-1)*spacing+gridH*imageSize+extraH, color.NRGBA{255, 255, 255, 255})

			depthMean := make([]float64, dims[3]*dims[2]*dims[1])
			filterMean := make([]float64, dims[3]*dims[2]*dims[0])

			for f := 0; f <= maxF; f++ {

				fmt.Printf("\r[%v:%v] Scanning kernels...", f+1, int(dims[0]))

				if InterruptRequested {
					fmt.Printf("canceled\n")
					return
				}

				if dims[1] == 3 {

					// assume the filter works on R G B channels

					img := imaging.New(int(dims[2]), int(dims[3]), color.NRGBA{0, 0, 0, 0})
					size := int(dims[2] * dims[3])
					data := make([]float64, 3*size)
					ddi := 0
					max := 0.0
					min := math.MaxFloat64

					// load data while finding max / min
					for ch := 0; ch < 3; ch++ {
						for r := 0; r < int(dims[2]); r++ {
							for c := 0; c < int(dims[3]); c++ {
								data[ddi] = float64(m.BlobProto.Data[di])
								max = math.Max(max, data[ddi])
								min = math.Min(min, data[ddi])
								ddi++
								di++
							}
						}
					}
					// subtract min and scale to [0 255]
					for y := 0; y < int(dims[3]); y++ {
						for x := 0; x < int(dims[2]); x++ {
							r := uint8(255 * (data[0*size+y*int(dims[2])+x] - min) / (max - min))
							g := uint8(255 * (data[1*size+y*int(dims[2])+x] - min) / (max - min))
							b := uint8(255 * (data[2*size+y*int(dims[2])+x] - min) / (max - min))
							img.Set(x, y, color.NRGBA{r, g, b, 255})
						}
					}
					img2 := imaging.Resize(img, imageSize, imageSize, imaging.NearestNeighbor)
					px := f % gridW
					py := f / gridW
					montage = imaging.Paste(montage, img2, image.Point{X: px*imageSize + padding + px*spacing, Y: py*imageSize + padding + py*spacing})
					if InterruptRequested {
						fmt.Printf("canceled\n")
						return
					}
				} else {
					for d := 0; d <= maxD; d++ {
						// image location in grid
						gx := f
						gy := d
						px := gx*imageSize + padding + gx*spacing
						py := gy*imageSize + padding + gy*spacing
						pixelW := imageSize / int(dims[2])
						pixelH := imageSize / int(dims[3])
						// subtract min and scale to [0 255]
						for y := 0; y < int(dims[3]); y++ {
							for x := 0; x < int(dims[2]); x++ {
								switch {
								case d == maxD && f < maxF:
									u := uint8(filterMean[f*int(dims[2]*dims[3])+y*int(dims[2])+x])
									draw.Draw(montage, image.Rect(px+x*pixelW, py+y*pixelH+extraH, px+(x+1)*pixelW, py+(y+1)*pixelH+extraH), &image.Uniform{C: color.NRGBA{u, u, u, 255}}, image.ZP, draw.Over)
								case d < maxD && f == maxF:
									u := uint8(depthMean[d*int(dims[2]*dims[3])+y*int(dims[2])+x])
									draw.Draw(montage, image.Rect(px+x*pixelW+extraW, py+y*pixelH, px+(x+1)*pixelW+extraW, py+(y+1)*pixelH), &image.Uniform{C: color.NRGBA{u, u, u, 255}}, image.ZP, draw.Over)
								case d < maxD && f < maxF:
									u := uint8(255 * (float64(m.BlobProto.Data[f*int(dims[1]*dims[2]*dims[3])+d*int(dims[2]*dims[3])+y*int(dims[2])+x]) - min) / (max - min))
									draw.Draw(montage, image.Rect(px+x*pixelW, py+y*pixelH, px+(x+1)*pixelW, py+(y+1)*pixelH), &image.Uniform{C: color.NRGBA{u, u, u, 255}}, image.ZP, draw.Over)
									depthMean[d*int(dims[2]*dims[3])+y*int(dims[2])+x] += float64(u) / float64(dims[0])
									filterMean[f*int(dims[2]*dims[3])+y*int(dims[2])+x] += float64(u) / float64(dims[1])

								}
							}
						}
					}
				}
			}
			n := fmt.Sprintf("filters.jpg")
			writer, _ := os.Create(n)
			fmt.Printf("Done. Saving %v\n", n)
			imaging.Encode(writer, montage, imaging.JPEG)
			writer.Close()
		}

	}
}
