package utils

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

func DownloadFile(filePath string, url string) error {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create the file
	dirName := filepath.Dir(filePath)
	if _, serr := os.Stat(dirName); serr != nil {
		merr := os.MkdirAll(dirName, os.ModePerm)
		if merr != nil {
			panic(merr)
		}
	}
	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func ConcatenateMessageSender(user, chat string) string {
	return user + "/" + chat
}

// thread-safe typed map wrapper
type SafeMap[K comparable, V any] struct {
	mx sync.RWMutex
	m  map[K]V
}

func NewSafeMap[K comparable, V any]() SafeMap[K, V] {
	return SafeMap[K, V]{
		m: make(map[K]V),
	}
}

func (tm *SafeMap[K, V]) Get(key K) V {
	tm.mx.RLock()
	defer tm.mx.RUnlock()
	return tm.m[key]
}

func (tm *SafeMap[K, V]) Set(key K, value V) {
	tm.mx.Lock()
	defer tm.mx.Unlock()
	tm.m[key] = value
}

func (tm *SafeMap[K, V]) Delete(key K) {
	tm.mx.Lock()
	defer tm.mx.Unlock()
	delete(tm.m, key)
}

func (tm *SafeMap[K, V]) Contains(key K) bool {
	tm.mx.RLock()
	defer tm.mx.RUnlock()
	_, exists := tm.m[key]
	return exists
}
