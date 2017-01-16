package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"teorem/grappler/caffe"
	"teorem/multimatrix/matchar"
	"teorem/tinyprompt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/gonum/matrix/mat64"
)

func eval(text string) bool {

	parts := strings.Split(text, " ")

switcher:
	switch parts[0] {

	case "history":
		tinyprompt.PrintHistory()

	case "put":
		if len(parts) < 2 {
			fmt.Printf("Usage: put key, values [into namespace.set]\n")
			break
		}

	case "get":
		if len(parts) != 2 {
			fmt.Printf("Usage: get key\n")
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
		lastKey, lastValue, err = selectedDBs[0].Get([]byte(parts[1]))
		if err != nil {
			fmt.Printf("Failed\n")
			break
		}

		//fmt.Printf("Raw: %v\n", data)

		d := &caffe.Datum{}
		err = proto.Unmarshal(lastValue, d)
		if err != nil {
			log.Fatal("unmarshaling error: ", err)
			break
		}
		fmt.Printf("Channels: %v, Height: %v, Width: %v\n", d.GetChannels(), d.GetHeight(), d.GetWidth())
		fmt.Printf("Labels: %v\n", d.GetLabel())

		d1 := d.GetData()
		lastFloats = d.GetFloatData()

		fmt.Printf("Data: %v bytes\nFloatData: %v float32 (%v bytes)\n", len(d1), len(lastFloats), len(lastFloats)*4)
		fmt.Printf("Unrecognized: %v bytes\n", len(d.XXX_unrecognized))

	case "size", "info":
		size := myDB.SizeOf([]byte("00000000"), []byte("99999999"))
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
				value := selectedDBs[0].Value()
				d := &caffe.Datum{}
				err := proto.Unmarshal(value, d)
				if err != nil {
					fmt.Printf("unmarshaling error\n")
					break
				}
				floats := d.GetFloatData()
				f64 := make([]float64, len(floats))
				for i, v := range floats {
					f64[i] = float64(v)
				}
				if !destReady {
					matrixes[mat] = matrixes[mat].Grow(int(max), len(floats)).(*mat64.Dense)
					destReady = true
				}
				matrixes[mat].SetRow(int(count), f64)
			}

			count++
			if count%5000 == 0 {
				fmt.Printf("%v records loaded\n", count)
			}
			if count >= max {
				break load_loop
			}
			if !selectedDBs[0].Next() {
				break load_loop
			}
		}
		if count%5000 != 0 {
			fmt.Printf("%v records loaded\n", count)
		}
		if parts[1] == "floats" {
			printMatrix(mat)
		} else {
			printCharMatrix(mat)
		}

		selectedDBs[0].Reset()

	case "create", "make":
		if len(parts) != 3 {
			fmt.Printf("usage: create <object> <name>\n")
			break
		}
		switch parts[1] {
		case "matrix":
			a := mat64.NewDense(0, 0, nil)
			matrixes[parts[2]] = a
			printMatrix(parts[2])

		default:
			fmt.Printf("unknown object %s\n", parts[1])
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

	case "limit":
		if len(parts) != 2 {
			fmt.Printf("usage: limit <number>\n")
			break
		}
		l, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Printf("malformed number\n")
		}
		limit = uint64(l)

	case "dump":
		//dump data to screen or file from one or several dbs

	case "set":
		if len(parts) != 3 {
			fmt.Printf("usage: set option value\n")
			break
		}
		switch parts[1] {

		//debug on/off
		case "debug":
			if parts[2] == "on" {
				debugMode = true
			} else {
				debugMode = false
			}

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

	case "test":
		tests := []string{"(3+(3+3))",
			"max(mean(random(10,10))')",
			"pca(random(100,100),20)'",
			"((((4.0))))",
			"random(3,3)+(random(3,3)-random(3,3))",
			"bh_tsne(pca(random(500,200),100),3,0.5,50)",
			"test' * test",
			"rand(3,3) .* test .* rand(3,3)"}
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

	case "stat":
		for _, db := range selectedDBs {
			db.Stat()
		}

	case "q", "quit", "exit":
		return false

	case "?", "help":
		if len(parts) == 1 {
			fmt.Printf("open close db dbs use quit list get reload\n")
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
		//check vars and that the row dimensions match
		vars := strings.Split(parts[1], ",")
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
		break

	case "close":
		for _, db := range selectedDBs {
			db.Close()
		}
		selectedDBs = selectedDBs[:0]

	case "show":
		if len(parts) > 2 {
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
		dbPath = parts[1]
		open(parts[1])

	case "":

	case "w", "who", "whos":
		if len(matrixes) == 0 && len(matrixesChar) == 0 {
			fmt.Printf("No variables yet\n")
			break
		}
		for m := range matrixes {
			r, c := matrixes[m].Dims()
			fmt.Printf("%s"+strings.Repeat(" ", 10-len(m))+"Float64      Dims(%v, %v)\n", m, r, c)
		}
		for m := range tempMatrixes {
			r, c := tempMatrixes[m].Dims()
			fmt.Printf("%s"+strings.Repeat(" ", 10-len(m))+"Float64      Dims(%v, %v)\n", m, r, c)
		}
		for m := range matrixesChar {
			r, c := matrixesChar[m].Dims()
			fmt.Printf("%s"+strings.Repeat(" ", 10-len(m))+"Char         Dims(%v, %v)\n", m, r, c)
		}

	default:

		//remove all spaces from string
		text = strings.Replace(text, " ", "", -1)
		//check here for assignment
		t := strings.Split(text, "=")
		re := regexp.MustCompile("^[a-zA-Z0-9]+$")
		if len(t) == 2 {
			if re.Match([]byte(t[0])) {
				r, err := parseExpression(t[1])
				if err == nil {
					matrixes[t[0]] = r
					printMatrix(t[0])
					break
				}
			} else {
				fmt.Printf("Only letters and digits allowed in variable names\n")
				break
			}
		}

		//parseExpression only works with float64 matrixes
		start := time.Now()
		ans, err := parseExpression(text)
		if err == nil {
			stop := time.Since(start)
			if stop.Seconds() > 1 {
				fmt.Printf("Time elapsed: %.4vs\n", stop)
			}
			matrixes["ans"] = ans
			printMatrix("ans")
		} else {
			//check if its a char matrix
			_, ok := matrixesChar[text]
			if ok {
				printCharMatrix(text)
			} else {
				fmt.Printf("%v\n", err)
			}
		}
		//release memory from temporary matrixes
		clearParser()
	}

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
	return 1

}
