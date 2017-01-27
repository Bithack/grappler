package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"teorem/anydb"
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

var matrixes = make(map[string]*mat64.Dense)
var matrixesChar = make(map[string]*matchar.Matchar)

type generateConfig struct {
	PercentCropping int
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
	// the number of workers to use for the heavy stuff (like generate dataset)
	// defaults to number of logical cpus
	config.Workers = runtime.NumCPU()
}

var usr *user.User

func main() {

	fmt.Printf(grapplerLogo + "\n\nTeorem Data Grappler\nVersion 0.0.11\n")

	rand.Seed(time.Now().UTC().UnixNano())

	if len(os.Args) > 1 {
		for i := 1; i < len(os.Args); i++ {
			dbPath = os.Args[i]
			open(dbPath)
		}
	}
	usr, _ = user.Current()

	// load config
	setDefaultConfig()
	configFile := usr.HomeDir + "/.config/grappler/config.json"
	file, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Printf("No config file found. Creating %v\n", configFile)
		os.MkdirAll(usr.HomeDir+"/.config/grappler/", 0700)
		ioutil.WriteFile(configFile, []byte("{}"), 0644)
	} else {
		json.Unmarshal(file, &config)
		fmt.Printf("Config read from %v\n", configFile)
	}

	fmt.Printf("Interactive mode. Type \"help\" for commands.\n")

	l, err := readline.NewEx(&readline.Config{
		Prompt:                 "\033[31m>\033[0m ",
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
		text, _ = l.Readline()
		if !eval(text) {
			break
		}
		l.SaveHistory(text)
	}

	//close all open connections
	for _, db := range allDBs {
		db.Close()
	}
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

		if debugMode {
			fmt.Printf("Trying %v\n", p)
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
		}
	}
	fmt.Printf("Could not open database %s\n", path)
}

func grLog(message string) {
	if debugMode {
		fmt.Printf("%v\n", message)
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
	readline.PcItem("write"),
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
