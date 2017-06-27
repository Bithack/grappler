package main

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"teorem/anydb"
	"teorem/grappler/caffe"

	"image/color"
	"image/jpeg"

	"github.com/disintegration/imaging"
	"github.com/golang/protobuf/proto"
)

var browseKeys []string
var browseDB *anydb.ADB

func launchBrowser(db *anydb.ADB) {
	fmt.Printf("Launching db browser at http://localhost:5001\n")
	browseDB = db

	http.HandleFunc("/", dbBrowser)
	http.HandleFunc("/image/", dbImage)
	go http.ListenAndServe(":5001", nil)

	max := int(math.Min(500, float64(db.Entries())))
	browseKeys = make([]string, max)
	i := 0
	for {
		browseKeys[i] = string(db.Key())
		if !db.Next() {
			break
		}
		i++
		if i >= max {
			break
		}
	}
	fmt.Printf("%v keys loaded\n", i+1)
}

func dbBrowser(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "text/html")
	wr(res, "Browsing db %v:%v<br>", browseDB.Identity(), browseDB.Path())
	max := len(browseKeys)
	for i := 0; i < max; i++ {
		wr(res, "<div style=\"width:25%%; float:left\">")
		wr(res, "<img style=\"width:100%%;\" src=\"/image/%s\" />", browseKeys[i])
		wr(res, "</div>")
	}
}

func wr(res http.ResponseWriter, s string, args ...interface{}) {
	text := fmt.Sprintf(s, args...)
	res.Write([]byte(text))
}

func dbImage(res http.ResponseWriter, req *http.Request) {
	uris := strings.Split(req.RequestURI, "/")
	key := uris[len(uris)-1]
	_, value, _ := browseDB.Get([]byte(key))
	datum := new(caffe.Datum)
	proto.Unmarshal(value, datum)

	img := imaging.New(int(datum.GetWidth()*datum.GetChannels()/3), int(datum.GetHeight()), color.White)

	if datum.GetChannels() == 6 {
		// image 1
		w := int(datum.GetWidth())
		h := int(datum.GetHeight())
		s := w * h
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(int(x), int(y), color.NRGBA{A: 255, R: uint8(datum.Data[0*s+y*w+x]), G: uint8(datum.Data[1*s+y*w+x]), B: uint8(datum.Data[2*s+y*w+x])})
			}
		}

		// image 2
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x+w, int(y), color.NRGBA{A: 255, R: uint8(datum.Data[3*s+y*w+x]), G: uint8(datum.Data[4*s+y*w+x]), B: uint8(datum.Data[5*s+y*w+x])})
			}
		}
	}

	jpeg.Encode(res, img, nil)
	//wr(res, "%+v", datum)
}
