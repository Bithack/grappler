package main

import (
	"bytes"
	"io/ioutil"
	"reflect"
	"strconv"
	"teorem/anydb"

	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"teorem/grappler/caffe"

	"github.com/golang/protobuf/proto"
	"github.com/gonum/matrix/mat64"
	"github.com/pkg/term"
)

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func contains(list interface{}, elem interface{}) bool {
	v := reflect.ValueOf(list)
	for i := 0; i < v.Len(); i++ {
		if v.Index(i).Interface() == elem {
			return true
		}
	}
	return false
}

func getch() []byte {
	t, _ := term.Open("/dev/tty")
	term.RawMode(t)
	bytes := make([]byte, 4)
	numRead, err := t.Read(bytes)
	t.Restore()
	t.Close()
	if err != nil {
		return nil
	}
	return bytes[0:numRead]
}

type dbInfo struct {
	/*path   string
	key    []byte
	value  []byte
	floats []float32*/
	dumpOption string
}

var allDBs []*anydb.ADB
var selectedDBs []*anydb.ADB
var myDB *anydb.ADB // deprecated, should be selectedDBs[0]
var dbInfos []dbInfo

var limit uint64
var writeFormat string

var dbPath string
var lastKey []byte
var lastValue []byte
var lastFloats []float32

var history []string
var historyScan = -1

var matrixes = make(map[string]*mat64.Dense)

func main() {

	//reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Teorem Data Grappler\nVersion 0.0.9\n")

	if len(os.Args) > 1 {
		for i := 1; i < len(os.Args); i++ {
			dbPath = os.Args[i]
			open(dbPath)
		}
	}

	fmt.Printf("Interactive mode. Type \"help\" for commands.\n")
loop:
	for {
		fmt.Print("> ")

		text := ""
	line_reader:
		for {
			c := getch()
			switch {
			case bytes.Equal(c, []byte{27, 91, 68}): // left
			case bytes.Equal(c, []byte{27, 91, 67}): // right
			case bytes.Equal(c, []byte{27, 91, 65}): // up
				if len(history) > 0 && historyScan > 0 {
					historyScan = historyScan - 1
					text = history[historyScan]
					fmt.Printf("\r> %s", text)
				}

			case bytes.Equal(c, []byte{27, 91, 66}): // down
				if len(history) > 0 && historyScan < len(history)-1 {
					historyScan = historyScan + 1
					text = history[historyScan]
					fmt.Printf("\r> %s", text+strings.Repeat(" ", 20)+strings.Repeat("\b", 20))
				}

			case bytes.Equal(c, []byte{13}): // enter
				fmt.Printf("\n")
				break line_reader

			case bytes.Equal(c, []byte{9}): // tab

			case bytes.Equal(c, []byte{127}): // backspace
				if len(text) > 0 {
					text = text[:len(text)-1]
					//fmt.Printf("\b ")
					fmt.Printf("\r> %v \b", text)
				}
				//fmt.Printf(chr(8) . " ";
				//fmt.Printf("\r> %v", text)

			default:
				//fmt.Printf("Key: %v", c)
				if len(c) == 1 {
					fmt.Printf("%c", c[0])
					text = text + string(c)
				}
			}
		}

		//text, _ = reader.ReadString('\n')
		//text = strings.TrimSuffix(text, "\n")

		history = append(history, text)
		historyScan = len(history) - 1

		parts := strings.Split(text, " ")
		switch parts[0] {

		case "d", "data":

		case "history":
			for _, cmd := range history {
				fmt.Printf("%v\n", cmd)
			}

		case "f", "floats":
			if len(lastFloats) > 0 {
				fmt.Printf("%v", lastFloats)
			}

		case "g", "get":
			if len(parts) != 2 {
				fmt.Printf("Usage: get key\n")
				break
			}
			var err error
			lastKey, lastValue, err = myDB.Get([]byte(parts[1]))
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

		case "reload":
			if myDB != nil {
				myDB.Close()
				open(dbPath)
			}
			break

		case "pca":

		case "load":
			if len(parts) != 4 {
				fmt.Printf("usage: load <field> into <object>\n")
				break
			}
			if len(selectedDBs) > 1 {
				fmt.Printf("currently only supports loading from one db, select one with \"use\"\n")
				break
			}
			//check if object exists
			_, ok := matrixes[parts[3]]
			if !ok {
				fmt.Printf("no such object. Use \"create\" first\n")
			}

			dest := matrixes[parts[3]]
			destReady := false

			selectedDBs[0].Release()
			selectedDBs[0].Scan()
			var count uint64
			var max uint64
			if limit != 0 {
				max = limit
			} else {
				max = selectedDBs[0].Entries()
			}

		load_loop:
			for {
				if !selectedDBs[0].Next() {
					break load_loop
				}

				switch parts[1] {
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
						dest = dest.Grow(int(max), len(floats))
						destReady = true
					}
					i, j := dest.Dims()
					fmt.Printf("Row: %v, %v %v\n", int(count)+1, i, j)
					dest.SetRow(int(count)+1, f64)

				}

				count++
				if count%5000 == 0 {
					fmt.Printf("%v records loaded\n", count)
				}
				if count >= max {
					break load_loop
				}
			}
			if count%5000 != 0 {
				fmt.Printf("%v records loaded\n", count)
			}

		case "create", "make":
			if len(parts) != 3 {
				fmt.Printf("usage: create <object> <name>\n")
				break
			}
			if len(selectedDBs) > 1 {
				fmt.Printf("currently only supports writing from one db\n")
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
			//specifies what should be outputed during next dump
			//key, floats
			case "dump":
				fields := []string{"key", "floats"}
				if !contains(fields, parts[2]) {
					fmt.Printf("unknown parameter\n")
					break
				}

			//sets output format for write
			case "format":
				formats := []string{"ascii", "tsne"}
				if !contains(formats, parts[2]) {
					fmt.Printf("unknown format\n")
					break
				}
				writeFormat = parts[2]

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
					j = -1
				}
				for _, db := range selectedDBs {
					db.SetFilterRange(i, j)
				}
			}

		case "reset":
			//reset iterator for all selected dbs
			for _, db := range selectedDBs {
				db.Release()
				db.Scan()
			}

		case "l", "ls", "list":
			// list keys from one or several dbs
			// finishes when max is reached, or any of the dbs reaches its end
			if myDB == nil {
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
					if !db.Next() {
						break list_loop
					}
					key := db.Key()
					value := db.Value()
					fmt.Printf("%s (%v bytes)        ", key, len(value))
				}
				fmt.Printf("\n")
				count++
				if count >= max {
					break
				}

			}

		case "debug":
			fmt.Printf("%+v\n", myDB)

		case "stat":
			myDB.Stat()

		case "q", "quit", "exit":
			break loop

		case "?", "help":
			if len(parts) == 1 {
				fmt.Printf("open close db dbs use quit list get reload\n")
			}

			//stat.Bhattacharyya
		case "write":
			if len(parts) < 4 {
				fmt.Printf("usage: write <field> to <filename>\n")
				break
			}
			if len(selectedDBs) > 1 {
				fmt.Printf("currently only supports writing from one db\n")
				break
			}

			f, err := os.Create(parts[3])
			if err != nil {
				fmt.Printf("couldn't create file %s\n", parts[3])
				break
			}
			defer f.Close()

			selectedDBs[0].Release()
			selectedDBs[0].Scan()
			count := 0
			var max int
			if limit != 0 {
				max = int(limit)
			} else {
				max = 9999999
			}

		write_loop:
			for {
				for _, db := range selectedDBs {
					if !db.Next() {
						break write_loop
					}

					switch parts[1] {
					case "floats", "floatdata":
						value := db.Value()
						d := &caffe.Datum{}
						err = proto.Unmarshal(value, d)
						if err != nil {
							fmt.Printf("unmarshaling error\n")
							break
						}
						floats := d.GetFloatData()
						s := fmt.Sprintf("%v\n", floats)
						// skip [ and ]
						_, err := f.WriteString(s[1 : len(s)-2])

						if err != nil {
							fmt.Printf("couldn't write string\n")
						}

					case "key":
						key := db.Key()
						_, err = f.WriteString(fmt.Sprintf("%s\n", string(key)))

					}
				}
				count++
				if count%5000 == 0 {
					fmt.Printf("%v records written\n", count)
				}
				if count >= max {
					break write_loop
				}
			}
			if count%5000 != 0 {
				fmt.Printf("%v records written\n", count)
			}

		case "db", "dbs", "use":

			if len(parts) > 2 {
				fmt.Printf("usage: use id\n")
				break
			}
			if len(parts) == 2 {
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
				if len(selectedDBs) > 0 {
					myDB = selectedDBs[0]
				}
			}
			for i := 0; i < len(allDBs); i++ {
				if allDBs[i] == myDB || contains(selectedDBs, allDBs[i]) {
					fmt.Printf("--> ")
				} else {
					fmt.Printf("    ")
				}
				fmt.Printf("[%v] %s: %s\n", i, allDBs[i].Identity(), allDBs[i].Path())
			}
			break

		case "close":
			if myDB != nil {
				myDB.Close()
				myDB = nil
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
			break

		default:
			//check if it is the name of an matrix
			_, ok := matrixes[parts[0]]
			if ok {
				printMatrix(parts[0])
			} else {
				fmt.Printf("Unknown command or object\n")
			}

		}
	}
	if myDB != nil {
		myDB.Close()
	}
}

func printMatrix(mat string) {
	fa := mat64.Formatted(matrixes[mat], mat64.Prefix("    "))
	fmt.Printf("%s =\n%v\n", mat, fa)
}

func open(path string) {
	path, err := filepath.Abs(path)
	if err != nil {
		fmt.Printf("Malformed path\n")
		return
	}
	myDB, err = anydb.Open(path)
	if err != nil {
		fmt.Printf("Could not open database %s: %v\n", path, err)
		return
	}
	fmt.Printf("Database opened. %v records found\n", myDB.Entries())

	// setup iterator
	myDB.Scan()

	allDBs = append(allDBs, myDB)
	selectedDBs = append(selectedDBs, myDB)

	var d dbInfo
	dbInfos = append(dbInfos, d)

}

/*
\a   U+0007 alert or bell
\b   U+0008 backspace
\f   U+000C form feed
\n   U+000A line feed or newline
\r   U+000D carriage return
\t   U+0009 horizontal tab
\v   U+000b vertical tab
\\   U+005c backslash
\'   U+0027 single quote  (valid escape only within rune literals)
\"   U+0022 double quote  (valid escape only within string literals)
*/
