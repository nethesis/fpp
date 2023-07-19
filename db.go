package main

import (
	"errors"
	"fmt"
	badger "github.com/dgraph-io/badger/v4"
	"github.com/go-co-op/gocron"
	"os"
	"time"
)

/** Database functions **/

func countRegisteredDevices() float64 {
	var count = 0
	db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if !it.Item().IsDeletedOrExpired() {
				count++
			}
		}
		return nil
	})

	return float64(count)
}

func getTokenFromTopic(topic string) (string, error) {
	var deviceToken []byte
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(topic))
		if err != nil {
			return errors.New("No topic found")
		}
		if item.IsDeletedOrExpired() {
			return errors.New("Topic deleted or expired")
		}
		ierr := item.Value(func(val []byte) error {
			deviceToken = make([]byte, len(val))
			copy(deviceToken, val)
			return nil
		})
		return ierr
	})

	return string(deviceToken), err
}

func saveTopicToken(topic string, token string) error {
	// Set a 6-months  TTL
	return db.Update(func(txn *badger.Txn) error {
		record := badger.NewEntry([]byte(topic), []byte(token)).WithTTL(24 * 180 * time.Hour)
		err := txn.SetEntry(record)
		return err
	})
}

func deleteTopic(topic string) error {
	return db.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(topic))
		return err
	})
}

func initDB() *badger.DB {
	dbPath := os.Getenv("DB_PATH")
	if len(dbPath) == 0 {
		fmt.Fprintln(os.Stderr, "Error initializing DB: invalid db path")
		os.Exit(4)
	}

	db, err := badger.Open(badger.DefaultOptions(dbPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error initializing DB: ", err.Error())
		os.Exit(5)
	}

	// Setup db cleanup job
	cleanupJob := gocron.NewScheduler(time.UTC)
	cleanupJob.Every(2).Seconds().Do(func() {
		err := db.RunValueLogGC(0.5)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error on DB cleanup: ", err.Error())
		}
	})
	return db
}
