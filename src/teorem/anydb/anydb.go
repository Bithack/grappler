/*
Package anydb provides a common lib agains different key-value storage
Currently supported: lmdb, leveldb
Might be supported in the future: bolt, aerospike
*/
package anydb

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"time"

	"github.com/bmatsuo/lmdb-go/lmdb"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"

	aerospike "github.com/aerospike/aerospike-client-go"
)

// ADB is the anydb struct
type ADB struct {
	identity string
	path     string

	lastKey []byte

	leveldb       *leveldb.DB
	levelIterator iterator.Iterator

	lmdb       lmdb.DBI
	lmdbEnv    *lmdb.Env
	lmdbTxn    *lmdb.Txn
	lmdbCursor *lmdb.Cursor
	//lmdbScanner *lmdbscan.Scanner
	lmdbKey   []byte
	lmdbValue []byte

	folderFiles    []os.FileInfo
	folderIterator int

	aerospikeClient *aerospike.Client

	keyFilter [2]int
}

// Entries returns estimated(?) number of entries
func (db *ADB) Entries() (entries uint64) {
	switch db.identity {
	case "lmdb":
		stat, _ := db.lmdbEnv.Stat()
		entries = stat.Entries
	case "folder":
		entries = uint64(len(db.folderFiles))
	}
	return
}

// Stat prints some lmdb stats
func (db *ADB) Stat() {
	switch db.identity {
	case "lmdb":
		stat, _ := db.lmdbEnv.Stat()
		fmt.Printf("%+v\n", stat)
	}

}

// Get returns the value of a key
func (db *ADB) Get(k []byte) (key []byte, value []byte, err error) {
	if bytes.Equal(k, []byte("last")) {
		key = db.lastKey
	} else {
		key = k
	}
	switch db.identity {
	case "folder":

	case "leveldb":
		value, err = db.leveldb.Get(key, nil)

	case "lmdb":
		err = db.lmdbEnv.View(func(txn *lmdb.Txn) (err error) {
			v, err := txn.Get(db.lmdb, key)
			value = make([]byte, len(v))
			copy(value, v)

			return err
		})

	}
	return
}

// Path returns path of this db
func (db *ADB) Path() string {
	return db.path
}

// Identity returns identity of this db
func (db *ADB) Identity() string {
	return db.identity
}

// SetFilterRange adds a filter to the output of keys, returning a subset of the key name between position i and j
func (db *ADB) SetFilterRange(i int, j int) {
	db.keyFilter[0] = i
	db.keyFilter[1] = j
}

// Release frees the iterator if there is one
func (db *ADB) Release() {
	if db.levelIterator != nil {
		db.levelIterator.Release()
		db.levelIterator = nil
	}
	if db.lmdbCursor != nil {
		db.lmdbCursor.Close()
		db.lmdbCursor = nil
	}
	if db.lmdbTxn != nil {
		db.lmdbTxn.Abort()
		db.lmdbTxn = nil
	}
}

// Scan setups iterator/cursor if there is none
func (db *ADB) Scan() {
	switch db.identity {
	case "leveldb":
		if db.levelIterator == nil {
			db.levelIterator = db.leveldb.NewIterator(nil, nil)
		}

	case "lmdb":
		var err error
		if db.lmdbTxn == nil {
			db.lmdbTxn, err = db.lmdbEnv.BeginTxn(nil, 0)
			if err != nil {
				fmt.Printf("Could not start transcation\n")
				return
			}
			db.lmdbCursor, err = db.lmdbTxn.OpenCursor(db.lmdb)
			if err != nil {
				fmt.Printf("Could not open cursor\n")
				return
			}
			db.Reset()
		}
	}
}

// Seek moves the cursor to k
func (db *ADB) Seek(k []byte) {
	switch db.identity {
	case "lmdb":
		//what about the key filter ?
		db.lmdbCursor.Get(k, nil, lmdb.SetRange)
	}
}

// Reset moves the cursor to the top
func (db *ADB) Reset() {
	switch db.identity {
	case "lmdb":
		db.lmdbKey, db.lmdbValue, _ = db.lmdbCursor.Get(nil, nil, lmdb.First)
	case "folder":
		db.folderIterator = 0
	}
}

// Key returns key of current iterator
func (db *ADB) Key() (key []byte) {
	switch db.identity {
	case "leveldb":
		key = db.levelIterator.Key()
	case "lmdb":
		key = db.lmdbKey
	case "folder":
		key = []byte(db.folderFiles[db.folderIterator].Name())
	}

	//apply key filter
	if db.keyFilter[0] != 0 || db.keyFilter[1] != 0 {
		if db.keyFilter[1] == 0 || db.keyFilter[1] > len(key) {
			key = key[db.keyFilter[0]:]
		} else {
			key = key[db.keyFilter[0] : db.keyFilter[1]+1]
		}
	}
	return
}

// PutGeoJSON stores GeoJSON data in an aerospike namespace/set/bin
func (db *ADB) PutGeoJSON(namespace string, set string, bin string, key []byte, json string) error {
	switch db.identity {
	case "aerospike":
		asKey, err := aerospike.NewKey(namespace, set, key)
		if err != nil {
			return err
		}
		asBin := aerospike.Bin{
			Name:  bin,
			Value: aerospike.NewGeoJSONValue(json),
		}
		err = db.aerospikeClient.PutBins(nil, asKey, &asBin)
		if err != nil {
			return err
		}
	}
	return nil
}

// Put ...
func (db *ADB) Put(keys []byte, values []byte) {
	switch db.identity {
	case "aerospike":

	}
}

// Value returns value of current iterator
func (db *ADB) Value() (value []byte) {
	switch db.identity {
	case "leveldb":
		value = db.levelIterator.Value()
	case "lmdb":
		value = db.lmdbValue
	}
	return
}

// Next returns the next record
func (db *ADB) Next() bool {
	switch db.identity {
	case "folder":
		if db.folderIterator < len(db.folderFiles)-1 {
			db.folderIterator++
		} else {
			return false
		}

	case "leveldb":
		return db.levelIterator.Next()
	case "lmdb":
		var err error
		db.lmdbKey, db.lmdbValue, err = db.lmdbCursor.Get(nil, nil, lmdb.Next)
		if lmdb.IsNotFound(err) {
			return false
		}
		db.lastKey = db.lmdbKey
	}
	return true
}

// Show returns info from aerospike node
func (db *ADB) Show(info string) (r map[string]string, err error) {
	if db.aerospikeClient != nil {
		nodes := db.aerospikeClient.GetNodes()
		r, err = aerospike.RequestNodeInfo(nodes[0], info)
		return
	}
	return nil, errors.New("Sorry, only works with an aerospike db")
}

// SizeOf returns approximate size of supplied key range
func (db *ADB) SizeOf(start []byte, stop []byte) (size int64) {
	switch db.identity {
	case "leveldb":
		var r util.Range
		r.Start = start
		r.Limit = stop
		sizes, _ := db.leveldb.SizeOf([]util.Range{r})
		size = sizes[0]
	}
	return
}

// Close closes it
func (db *ADB) Close() {
	if db.leveldb != nil {
		db.leveldb.Close()
	}
	if db.lmdbEnv != nil {
		db.lmdbEnv.Close()
	}
	if db.aerospikeClient != nil {
		db.aerospikeClient.Close()
	}
}

// Open opens a database located at the supplied path (could be file or directory or server)
// With empty dbType is will guess
func Open(path string, dbType string) (db *ADB, err error) {
	db = &ADB{}

	if dbType != "" {
		db.identity, db.path = dbType, path
	} else {
		db.identity, db.path = guessDBType(path)
	}

	//now open it
	switch db.identity {
	case "aerospike":
		policy := aerospike.NewClientPolicy()
		policy.Timeout = 5000 * time.Millisecond
		db.aerospikeClient, err = aerospike.NewClientWithPolicy(policy, path, 3000)
		if err == nil {
			return db, nil
		}

	case "folder":
		db.folderFiles, _ = ioutil.ReadDir(db.path)

	case "lmdb":
		db.lmdbEnv, err = lmdb.NewEnv()
		if err != nil {
			return nil, err
		}
		err = db.lmdbEnv.Open(path, 0, 0644)
		if err != nil {
			return nil, err
		}
		db.lmdbEnv.SetMaxDBs(10)
		err = db.lmdbEnv.Update(func(txn *lmdb.Txn) (err error) {
			db.lmdb, err = txn.OpenRoot(0)
			return err
		})

	case "leveldb":
		var options opt.Options
		options.ErrorIfMissing = true
		db.leveldb, err = leveldb.OpenFile(path, &options)

	case "bolt":
		err = fmt.Errorf("bolt unsupported")

	case "unknown":
		err = errors.New("No such db")
	}

	return
}

func guessDBType(path string) (string, string) {

	re := regexp.MustCompile("^(?:[0-9]{1,3}\\.){3}[0-9]{1,3}$")
	if re.Match([]byte(path)) {
		//ip adress ?
		return "aerospike", path
	}

	// probably a file path
	//expand tilde symbol
	if len(path) > 1 && path[:2] == "~/" {
		usr, _ := user.Current()
		path = filepath.Join(usr.HomeDir, path[2:])
	}
	path, _ = filepath.Abs(path)

	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		_, err = os.Stat(path + "/data.mdb")
		if err == nil {
			return "lmdb", path
		}
		files, err := filepath.Glob("*.ldb")
		if err == nil && len(files) > 0 {
			return "leveldb", path
		}
		return "folder", path
	}

	return "unknown", path
}
