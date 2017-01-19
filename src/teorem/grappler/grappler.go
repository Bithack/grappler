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
	"strings"
	"time"

	"teorem/anydb"
	"teorem/multimatrix/matchar"
	"teorem/tinyprompt"

	"github.com/gonum/matrix/mat64"
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

var debugMode = false

var matrixes = make(map[string]*mat64.Dense)
var matrixesChar = make(map[string]*matchar.Matchar)

type grapplerConfig struct {
	Path    []string `json:"path"`
	Servers []string `json:"servers"`
}

var config grapplerConfig
var usr *user.User

func main() {

	fmt.Printf("Teorem Data Grappler\nVersion 0.0.10\n")

	rand.Seed(time.Now().UTC().UnixNano())

	if len(os.Args) > 1 {
		for i := 1; i < len(os.Args); i++ {
			dbPath = os.Args[i]
			open(dbPath)
		}
	}
	usr, _ = user.Current()

	// load history
	tinyprompt.LoadHistory(usr.HomeDir + "/.grappler_history")

	// load config
	configFile := usr.HomeDir + "/.config/grappler/config.json"
	file, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Printf("No config file found. Creating %v\n", configFile)
		os.MkdirAll(usr.HomeDir+"/.config/grappler/", 0700)
		ioutil.WriteFile(configFile, []byte("{}"), 0644)
	} else {
		json.Unmarshal(file, &config)
	}

	fmt.Printf("Interactive mode. Type \"help\" for commands.\n")

	for {
		text := tinyprompt.GetCommand(debugMode)
		if !eval(text) {
			break
		}
	}

	//close all open connections
	for _, db := range allDBs {
		db.Close()
	}
	// save history
	tinyprompt.SaveHistory("~/.grappler_history", []string{"q", "quit"})
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

	var err error
	for _, p := range toTry {

		if debugMode {
			fmt.Printf("Trying %v\n", p)
		}

		myDB, err = anydb.Open(p, dbType)
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

			var d dbInfo
			dbInfos = append(dbInfos, d)

			return
		}
	}
	fmt.Printf("Could not open database %s\n", path)
}

/*
+-----+---+--------------------------+
| rwx | 7 | Read, write and execute  |
| rw- | 6 | Read, write              |
| r-x | 5 | Read, and execute        |
| r-- | 4 | Read,                    |
| -wx | 3 | Write and execute        |
| -w- | 2 | Write                    |
| --x | 1 | Execute                  |
| --- | 0 | no permissions           |
+------------------------------------+

+------------+------+-------+
| Permission | Octal| Field |
+------------+------+-------+
| rwx------  | 0700 | User  |
| ---rwx---  | 0070 | Group |
| ------rwx  | 0007 | Other |
+------------+------+-------+
*/
