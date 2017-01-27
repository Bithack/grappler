package main

import (
	"fmt"
	"sync"
	"teorem/anydb"
	"teorem/grappler/caffe"
	"time"

	"github.com/golang/protobuf/proto"
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
		close(images)
	}()

	// read one image to scan channel info
	j := <-images
	d := &caffe.Datum{}
	err := proto.Unmarshal(j.data, d)
	if err != nil {
		return
	}
	channels := int(d.GetChannels())
	size := int(d.GetWidth() * d.GetHeight())
	// every worker writes to her own sum
	meanSums := make([]float64, channels*config.Workers)
	// put it back again
	images <- j

	var wCount int
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
				for i := 0; i < channels; i++ {
					meanSums[w*channels+i] += meanInt(data[(size)*i : (size)*(i+1)])
				}
				wCount++
			}
		}(w)
	}
	wg.Wait()

	stop := time.Since(start)
	fmt.Printf("\nDone in %v\n", stop)

	// sum it upp
	finalSums := make([]float64, channels)
	for i := range finalSums {
		for w := 0; w < config.Workers; w++ {
			finalSums[i] += meanSums[w*channels+i]
		}
	}

	// print out mean values
	fmt.Printf("Image mean values:\n")
	for i := range finalSums {
		fmt.Printf("Channel %v: %f\n", i, finalSums[i]/float64(wCount))
	}
	// in half
	if len(finalSums)%2 == 0 {
		fmt.Printf("Mean of means:\n")
		for i := 0; i < len(finalSums)/2; i++ {
			fmt.Printf("Channel %v, %v: %f\n", i, i+len(finalSums)/2, (finalSums[i]+finalSums[i+len(finalSums)/2])/(float64(wCount)*2))
		}
	}

}
