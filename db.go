package main

import (
	"os"
	"fmt"
	"time"
	"github.com/go-co-op/gocron"
	badger "github.com/dgraph-io/badger/v4"
)

/** Database functions **/

func countRegisteredDevices() float64{
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

func initDB() (* badger.DB){
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


