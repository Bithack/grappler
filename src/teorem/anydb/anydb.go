/*
Package anydb provides a common lib agains different key-value storage
Currently supported: lmdb, leveldb
To be supported: bolt
*/
package anydb

import (
	"bytes"
	"fmt"
	"os"

	"github.com/bmatsuo/lmdb-go/lmdb"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
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

	keyFilter [2]int
}

// Entries returns estimated(?) number of entries
func (db *ADB) Entries() (entries uint64) {
	switch db.identity {
	case "lmdb":
		stat, _ := db.lmdbEnv.Stat()
		entries = stat.Entries
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

// Get return the value of a key
func (db *ADB) Get(k []byte) (key []byte, value []byte, err error) {
	if bytes.Equal(k, []byte("last")) {
		key = db.lastKey
	} else {
		key = k
	}
	switch db.identity {
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

//Path returns path of this db
func (db *ADB) Path() string {
	return db.path
}

//Identity returns identity of this db
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
		}
	}
}

// Seek moves the cursor to k
func (db *ADB) Seek(k []byte) {
	//what about the key filter ?
	db.lmdbCursor.Get(k, nil, lmdb.SetRange)
}

// Key returns key of current iterator
func (db *ADB) Key() (key []byte) {
	switch db.identity {
	case "leveldb":
		key = db.levelIterator.Key()
	case "lmdb":
		key = db.lmdbKey
	}

	//apply key filter
	if db.keyFilter[0] != 0 || db.keyFilter[1] != -1 {
		if db.keyFilter[1] == -1 || db.keyFilter[1] > len(key) {
			key = key[db.keyFilter[0]:]
		} else {
			key = key[db.keyFilter[0] : db.keyFilter[1]+1]
		}
	}
	return
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
}

// Open opens a database located at the supplied path (could be file or directory)
func Open(path string) (db *ADB, err error) {

	db = &ADB{}

	//first detect db type
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		_, err = os.Stat(path + "/data.mdb")
		if err == nil {
			db.identity = "lmdb"
		} else {
			//TODO: do some check
			db.identity = "leveldb"
		}
	} else {
		//assume bolt
		db.identity = "bolt"
	}

	//then open it
	switch db.identity {
	case "lmdb":
		db.lmdbEnv, err = lmdb.NewEnv()
		if err != nil {
			return nil, err
		}
		err = db.lmdbEnv.Open(path, 0, 0644)
		if err != nil {
			return nil, err
		}
		db.path = path
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
	}

	db.keyFilter[0] = 0
	db.keyFilter[1] = -1
	return
}
