package main

import (
	"bytes"
	"fmt"
	"image"
	"math/rand"
	"os"
	"sync/atomic"
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

	start := time.Now()

	// creates db
	newDB, err := create(dbName, "lmdb")
	if err != nil {
		fmt.Printf("Could not create DB: %v\n", err)
		return
	}

	logFile, err := os.Create(dbName + ".log.txt")
	if err != nil {
		fmt.Printf("Could not create logfile: %v\n", err)
		return
	}
	defer logFile.Close()

	grLogsf(logFile, "Will generate LMDB database \"%v\" with siamese image blocks of size %v x %v\n", dbName, destW, destH)
	if config.Generate.CropAll {
		grLogsf(logFile, "All images will be randomly cropped by 0-%v%%\n", 100-config.Generate.PercentCropping)
	}
	grLogsf(logFile, "Operations: %v\n", todo)
	grLogsf(logFile, "Number of operations on each image: %v\n", config.Generate.OperationCount)
	fmt.Printf("Working...\n")

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

	// stat counters
	var (
		cr int32
		bl int32
		sh int32
		co int32
		ga int32
		no int32
		br int32
	)

	// result channels need room for the final results to drop in when we are closing down
	results := make(chan jobResult, 100)

	noMoreWork := make(chan int, config.Workers)
	// set up some workers, reading data from the jobs channel and saving them to the result channel
	for i := 0; i < config.Workers; i++ {
		grLog(fmt.Sprintf("Image worker %v started", i))
		go func(w int) {
			for {
				select {
				case <-noMoreWork:
					grLog(fmt.Sprintf("Image worker %v finished", w))
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
						for {
							k := <-randomImages
							dst, err = imaging.Decode(bytes.NewReader(k.value))
							if err == nil {
								// if we happenened the get the same image, change the label to similar
								if bytes.Compare(k.key, j.key) == 0 {
									pairLabel = 1
								}
								break
							}
							if config.Generate.CropAll {
								dst = crop(dst)
							}
						}

					case 1:
						// SIMILAR PAIR

						dst = imaging.Clone(img)

						if config.Generate.CropAll {
							// dst is cropped before other operations...
							dst = crop(dst)
						}

						for j := 0; j < config.Generate.OperationCount; j++ {

							// select one random operation from the todo list
							op := rand.Intn(len(todo))

							switch todo[op] {
							case "cropping", "croppings", "crop":
								atomic.AddInt32(&cr, 1)
								dst = crop(dst)
							case "brightness":
								atomic.AddInt32(&br, 1)
								dst = imaging.AdjustBrightness(dst, float64(rand.Intn(100)-50))
							case "sharpness":
								atomic.AddInt32(&sh, 1)
								dst = imaging.Sharpen(dst, float64(rand.Intn(6)))
							case "gamma":
								atomic.AddInt32(&ga, 1)
								dst = imaging.AdjustGamma(dst, 1.5-rand.Float64()) // gamma 0.5 to 1.5
							case "contrast":
								atomic.AddInt32(&co, 1)
								dst = imaging.AdjustContrast(dst, float64(rand.Intn(100)-50)) // contrast -50 to 50
							case "blur":
								atomic.AddInt32(&bl, 1)
								dst = imaging.Blur(dst, 1.0)
							case "nothing", "none", "noop":
								atomic.AddInt32(&no, 1)
								dst = imaging.Clone(dst)
							default:
								fmt.Printf("Unknown operation! Available: crop, brightness, sharpness, gamma, contrast, blur, none\n")
								return
							}
						}
					}

					if config.Generate.CropAll {
						// crop the first image (original) in the pair
						img = crop(img)
					}

					// now resize to final desination
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
					size := destW * destH

					// skip alpha channel! pixel in img2 and dst2 are stored as R G B A R G B A ...
					// in d.Data channels are stored as seperate blocks
					d.Data = make([]byte, size*3*2)
					for ch := 0; ch < 3; ch++ {
						for i := 0; i < size; i++ {
							d.Data[ch*size+i] = img2.Pix[ch+i*4]
						}
					}
					for ch := 0; ch < 3; ch++ {
						for i := 0; i < size; i++ {
							d.Data[(3+ch)*size+i] = dst2.Pix[ch+i*4]
						}
					}

					var r jobResult
					r.key = j.key
					r.value, err = proto.Marshal(d)

					for i := range r.meanValues {
						r.meanValues[i] = meanInt(d.Data[(size)*i : (size)*(i+1)])
					}

					if err == nil {
						// this will lock if result channel is full
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
		}(i)
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
				// if there is room
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
		err = newDB.Put([]byte(k), r.value)
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
		if wCount >= max || InterruptRequested {
			break
		}
	}
	grLog("\nDB writer finished")
	grCloseDB(newDB)

	noMoreRandom <- 1 // close the randomImages producer
	for i := 0; i < config.Workers; i++ {
		grLog("Worker go home!")
		noMoreWork <- 1 // tell the workers to go home
	}

	// flush result channel
	for i := 0; i < config.Workers; i++ {
		if len(results) > 0 {
			<-results
		}
	}

	stop := time.Since(start)
	fmt.Printf("\nDone in %.4v\n", stop)

	grLogsf(logFile, "Operation stats:\ncrops: %v\nblur: %v\nsharpness: %v\ncontrast: %v\ngamma: %v\nbrightness: %v\nnoop: %v\n", cr, bl, sh, co, ga, br, no)

	// print out mean values
	grLogsf(logFile, "\nImage mean values:\n")
	for i := 0; i < 3; i++ {
		grLogsf(logFile, "Channel %v: %f\n", i, meanSums[i]/float64(wCount*2))
	}
}

func crop(img image.Image) image.Image {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	pc := float64(config.Generate.PercentCropping) / 100
	nw := int(pc*float64(w) + rand.Float64()*(1-pc)*float64(w))
	nh := int(pc*float64(h) + rand.Float64()*(1-pc)*float64(h))
	x := rand.Intn(w - nw)
	y := rand.Intn(h - nh)
	r := image.Rect(x, y, x+nw, y+nh)
	return imaging.Crop(img, r)
}
