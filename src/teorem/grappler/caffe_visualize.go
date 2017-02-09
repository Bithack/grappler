package main

import (
	"fmt"
	"image/color"
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
			di := 0
			for f := 0; f < int(dims[0]); f++ {
				fmt.Printf("Filter %v:\n", f)
				if dims[1] == 3 {

					// assume the filter works on R G B channels

					img := imaging.New(int(dims[2]), int(dims[3]), color.NRGBA{0, 0, 0, 0})
					size := int(dims[2] * dims[3])
					data := make([]float64, 3*size)
					ddi := 0
					max := 0.0
					min := math.MaxFloat64

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
					img2 := imaging.Resize(img, 200, 200, imaging.NearestNeighbor)
					n := fmt.Sprintf("filter_%v_rgb.jpg", f)
					writer, _ := os.Create(n)
					fmt.Printf("Saving %v\n", n)
					imaging.Encode(writer, img2, imaging.JPEG)
					writer.Close()

				} else {

					for d := 0; d < int(dims[1]); d++ {

						img := imaging.New(int(dims[2]), int(dims[3]), color.NRGBA{0, 0, 0, 0})

						data := make([]float64, int(dims[2]*dims[3]))
						ddi := 0
						max := 0.0
						min := math.MaxFloat64
						for r := 0; r < int(dims[2]); r++ {
							for c := 0; c < int(dims[3]); c++ {
								data[ddi] = float64(m.BlobProto.Data[di])
								max = math.Max(max, data[ddi])
								min = math.Min(min, data[ddi])
								ddi++
								di++
							}
						}
						// subtract min and scale to [0 255]
						for y := 0; y < int(dims[3]); y++ {
							for x := 0; x < int(dims[2]); x++ {
								u := uint8(255 * (data[y*int(dims[2])+x] - min) / (max - min))
								img.Set(x, y, color.NRGBA{u, u, u, 255})
							}
						}

						img2 := imaging.Resize(img, 200, 200, imaging.NearestNeighbor)
						n := fmt.Sprintf("filter_%v_%v.jpg", f, d)
						writer, _ := os.Create(n)
						fmt.Printf("Saving %v\n", n)
						imaging.Encode(writer, img2, imaging.JPEG)
						writer.Close()

					}
				}

			}
		}

	}
}
