package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"teorem/anydb"
	"teorem/grappler/caffe"
	"teorem/grappler/vars"
	"teorem/multimatrix/matchar"

	"github.com/chzyer/readline"
	"github.com/gonum/matrix/mat64"
)

var allDBs []*anydb.ADB
var selectedDBs []*anydb.ADB

var limit uint64

var maxPrintWidth = 10

var dbPath string
var lastKey []byte
var lastValue []byte
var lastFloats []float32

var debugMode = false

var variables = make(map[string]*vars.Variable)

var matrixes = make(map[string]*mat64.Dense)
var matrixesChar = make(map[string]*matchar.Matchar)

type generateConfig struct {
	PercentCropping   int      `json:"parcent_cropping"`
	CropAll           bool     `json:"crop_all"`
	OperationCount    int      `json:"operation_count"`
	DefaultOperations []string `json:"default_operations"`
}

type grapplerConfig struct {
	Path     []string `json:"path"`
	Servers  []string `json:"servers"`
	Generate generateConfig
	Workers  int
}

var config grapplerConfig

func setDefaultConfig() {
	config.Generate.PercentCropping = 80
	config.Generate.OperationCount = 1
	config.Generate.CropAll = true
	config.Generate.DefaultOperations = []string{"brightness", "contrast", "gamma", "blur", "sharpness"}
	// the number of workers to use for the heavy stuff (like generate dataset)
	// defaults to number of logical cpus
	config.Workers = runtime.NumCPU()
}

var usr *user.User
var hostName string

// InterruptRequested is set to true when the program receives a system signal
var InterruptRequested bool

func main() {

	fmt.Printf(grapplerLogo + "\n\nTeorem Data Grappler\nVersion 0.0.12\n")

	rand.Seed(time.Now().UTC().UnixNano())

	// Use this to trap and handle system signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for {
			select {
			case <-c:
				if InterruptRequested {
					fmt.Printf("\n- Terminated -\n")
					// second signal without result -> terminate
					for _, db := range allDBs {
						db.Close()
					}
					os.Exit(1)
				}
				InterruptRequested = true
			}
		}
	}()

	if len(os.Args) > 1 {
		for i := 1; i < len(os.Args); i++ {
			dbPath = os.Args[i]
			open(dbPath)
		}
	}
	usr, _ = user.Current()
	hostName, _ := os.Hostname()

	// load config
	setDefaultConfig()
	configFile := usr.HomeDir + "/.config/grappler/config.json"
	file, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Printf("No config file found. Creating %v\n", configFile)
		os.MkdirAll(usr.HomeDir+"/.config/grappler/", 0700)
		ioutil.WriteFile(configFile, []byte("{}"), 0644)
	} else {
		err = json.Unmarshal(file, &config)
		if err != nil {
			fmt.Printf("Error parsing config %v: %v\n", configFile, err)
		} else {
			fmt.Printf("Config read from %v\n", configFile)
		}
	}

	fmt.Printf("Interactive mode. Type \"help\" for commands.\n")

	l, err := readline.NewEx(&readline.Config{
		Prompt:                 "[" + hostName + "] \033[31m>\033[0m ",
		HistoryFile:            usr.HomeDir + "/.grappler_history",
		AutoComplete:           completer,
		InterruptPrompt:        "^C",
		EOFPrompt:              "exit",
		DisableAutoSaveHistory: true,
		HistorySearchFold:      true,
	})
	if err != nil {
		fmt.Printf("Readline failed\n")
		return
	}
	defer l.Close()

	var text string
	for {
		InterruptRequested = false

		text, _ = l.Readline()

		if text != "q" && text != "quit" {
			l.SaveHistory(text)
		}

		if !eval(text) {
			break
		}

	}

	//close all open connections
	for _, db := range allDBs {
		db.Close()
	}
}

func grCloseDB(db *anydb.ADB) {
	for i := range allDBs {
		if db == allDBs[i] {
			allDBs = append(allDBs[:i], allDBs[i+1:]...)
			break
		}
	}
	for i := range selectedDBs {
		if db == selectedDBs[i] {
			selectedDBs = append(selectedDBs[:i], selectedDBs[i+1:]...)
			break
		}
	}
	db.Close()
}

// Create new db and return it
// Does not update selectedDBs
func create(path string, dbtype string) (db *anydb.ADB, err error) {
	db, err = anydb.Create(path, dbtype)
	if err != nil {
		return nil, err
	}
	allDBs = append(allDBs, db)
	// setup iterator
	db.Scan()
	return
}

// Open LMDB, leveldb, aerospike server or just an file folder
// Will try to guess which kinds of database path refers to
// We can also give directions with "aerospike:t4", "lmdb:/foo/bar"
func open(path string) {

	dbType := ""

	// check prefix
	check := strings.Split(path, ":")
	if len(check) == 2 {
		dbType = check[0]
		path = check[1]
	}

	//a server from config ?
	if contains(config.Servers, path) {
		dbType = "aerospike"
	}

	// try first given path, then combined with paths from the config file
	// skip if aerospike used as prefix?
	var toTry = []string{path}
	for _, p := range config.Path {
		toTry = append(toTry, filepath.Join(p, path))
	}

	for _, p := range toTry {

		grLogs("Trying %v", p)

		//check for non-database files (like caffemodel or prototxt)
		switch filepath.Ext(p) {
		case ".caffemodel":
			data, err := ioutil.ReadFile(p)
			if err != nil {
				grLogs("Error reading file: %v", err)
				return
			}
			var net caffe.Message
			err = net.Unmarshal(data, "NetParameter")
			if err != nil {
				grLogs("Error unmarshaling protobuf: %v", err)
				return
			}
			variables["caffemodel"] = vars.NewFromMessage(&net)
			variables["caffemodel"].Print("caffemodel")
			return

		case ".prototxt":
			data, err := ioutil.ReadFile(p)
			if err != nil {
				grLogs("%v", err)
				return
			}
			var net caffe.Message
			err = net.UnmarshalText(data, "NetParameter")
			if err != nil {
				grLogs("%v", err)
				return
			}
			variables["caffemodel"] = vars.NewFromMessage(&net)
			variables["caffemodel"].Print("caffemodel")
			return
		}

		myDB, err := anydb.Open(p, dbType)
		if err == nil {
			fmt.Printf("Database opened.")
			e := myDB.Entries()
			if e > 0 {
				fmt.Printf(" %v records found", e)
			}
			fmt.Printf("\n")

			// setup iterator
			myDB.Scan()

			allDBs = append(allDBs, myDB)
			selectedDBs = []*anydb.ADB{myDB}

			return
		} else {
			grLog(fmt.Sprintf("Open failed: %v", err))
		}
	}
	fmt.Printf("Could not open database %s\n", path)
}

func grLog(message string) {
	if debugMode {
		fmt.Printf("%v\n", message)
	}
}

func grLogsf(f *os.File, s string, args ...interface{}) {
	f.WriteString(fmt.Sprintf(s, args...))
	fmt.Printf(s, args...)
}

func grLogs(s string, a ...interface{}) {
	grLog(fmt.Sprintf(s, a...))
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

var completer = readline.NewPrefixCompleter(
	readline.PcItem("close"),
	readline.PcItem("dbs"),
	readline.PcItem("use"),
	readline.PcItem("reset"),
	readline.PcItem("help", readline.PcItemDynamic(listFunctions)),
	readline.PcItem("ls"),
	readline.PcItem("open",
		readline.PcItemDynamic(listFiles(".")),
	),
	readline.PcItem("who"),
	readline.PcItem("show", readline.PcItem("namespaces"), readline.PcItem("sets"), readline.PcItem("bins")),
	readline.PcItem("generate", readline.PcItem("siamese", readline.PcItem("dataset"))),
	readline.PcItem("get"),
	readline.PcItem("load", readline.PcItem("keys"), readline.PcItem("floats")),
	readline.PcItem("set", readline.PcItem("limit"), readline.PcItem("filter")),
	readline.PcItem("write", readline.PcItemDynamic(listVars)),
	readline.PcItem("put"),
	readline.PcItemDynamic(listVars, readline.PcItem("=", readline.PcItemDynamic(listVars))),
)

func listFunctions(line string) []string {
	names := make([]string, 0)
	for mat := range helpTexts {
		names = append(names, mat)
	}
	return names
}

func listVars(line string) []string {
	names := make([]string, 0)
	for n, v := range variables {
		names = append(names, n)
		for _, s := range v.GetFields() {
			names = append(names, n+"."+s)
		}
	}
	for mat := range matrixes {
		names = append(names, mat)
	}
	return names
}

// Function constructor - constructs new function for listing given directory
func listFiles(path string) func(string) []string {
	return func(line string) []string {
		names := make([]string, 0)
		files, _ := ioutil.ReadDir(path)
		for _, f := range files {
			names = append(names, f.Name())
		}
		return names
	}
}

var grapplerLogo = ` _______ ______ _______ ______ ______ _____   _______ ______ 
|     __|   __ \   _   |   __ \   __ \     |_|    ___|   __ \
|    |  |      <       |    __/    __/       |    ___|      <
|_______|___|__|___|___|___|  |___|  |_______|_______|___|__|`
