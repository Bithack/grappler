package main

import (
	"fmt"
	"strconv"
	"sync"
	"teorem/anydb"
	"teorem/grappler/caffe"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/gonum/floats"
	"github.com/gonum/matrix/mat64"
)

func computeImageMean(db *anydb.ADB) {

	start := time.Now()

	type imageJob struct {
		data []byte
	}
	images := make(chan imageJob, 100)
	max := db.Entries()

	db.Scan()
	db.Reset()

	// read first image to get channel info
	j := db.Value()
	d := &caffe.Datum{}
	err := proto.Unmarshal(j, d)
	if err != nil {
		return
	}
	channels := int(d.GetChannels())
	width := int(d.GetWidth())
	height := int(d.GetHeight())
	size := width * height

	// LOADER
	go func() {
		var c int
		for {
			var j imageJob
			j.data = db.Value()
			images <- j
			c++
			if c%10 == 0 {
				fmt.Printf("\r[%v:%v] (images: %v) Working...", c, max, len(images))
			}
			if !db.Next() {
				break
			}
		}
		if c%10 != 0 {
			fmt.Printf("\r[%v:%v] (images: %v) Working...", c, max, len(images))
		}
		close(images)
	}()

	// every worker writes to her own channels sums and counters
	matrixSums := make([]*mat64.Dense, config.Workers*channels)
	for i := range matrixSums {
		matrixSums[i] = mat64.NewDense(height, width, nil)
	}

	wCount := make([]int, config.Workers)
	var wg sync.WaitGroup
	for w := 0; w < config.Workers; w++ {
		wg.Add(1)
		grLog("Worker started")
		go func(w int) {
			for {
				j, more := <-images
				if !more {
					wg.Done()
					return
				}
				d := &caffe.Datum{}
				err := proto.Unmarshal(j.data, d)
				if err != nil {
					fmt.Printf("\nUnmarshaling error: %v\n", err)
					return
				}
				data := d.GetData()
				floatData := d.GetFloatData()
				var convData []float64
				if len(data) == 0 && len(floatData) > 0 {
					convData = make([]float64, len(floatData))
					for j := range floatData {
						convData[j] = float64(floatData[j])
					}
				} else {
					convData = make([]float64, len(data))
					for j := range data {
						convData[j] = float64(data[j])
					}
				}

				for i := 0; i < channels; i++ {
					b := mat64.NewDense(height, width, convData[size*i:size*(i+1)])
					matrixSums[w*channels+i].Add(matrixSums[w*channels+i], b)
					//meanSums[w*channels+i] += meanInt(data[(size)*i : (size)*(i+1)])
				}
				wCount[w]++
			}
		}(w)
	}
	wg.Wait()

	stop := time.Since(start)
	fmt.Printf("\nDone in %v\n", stop)

	// sum up the workers matrixes
	finalSums := make([]*mat64.Dense, channels)
	for i := range finalSums {
		finalSums[i] = mat64.NewDense(height, width, nil)
		for w := 0; w < config.Workers; w++ {
			finalSums[i].Add(finalSums[i], matrixSums[w*channels+i])
		}
	}

	wCountTotal := 0
	for w := range wCount {
		wCountTotal += wCount[w]
	}

	// calc and print out mean values
	fmt.Printf("Image mean values:\n")
	means := make([]float64, channels)
	for i := range finalSums {
		finalSums[i].Scale(1/float64(wCountTotal), finalSums[i])
		matrixes["channelMean"+strconv.Itoa(i)] = finalSums[i]
		means[i] = floats.Sum(finalSums[i].RawMatrix().Data) / float64(size)
		fmt.Printf("Channel %v: %f\n", i, means[i])
	}
	// in half
	if channels%2 == 0 {
		fmt.Printf("Mean of means:\n")
		for i := 0; i < channels/2; i++ {
			fmt.Printf("Channel %v, %v: %f\n", i, i+channels/2, (means[i]+means[i+channels/2])/2)
		}
	}

}
