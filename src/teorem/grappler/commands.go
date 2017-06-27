package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"teorem/grappler/caffe"
	"teorem/grappler/vars"
	"teorem/matlab"
	"teorem/multimatrix/matchar"
	"teorem/tinyprompt"
	"time"

	"image"

	"github.com/disintegration/imaging"
	"github.com/golang/protobuf/proto"
	"github.com/gonum/matrix/mat64"
)

var monitorStarted bool

func eval(text string) (status bool) {

	status = true
	parts := strings.Split(strings.ToLower(strings.Trim(text, " ")), " ")

switcher:
	switch parts[0] {

	case "history":
		tinyprompt.PrintHistory()

	case "put":
		if len(parts) < 2 {
			fmt.Printf("Usage: put <keys>,<values> [into <namespace>.<set>]\n")
			break
		}
		if len(selectedDBs) != 1 {
			fmt.Printf("Open and select ONE db first\n")
			break
		}
		v := strings.Split(parts[1], ",")
		if len(v) != 2 {
			fmt.Printf("Expected key,value as second parameter\n")
			break
		}
		keys, ok := matrixesChar[v[0]]
		if !ok {
			fmt.Printf("No such variable %s\n", v[0])
			break
		}
		values, ok := matrixes[v[1]]
		if !ok {
			fmt.Printf("No such variable %s\n", v[1])
			break
		}
		var namespace, set string
		if len(parts) > 2 && parts[2] == "into" {
			nss := strings.Split(parts[3], ".")
			namespace = nss[0]
			set = nss[1]
		} else {
			namespace = "test"
			set = "geohashes"
		}
		//Aerospike GEOPoint hack!
		r, c := values.Dims()
		r2, _ := keys.Dims()
		if r != r2 {
			fmt.Printf("Row count of %v and %v doesnt match\n", v[0], v[1])
			break
		}
		if c == 2 && selectedDBs[0].Identity() == "aerospike" {
			for i := 0; i < r; i++ {
				jsonPoint := `{ "type": "Point", "coordinates": [` + strconv.FormatFloat(values.At(i, 0), 'f', -1, 64) + "," + strconv.FormatFloat(values.At(i, 1), 'f', -1, 64) + `] }`
				//fmt.Printf("%s\n", jsonPoint)
				err := selectedDBs[0].PutGeoJSON(namespace, set, "point", []byte(keys.RowView(i)), jsonPoint)
				if err != nil {
					fmt.Printf("Aerospike error: %v", err)
				}
				if (i+1)%10 == 0 {
					fmt.Printf("\r[%v:%v] Writing records...", i+1, r)
				}
			}
			fmt.Printf("\r[%v:%v] Writing records... Done\n", r, r)
		} else {
			fmt.Printf("Not supported yet\n")
		}

	case "search":
		if len(parts) < 8 {
			fmt.Printf("Usage: search <namespace>.<set> where <bin> within <meters> from <lat>,<lng>\n")
			break
		}
		if len(selectedDBs) != 1 || selectedDBs[0].Identity() != "aerospike" {
			fmt.Printf("Open and select ONE aerospike db first\n")
			break
		}
		var namespace, set string
		nss := strings.Split(parts[1], ".")
		namespace = nss[0]
		set = nss[1]
		bin := parts[3]
		radius, err := strconv.ParseFloat(parts[5], 64)
		if err != nil {
			fmt.Printf("Error parsing distance\n")
			break
		}
		latlng := strings.Split(parts[7], ",")
		lat, err := strconv.ParseFloat(latlng[0], 64)
		if err != nil {
			fmt.Printf("Error parsing latitude\n")
			break
		}
		lng, err := strconv.ParseFloat(latlng[1], 64)
		if err != nil {
			fmt.Printf("Error parsing longitude\n")
			break
		}
		result, err := selectedDBs[0].RadiusSearch(namespace, set, bin, lat, lng, radius)
		if err != nil {
			fmt.Printf("%v\n", err)
			break
		}
		count := 0
		for res := range result.Results() {
			if res.Err != nil {
				fmt.Printf("%v\n", res.Err)
			}
			fmt.Printf("%v\n", res.Record)
			count++
		}
		fmt.Printf("Found %v records\n", count)

		//save session variables to MATLAB 5.0 file, not implemented yet
	case "save":
		if len(parts) > 2 {
			fmt.Printf("Usage: save [<filename>]\n")
			break
		}
		filename := "session.mat"
		if len(parts) == 2 {
			filename = parts[1]
		}
		matlab.WriteMatlabFile(filename, nil)

	case "config":
		fmt.Printf("%+v\n", config)

	case "next":
		for i := range selectedDBs {
			selectedDBs[i].Next()
		}
		eval("get last")

	case "get":
		if len(parts) < 2 {
			fmt.Printf("Usage: get key [from <namespace>.<set>] [as <object>]\n")
			break
		}
		if len(selectedDBs) == 0 {
			fmt.Printf("Open a db first\n")
			break
		}
		if len(selectedDBs) > 1 {
			fmt.Printf("only supports retrieving from one db, select one with \"use\"\n")
			break
		}

		var err error

		// SPECIAL CASE FOR AEROSPIKE DB
		if len(parts) == 4 && parts[2] == "from" {
			if selectedDBs[0].Identity() != "aerospike" {
				fmt.Printf("Syntax only available for aerospike dbs\n")
				break
			}
			//assume aerospike
			nss := strings.Split(parts[3], ".")
			selectedDBs[0].SetContext(nss[0], nss[1])
			record, err := selectedDBs[0].GetRecord([]byte(parts[1]))
			if err != nil {
				fmt.Printf("Didn't work\n")
				break
			}
			fmt.Printf("%+v\n", record)
			break
		}

		switch parts[1] {
		case "random":
			lastKey, lastValue, err = selectedDBs[0].GetRandom()
		default:
			lastKey, lastValue, err = selectedDBs[0].Get([]byte(parts[1]))
		}
		if err != nil {
			fmt.Printf("GET failed: %v\n", err)
			break
		}

		if len(parts) == 4 && parts[2] == "as" {
			switch parts[3] {
			case "image":
				i, err := imaging.Decode(bytes.NewReader(lastValue))
				if err != nil {
					fmt.Printf("Failed: %v", err)
					return
				}
				// try it as a NRGBA
				img, ok := i.(*image.NRGBA)
				if !ok {
					fmt.Printf("Not a NRGBA\n")
					return
				}
				bounds := img.Bounds()
				w := bounds.Dx()
				h := bounds.Dy()
				s := w * h
				fmt.Printf("Image: %v\n", bounds)
				fmt.Printf("4 channels, %v bytes per channel\n", len(img.Pix)/4)
				rm := meanInt(img.Pix[0*s : s])
				gm := meanInt(img.Pix[1*s : 2*s])
				bm := meanInt(img.Pix[2*s : 3*s])
				fmt.Printf("Mean values: %.4f, %.4f, %.4f\n", rm, gm, bm)
				return
			}
		}

		if selectedDBs[0].Identity() == "lmdb" || selectedDBs[0].Identity() == "leveldb" {
			d := &caffe.Datum{}
			err = proto.Unmarshal(lastValue, d)
			if err != nil {
				log.Fatal("unmarshaling error: ", err)
				break
			}
			fmt.Printf("Channels: %v, Height: %v, Width: %v\n", d.GetChannels(), d.GetHeight(), d.GetWidth())
			fmt.Printf("Labels: %v\n", d.GetLabel())
			d1 := d.GetData()
			if len(d1) > 0 {
				floatData := make([]float64, len(d1))
				for j := range d1 {
					floatData[j] = float64(d1[j])
				}
				matrixes["Data"] = mat64.NewDense(int(d.GetHeight()*d.GetChannels()), int(d.GetWidth()), floatData)
			}
			d2 := d.GetFloatData()
			if len(d2) > 0 {
				floatData := make([]float64, len(d2))
				for j := range d2 {
					floatData[j] = float64(d2[j])
				}
				matrixes["FloatData"] = mat64.NewDense(int(d.GetHeight()*d.GetChannels()), int(d.GetWidth()), floatData)
			}
			fmt.Printf("Data: %v bytes\nFloatData: %v float32 (%v bytes)\n", len(d1), len(lastFloats), len(lastFloats)*4)
			fmt.Printf("Unrecognized: %v bytes\n", len(d.XXX_unrecognized))
		} else {
			fmt.Printf("%+v\n", lastValue)
		}

	case "clear":
		for m := range matrixes {
			matrixes[m] = nil
			delete(matrixes, m)
		}

	case "memory":
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		fmt.Printf("%+v\n", stats)

	case "size", "info":
		size := selectedDBs[0].SizeOf([]byte("00000000"), []byte("99999999"))
		mb := size / (1024 * 1024)
		fmt.Printf("Approximate size whole db: %v Mb\n", mb)

	case "load":
		if len(parts) != 4 && len(parts) != 2 {
			fmt.Printf("usage: load <field> [as <object>] \n")
			break
		}
		if len(selectedDBs) == 0 {
			fmt.Printf("Open a db first\n")
			break
		}
		if len(selectedDBs) > 1 {
			fmt.Printf("currently only supports loading from one db, select one with \"use\"\n")
			break
		}

		var mat string
		if len(parts) == 2 {
			mat = parts[1]
		} else {
			mat = parts[3]
		}

		//check if object exists, if not create it
		switch parts[1] {
		case "keys":
			_, ok := matrixesChar[mat]
			if !ok {
				a := matchar.NewMatchar(nil)
				matrixesChar[mat] = a
			} else {
				matrixesChar[mat].Reset()
			}
		case "floats", "floatdata":
			_, ok := matrixes[mat]
			if !ok {
				a := mat64.NewDense(0, 0, nil)
				matrixes[mat] = a
			} else {
				matrixes[mat].Reset()
			}
		}
		destReady := false

		selectedDBs[0].Reset() //reset cursor to start

		var count uint64
		var max uint64
		if limit != 0 {
			max = limit
		} else {
			max = selectedDBs[0].Entries()
		}
		count = 0

	load_loop:
		for {
			switch parts[1] {
			case "keys":
				key := selectedDBs[0].Key()
				matrixesChar[mat].Append(string(key))

			case "floats", "floatdata":

				/*	for i := 1; i < len(row); i++ {
					f, err := strconv.ParseFloat(row[i], 64)
					if err != nil {
						f = 0
					}
					db.fileFloats = append(db.fileFloats, f)*/

				var f64 []float64
				var err error
				if selectedDBs[0].Identity() == "lmdb" {
					value := selectedDBs[0].Value()
					d := &caffe.Datum{}
					err = proto.Unmarshal(value, d)
					if err != nil {
						fmt.Printf("unmarshaling error\n")
						break
					}
					floats := d.GetFloatData()
					f64 = make([]float64, len(floats))
					for i, v := range floats {
						f64[i] = float64(v)
					}
				}
				if selectedDBs[0].Identity() == "file" {
					// space seperated list with floats
					value := string(selectedDBs[0].Value())
					floats := strings.Split(value, " ")
					f64 = make([]float64, len(floats))
					for i := range floats {
						f64[i], err = strconv.ParseFloat(floats[i], 64)
					}
				}

				if !destReady {
					matrixes[mat] = matrixes[mat].Grow(int(max), len(f64)).(*mat64.Dense)
					destReady = true
				}
				matrixes[mat].SetRow(int(count), f64)
			}

			count++
			if count%10 == 0 {
				fmt.Printf("\r[%v:%v] Loading records...", count, max)
			}
			if count >= max {
				break load_loop
			}
			if !selectedDBs[0].Next() {
				break load_loop
			}
		}
		fmt.Printf("\r[%v:%v] Loading records... Done\n", count, max)

		if parts[1] == "floats" {
			printMatrix(mat)
		} else if parts[1] == "keys" {
			printCharMatrix(mat)
		}

		selectedDBs[0].Reset()

	case "generate":
		if len(parts) < 5 || parts[1] != "siamese" || parts[2] != "dataset" {
			fmt.Printf("Usage:\nGENERATE SIAMESE DATASET <db> (<width>,<height>) [with operation,operation,...]\n")
			fmt.Printf("Available operations: brightness, contrast, gamma, blur, sharpness, crop\n")
			fmt.Printf("Default: %v\n", config.Generate.DefaultOperations)
			return
		}
		if len(selectedDBs) != 1 {
			fmt.Printf("Open and select ONE db first\n")
			return
		}
		destSize := strings.Split(strings.Trim(parts[4], ")("), ",")
		if len(destSize) != 2 {
			fmt.Printf("Mistyped image dimensions\n")
			return
		}
		destW, err := strconv.Atoi(destSize[0])
		if err != nil {
			fmt.Printf("Malformed integer\n")
			return
		}
		destH, err := strconv.Atoi(destSize[1])
		if err != nil {
			fmt.Printf("Malformed integer\n")
			return
		}
		todo := config.Generate.DefaultOperations
		if len(parts) == 7 && parts[5] == "with" {
			todo = strings.Split(parts[6], ",")
		}
		generateSiameseDataset(parts[3], destW, destH, todo)

	case "compute":
		if len(parts) != 2 {
			fmt.Printf("usage: compute mean\n")
			break
		}
		if len(selectedDBs) != 1 {
			fmt.Printf("select ONE db\n")
			break
		}
		computeImageMean(selectedDBs[0])

	case "browse":
		if len(parts) != 1 {
			fmt.Printf("usage: browse\n")
			break
		}
		if len(selectedDBs) != 1 {
			fmt.Printf("Select one db first\n")
			break
		}
		launchBrowser(selectedDBs[0])

	case "caffe":
		if len(parts) < 2 {
			fmt.Printf("usage: caffe monitor\n")
			break
		}
		switch parts[1] {

		case "monitor":
			if monitorStarted {
				fmt.Printf("Monitor already started\n")
				return
			}
			info, err := os.Stat("/tmp/caffe.INFO")
			if err != nil || info.IsDir() {
				fmt.Printf("No running caffe jobs found\n")
				return
			}
			monitorStarted = true
			fmt.Printf("Found running caffe job. Launching monitor at http://localhost:5000\n")
			http.HandleFunc("/", caffeMonitor)
			http.HandleFunc("/graph", caffeGraph)
			go http.ListenAndServe(":5000", nil)

		}

	case "start", "seek":
		if len(parts) != 2 {
			fmt.Printf("usage: start <string>\n")
			break
		}
		for _, db := range selectedDBs {
			db.Seek([]byte(parts[1]))
		}

	case "end":
		if len(parts[1]) != 2 {
			fmt.Printf("usage: end <string>\n")
			break
		}

	case "set":
		if len(parts) < 2 || len(parts) > 3 {
			fmt.Printf("usage: set option [value]\n")
			break
		}
		if len(parts) == 2 {
			switch parts[1] {
			case "workers":
				fmt.Printf("%v\n", config.Workers)
			case "width":
				fmt.Printf("%v\n", maxPrintWidth)
			case "debug":
				fmt.Printf("%v\n", debugMode)
			case "limit":
				fmt.Printf("%v\n", limit)
			}
			break
		}
		switch parts[1] {
		case "workers":
			config.Workers, _ = strconv.Atoi(parts[2])
		// options for printing matrixes
		case "width":
			maxPrintWidth, _ = strconv.Atoi(parts[2])
		//debug on/off
		case "debug":
			if parts[2] == "on" {
				debugMode = true
			} else {
				debugMode = false
			}
		case "limit":
			l, err := strconv.Atoi(parts[2])
			if err != nil {
				fmt.Printf("malformed number\n")
			}
			limit = uint64(l)
		//adds a filter to the key before it is printed
		case "filter":
			//expect [i:j], [i:], [:j]
			parts[2] = strings.TrimPrefix(parts[2], "[")
			parts[2] = strings.TrimSuffix(parts[2], "]")
			ij := strings.Split(parts[2], ":")
			i, err := strconv.Atoi(ij[0])
			if err != nil {
				i = 0
			}
			j, err := strconv.Atoi(ij[1])
			if err != nil {
				j = 0
			}
			for _, db := range selectedDBs {
				db.SetFilterRange(i, j)
			}
		default:
			fmt.Printf("No such option\n")
		}

	case "_test":
		tests := []string{"(3+(3+3))",
			"max(mean(random(10,10))')",
			"pca(random(100,100),20)'",
			"((((4.0))))",
			"random(3,3)+(random(3,3)-random(3,3))",
			"bh_tsne(pca(random(500,200),100),3,0.5,50)",
			"test' * test",
			"rand(3,3) .* test .* rand(3,3)",
			"test(:,:)",
			"[1 2; 3 4; 5 6]"}
		passed := 0
		for i := range tests {
			passed = passed + doTest(tests[i])
		}
		fmt.Printf("TEST REPORT\n%v passed\n%v failed\n", passed, len(tests)-passed)

	case "reset":
		//reset iterator for all selected dbs
		for _, db := range selectedDBs {
			db.Reset()
		}

	case "l", "ls", "list":
		// list keys from one or several dbs
		// finishes when max is reached, or any of the dbs reaches its end
		if len(selectedDBs) == 0 {
			fmt.Printf("Open a db first\n")
			break
		}

		var max int
		if len(parts) == 2 {
			max, _ = strconv.Atoi(parts[1])
		} else {
			max = 32
		}

		count := 0

	list_loop:
		for {

			for _, db := range selectedDBs {
				key := db.Key()
				value := db.Value()
				fmt.Printf("%s (%v bytes)        ", key, len(value))
				if !db.Next() {
					break list_loop
				}
			}
			fmt.Printf("\n")
			count++
			if count >= max {
				break
			}
		}
		fmt.Printf("\n")

	case "stat":
		for _, db := range selectedDBs {
			db.Stat()
		}

	case "q", "quit", "exit":
		return false

	case "?", "help":
		if len(parts) == 2 {
			fmt.Printf("%v\n", parseGetHelp(parts[1]))
		}
		if len(parts) == 1 {
			fmt.Printf("COMMANDS\n")
			fmt.Printf("\n")
			fmt.Printf("  DATABASES\n")
			fmt.Printf("    OPEN /path/to/lmdb | /path/to/image-folder | <filename> | aerospike:<server>\n")
			fmt.Printf("    CLOSE\n")
			fmt.Printf("    DBS\n")
			fmt.Printf("    USE <id>[,<id>]\n")
			fmt.Printf("\n")
			fmt.Printf("  ITERATOR\n")
			fmt.Printf("    RESET\n")
			fmt.Printf("    LS [n]\n")
			fmt.Printf("\n")
			fmt.Printf("  OPTIONS\n")
			fmt.Printf("    SET start n\n")
			fmt.Printf("    SET limit n\n")
			fmt.Printf("    SET filter [i]:[j]\n")
			fmt.Printf("    SET filter [i]:[j]\n")
			fmt.Printf("\n")
			fmt.Printf("  READ/WRITE\n")
			fmt.Printf("    GET <key> [from <namespace>.<set>]\n")
			fmt.Printf("    LOAD <field> [as <variable>]\n")
			fmt.Printf("    WRITE <variable>[,variable] to <filename>\n")
			fmt.Printf("    PUT <keys>,<values> [into <namespace>.<set>]\n")
			fmt.Printf("\n")
			fmt.Printf("  IMAGE OPERATIONS\n")
			fmt.Printf("    GENERATE SIAMESE DATASET <db> with none | cropping[,brightness][,sharpness][,blur]\n")
			fmt.Printf("\n")
			fmt.Printf("  INFO\n")
			fmt.Printf("    WHO\n")
			fmt.Printf("    SHOW namespaces | sets | bins | namespace/<namespace>\n")
			fmt.Printf("\n")
			fmt.Printf("  MATH\n")
			fmt.Printf("    Functions: rand, ones, zeros, max, min, mean, size, pca, var, bh_tsne, hist, svg, normr, sort\n")
			fmt.Printf("    For details write \"HELP function\"\n")
			fmt.Printf("    Operators: A', A + B, A - B, A * B, A .* B, A / B, A ./ B, a:b, a:b:c\n")
			fmt.Printf("\n")
		}

	case "write":

		if len(parts) < 4 {
			fmt.Printf("usage: write <variable>[,<variable>] to <filename>\n")
			break
		}

		f, err := os.Create(parts[3])
		if err != nil {
			fmt.Printf("couldn't create file %s\n", parts[3])
			break
		}

		parts := strings.Split(strings.Trim(text, " "), " ")
		vars := strings.Split(parts[1], ",")
		v, ok := variables[vars[0]]

		// check if first variable is a Message (only one seems useful right now)
		if ok && len(vars) == 1 && v.IsMessage() {
			f.WriteString(v.Message.MarshalText())
			return
		}

		// check vars and that the row dimensions match
		var lastr = 0
		for _, v := range vars {
			var matFloat *mat64.Dense
			var matChar *matchar.Matchar
			// is it float64 or char?
			matFloat, isfloat := matrixes[v]
			matChar, ischar := matrixesChar[v]
			if !(isfloat || ischar) {
				fmt.Printf("no such variable: %s\n", v)
				break switcher
			}
			var r int
			if isfloat {
				r, _ = matFloat.Dims()
			} else {
				r, _ = matChar.Dims()
			}
			if lastr != 0 && lastr != r {
				fmt.Printf("matrix dimensions doesn't match\n")
				break switcher
			}
			lastr = r
		}

		//write loop
		for i := 0; i < lastr; i++ {
			first := true
			for _, v := range vars {
				if !first {
					f.WriteString(" ")
				}
				matFloat, isfloat := matrixes[v]
				matChar, ischar := matrixesChar[v]
				if isfloat {
					floats := matFloat.RawRowView(i)
					s := fmt.Sprintf("%v", floats)
					f.WriteString(s[1 : len(s)-2])
				}
				if ischar {
					f.WriteString(fmt.Sprintf("%s", matChar.RowView(i)))
				}
				first = false
			}
			f.WriteString("\n")
		}
		f.Close()
		fmt.Printf("%v records written\n", lastr)

	case "db", "dbs", "use":

		if len(parts) > 2 {
			fmt.Printf("usage: use id\n")
			break
		}
		if len(parts) == 2 {
			if parts[1] == "all" {
				selectedDBs = allDBs
			} else {
				ids := strings.Split(parts[1], ",")
				selectedDBs = selectedDBs[:0]
				for _, id := range ids {
					i, err := strconv.Atoi(strings.TrimSpace(id))
					if err != nil {
						fmt.Printf("malformed id\n")
						break
					}
					if i > len(allDBs)-1 {
						fmt.Printf("no such id\n")
						break
					}
					selectedDBs = append(selectedDBs, allDBs[i])
				}
			}
		}
		for i := 0; i < len(allDBs); i++ {
			if contains(selectedDBs, allDBs[i]) {
				fmt.Printf("--> ")
			} else {
				fmt.Printf("    ")
			}
			fmt.Printf("[%v] %s: %s\n", i, allDBs[i].Identity(), allDBs[i].Path())
		}
		if debugMode {
			for i := range selectedDBs {
				fmt.Printf("%+v\n", selectedDBs[i])
			}
		}
		break

	case "pwd":
		dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
		if err == nil {
			fmt.Printf("%v\n", dir)
		}

	case "close":
		for _, db := range selectedDBs {
			for i := range allDBs {
				if db == allDBs[i] {
					allDBs = append(allDBs[:i], allDBs[i+1:]...)
					break
				}
			}
			db.Close()
		}
		selectedDBs = selectedDBs[:0]

	case "show":
		if len(parts) != 2 {
			fmt.Printf("usage: show parameter\n")
			break
		}
		if len(selectedDBs) == 0 || len(selectedDBs) > 1 || selectedDBs[0].Identity() != "aerospike" {
			fmt.Printf("Select an aerospike database first\n")
			break
		}
		infomap, err := selectedDBs[0].Show(parts[1])
		if err != nil {
			break
		}
		for _, v := range infomap {
			v2 := strings.Split(v, ";")
			for _, u := range v2 {
				if len(u) > 0 {
					fmt.Printf("%s\n", u)
				}
			}

		}

	case "cat", "type":
		if len(parts) != 2 {
			fmt.Printf("usage: cat <filename>\n")
			break
		}
		b, err := ioutil.ReadFile(parts[1])
		if err != nil {
			fmt.Printf("Couldn't open file: %v\n", err)
		}
		fmt.Printf(string(b) + "\n")

	case "open":
		if len(parts) != 2 {
			fmt.Printf("usage: open path/to/db\n")
			break
		}
		// parts have been converted to lowercase, reparse it before trying to open it
		parts := strings.Split(strings.Trim(text, " "), " ")
		dbPath = parts[1]
		open(parts[1])

	case "create":
		if len(parts) != 3 {
			fmt.Printf("usage: create db <type>:<path>\n")
			break
		}
		// parts have been converted to lowercase, reparse it before trying to open it
		parts := strings.Split(strings.Trim(text, " "), " ")
		newDB := strings.Split(parts[2], ":")
		_, err := create(newDB[1], newDB[0])
		if err != nil {
			fmt.Printf("Failed: %v\n", err)
			return
		}

	case "":

	case "who", "whos":
		if len(variables) == 0 && len(matrixes) == 0 && len(matrixesChar) == 0 {
			fmt.Printf("No variables yet\n")
		} else {
			maxLength := 0
			for m := range variables {
				if len(m) > maxLength {
					maxLength = len(m)
				}
			}
			for m := range matrixes {
				if len(m) > maxLength {
					maxLength = len(m)
				}
			}
			for n, v := range variables {
				fmt.Printf("%s"+strings.Repeat(" ", maxLength+1-len(n))+v.Type()+"\n", n)
			}
			for m := range matrixes {
				r, c := matrixes[m].Dims()
				fmt.Printf("%s"+strings.Repeat(" ", maxLength+1-len(m))+"Float64      Dims(%v, %v)\n", m, r, c)
			}
			for m := range matrixesChar {
				r, c := matrixesChar[m].Dims()
				fmt.Printf("%s"+strings.Repeat(" ", maxLength+1-len(m))+"Char         Dims(%v, %v)\n", m, r, c)
			}
		}
		if debugMode {
			fmt.Printf("tempMatrixes:\n")
			for m := range tempMatrixes {
				r, c := tempMatrixes[m].Dims()
				fmt.Printf("%s"+strings.Repeat(" ", 20-len(m))+"Float64      Dims(%v, %v)\n", m, r, c)
			}
			fmt.Printf("returnMatrixes:\n")
			for m := range returnMatrixes {
				r, c := returnMatrixes[m].Dims()
				fmt.Printf("%s"+strings.Repeat(" ", 20-len(m))+"Float64      Dims(%v, %v)\n", m, r, c)

			}
		}

	default:

		// check here for assignment
		// currently skips any returnMatrixes, but we could implement multivariable assignment
		t := strings.Split(strings.Trim(text, " "), "=")
		re := regexp.MustCompile("^[a-zA-Z0-9]+$")
		if len(t) == 2 {
			t[0] = strings.Trim(t[0], " ")
			if re.Match([]byte(t[0])) {
				r, err := parseExpression(t[1])
				if err != nil {
					fmt.Printf("%v\n", err)
					break
				}
				variables[t[0]] = vars.NewFromFloat(r)
				matrixes[t[0]] = r
				printMatrix(t[0])
				break
			} else {
				fmt.Printf("Only letters and digits allowed in variable names\n")
				break
			}
		}

		// check for subfield syntax
		t2 := strings.Split(strings.Trim(text, " "), ".")
		_, ok := variables[t2[0]]
		if ok {
			if len(t2) == 1 {
				variables[t2[0]].Print(text)
			} else {
				//subfield(s) requested
				f := variables[t2[0]]
				for i := 1; i < len(t2); i++ {
					f = f.GetField(t2[i])
					if f == nil {
						fmt.Printf("No such value %v\n", t2[i])
						break
					}
				}
				if f != nil {
					variables["ans"] = f
					f.Print(text)
				}
			}
			break
		}

		//parseExpression only works with float64 matrixes
		start := time.Now()
		ans, err := parseExpression(text)
		if err == nil {
			stop := time.Since(start)
			if stop.Seconds() > 1 {
				fmt.Printf("Time elapsed: %.4vs\n", stop)
			}
			for _, i := range returnMatrixesOrder {
				matrixes[i] = returnMatrixes[i]
				printMatrix(i)
			}
			matrixes["ans"] = ans
			printMatrix("ans")
		} else {
			//check if its a char matrix (currently not handled by parseExpression)
			_, ok := matrixesChar[text]
			if ok {
				printCharMatrix(text)
			} else {
				fmt.Printf("%v\n", err)
			}
		}
	}

	//release memory from temporary matrixes
	clearParser()
	return true
}

func doTest(expr string) int {
	var err error

	matrixes["test"], err = parseExpression(expr)
	if err != nil {
		fmt.Printf("TEST FAILED\n")
		return 0
	}
	printMatrix("test")
	clearParser()
	return 1

}

func newFloat32(f float32) (ref *float32) {
	ref = new(float32)
	*ref = f
	return
}

func newString(s string) (ref *string) {
	ref = new(string)
	*ref = s
	return
}

func meanInt(data []uint8) (v float64) {
	for i := 0; i < len(data); i++ {
		v += float64(data[i])
	}
	v = v / float64(len(data))
	return
}
