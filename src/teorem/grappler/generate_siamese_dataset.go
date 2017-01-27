package main

import (
	"bytes"
	"fmt"
	"image"
	"math/rand"
	"os"
	"teorem/grappler/caffe"
	"time"

	"github.com/disintegration/imaging"
	"github.com/golang/protobuf/proto"
)

func generateSiameseDataset(dbName string, destW, destH int, todo []string) {
	var max int
	if limit != 0 {
		max = int(limit)
	} else {
		max = int(selectedDBs[0].Entries())
	}

	fmt.Printf("Will generate LMDB database \"%v\" with siamese image blocks of size %v x %v\n", dbName, destW, destH)
	fmt.Printf("Working...\n")

	start := time.Now()

	// creates db
	newDB, err := create(dbName, "lmdb")
	if err != nil {
		fmt.Printf("Could not create DB: %v\n", err)
		return
	}

	// append the newDB to selectedDBs, we are now reading from [0] and writing to [1]
	selectedDBs = append(selectedDBs, newDB)

	type imageJob struct {
		key   []byte
		value []byte
		count int
	}
	randomImages := make(chan imageJob, 100)
	type jobResult struct {
		key        []byte
		value      []byte
		meanValues [6]float64
	}
	results := make(chan jobResult, 100)

	noMoreWork := make(chan int)
	// set up some workers, reading data from the jobs channel and saving them to the result channel
	for i := 0; i < config.Workers; i++ {
		grLog("Image worker started")
		go func() {
			for {
				select {
				case <-noMoreWork:
					grLog("Image worker finished")
					return
				default:
					j := <-randomImages

					// DO THE WORK HERE

					// decode the image, typically a JPEG
					img, err := imaging.Decode(bytes.NewReader(j.value))
					if err != nil {
						continue
					}

					// Split 50%/50% probability between producing similar and dissimilar pairs
					// label=1 -> similar
					// label=0 -> dissimilar
					var dst image.Image
					var pairLabel = rand.Intn(2)
					switch pairLabel {
					case 0:
						// DISSIMILAR PAIR
						// read another one from randomImages
						// the random image could be the same as img! with a large dataset it is probably acceptably
						for {
							j := <-randomImages
							dst, err = imaging.Decode(bytes.NewReader(j.value))
							if err == nil {
								break
							}
						}

					case 1:
						// SIMILAR PAIR
						// select one random operation from the todo
						op := rand.Intn(len(todo))
						w := img.Bounds().Dx()
						h := img.Bounds().Dy()
						switch todo[op] {
						case "cropping", "croppings":
							//max 80% cropping
							pc := float64(config.Generate.PercentCropping) / 100
							nw := int(pc*float64(w) + rand.Float64()*(1-pc)*float64(w))
							nh := int(pc*float64(h) + rand.Float64()*(1-pc)*float64(h))
							x := rand.Intn(w - nw)
							y := rand.Intn(h - nh)
							r := image.Rect(x, y, x+nw, y+nh)
							dst = imaging.Crop(img, r)
						case "brightness":
							dst = imaging.AdjustBrightness(img, float64(rand.Intn(100)-50))
						case "sharpness":
							dst = imaging.Sharpen(img, float64(rand.Intn(6)))
						case "blur":
							dst = imaging.Blur(img, float64(rand.Intn(6)))
						default:
							fmt.Printf("Unknown operation!\n")
							return
						}
					}

					img = imaging.Resize(img, destW, destH, imaging.Lanczos)
					dst = imaging.Resize(dst, destW, destH, imaging.Lanczos)

					// no need to check type assertion since imaging always returns NRGBA
					img2, _ := img.(*image.NRGBA)
					dst2, _ := dst.(*image.NRGBA)

					d := &caffe.Datum{}
					channels := int32(6)
					width := int32(destW)
					height := int32(destH)
					label := int32(pairLabel)
					d.Channels = &channels
					d.Width = &width
					d.Height = &height
					d.Label = &label
					// skip alpha channel (!)
					d.Data = append(img2.Pix[0:(destW*destH)*3], dst2.Pix[0:(destW*destH)*3]...)

					var r jobResult
					r.key = j.key
					r.value, err = proto.Marshal(d)

					for i := range r.meanValues {
						r.meanValues[i] = meanInt(d.Data[(destW*destH)*i : (destW*destH)*(i+1)])
					}

					if err == nil {
						results <- r
					} else {
						fmt.Printf("Protobuf marshal failed: %v\n", err)
					}

					if debugMode {
						//write images pair to disk
						n := fmt.Sprintf("image_%v_%v_A.jpg", j.count, pairLabel)
						writer, _ := os.Create(n)
						imaging.Encode(writer, img, imaging.JPEG)
						writer.Close()

						n = fmt.Sprintf("image_%v_%v_B.jpg", j.count, pairLabel)
						writer, _ = os.Create(n)
						imaging.Encode(writer, dst, imaging.JPEG)
						writer.Close()
					}
				}
			}
		}()
	}

	// set up a goroutine to keep the channel randomImages filled
	// to close it send a value to the quit channel
	noMoreRandom := make(chan int)
	go func() {
		grLog("Random data loader started")
		var count int
		for {
			select {
			case <-noMoreRandom:
				grLog("Random data loader finished")
				return
			default:
				// if there is room, and it looks like we will need them, add to the random channel
				if len(randomImages) < 100 {
					var j imageJob
					j.key, j.value, err = selectedDBs[0].GetRandom()
					if err == nil {
						j.count = count
						randomImages <- j
						count++
					}
				}
			}
		}
	}()

	var wCount, writeFailures int
	var meanSums [3]float64
	grLog("DB writer started")
	for r := range results {
		// create unique key
		k := fmt.Sprintf("%010d", wCount)
		err = selectedDBs[1].Put([]byte(k), r.value)
		if err != nil {
			fmt.Printf("\nCouldn't save to db: %v\n", err)
			writeFailures++
			if writeFailures > 20 {
				fmt.Printf("\nMore than 20 write errors, shutting down\n")
				break
			}
			continue
		}
		wCount++
		for i := 0; i < 3; i++ {
			meanSums[i] += r.meanValues[i]   //add first image (channel 0,1,2)
			meanSums[i] += r.meanValues[i+3] //add second image (channel 3,4,5)
		}
		fmt.Printf("\r[%v:%v] (images: %v, results: %v) %s (%v bytes)", wCount, max, len(randomImages), len(results), k, len(r.value))
		if wCount >= max {
			break
		}
	}
	grLog("DB writer finished")

	noMoreRandom <- 1 // close the randomImages producer
	for i := 0; i < config.Workers; i++ {
		noMoreWork <- 1 // tell the workers to go home
	}
	stop := time.Since(start)
	fmt.Printf("\nDone in %.4v\n", stop)

	// print out mean values
	fmt.Printf("\nImage mean values:\n")
	for i := 0; i < 3; i++ {
		fmt.Printf("Channel %v: %f\n", i, meanSums[i]/float64(wCount*2))
	}
}
