package database

import (
	"fmt"
	"log"
	"os"

	"github.com/FraMan97/kairos-p2p-engine/internal/config"
	"github.com/dgraph-io/badger/v4"
)

func makeKey(prefix, key string) []byte {
	return []byte(fmt.Sprintf("%s:%s", prefix, key))
}

func OpenDatabase(dbDir string) (*badger.DB, error) {
	log.Printf("[Database] - [INFO] Attempting to open BadgerDB in directory: %s", dbDir)

	err := os.MkdirAll(dbDir, 0700)
	if err != nil {
		log.Printf("[Database] - [ERROR] Failed to create database directory: %v", err)
		return nil, err
	}

	opts := badger.DefaultOptions(dbDir)
	opts.Logger = nil

	db, err := badger.Open(opts)
	if err != nil {
		log.Printf("[Database] - [ERROR] Critical failure opening BadgerDB: %v", err)
		return nil, err
	}

	log.Printf("[Database] - [SUCCESS] BadgerDB opened successfully in '%s'", dbDir)
	config.DB = db
	return db, nil
}

func GetData(db *badger.DB, prefix string, key string) ([]byte, error) {
	var value []byte
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(makeKey(prefix, key))
		if err != nil {
			return err
		}
		err = item.Value(func(val []byte) error {
			value = append([]byte{}, val...)
			return nil
		})
		return err
	})

	if err != nil {
		if err != badger.ErrKeyNotFound {
			log.Printf("[Database: GetData] - [ERROR] Failed to retrieve key '%s:%s': %v", prefix, key, err)
		}
		return nil, err
	}
	return value, nil
}

func PutData(db *badger.DB, prefix string, key string, data []byte) error {
	err := db.Update(func(txn *badger.Txn) error {
		return txn.Set(makeKey(prefix, key), data)
	})

	if err != nil {
		log.Printf("[Database: PutData] - [ERROR] Failed to write key '%s:%s': %v", prefix, key, err)
	}
	return err
}

func DeleteKey(db *badger.DB, prefix string, key string) error {
	err := db.Update(func(txn *badger.Txn) error {
		return txn.Delete(makeKey(prefix, key))
	})

	if err != nil {
		log.Printf("[Database: DeleteKey] - [ERROR] Failed to delete key '%s:%s': %v", prefix, key, err)
	}
	return err
}

func ExistsKey(db *badger.DB, prefix string, key string) (bool, error) {
	exists := false
	err := db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(makeKey(prefix, key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}
		exists = true
		return nil
	})

	if err != nil {
		log.Printf("[Database: ExistsKey] - [ERROR] Unexpected error checking key '%s:%s': %v", prefix, key, err)
	}
	return exists, err
}

func GetAllData(db *badger.DB, prefix string) (map[string][]byte, error) {
	values := make(map[string][]byte)
	prefixBytes := []byte(fmt.Sprintf("%s:", prefix))

	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefixBytes); it.ValidForPrefix(prefixBytes); it.Next() {
			item := it.Item()
			k := item.Key()
			originalKey := string(k[len(prefixBytes):])

			err := item.Value(func(v []byte) error {
				valueCopy := append([]byte{}, v...)
				values[originalKey] = valueCopy
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		log.Printf("[Database: GetAllData] - [ERROR] Failed to iterate over prefix '%s': %v", prefix, err)
	}
	return values, err
}

func GetAllKeys(db *badger.DB, prefix string) ([]string, error) {
	var keys []string
	prefixBytes := []byte(fmt.Sprintf("%s:", prefix))

	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false

	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefixBytes); it.ValidForPrefix(prefixBytes); it.Next() {
			k := it.Item().Key()
			keys = append(keys, string(k[len(prefixBytes):]))
		}
		return nil
	})

	if err != nil {
		log.Printf("[Database: GetAllKeys] - [ERROR] Failed to list keys for prefix '%s': %v", prefix, err)
	}
	return keys, err
}