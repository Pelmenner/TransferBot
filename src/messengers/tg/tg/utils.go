package tg

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
	defer func() { err = resp.Body.Close() }()

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
	defer func() { err = out.Close() }()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

// Map is thread-safe typed map wrapper
type Map[K comparable, V any] struct {
	mx sync.RWMutex
	mp map[K]V
}

func NewMap[K comparable, V any]() Map[K, V] {
	return Map[K, V]{
		mp: make(map[K]V),
	}
}

func (m *Map[K, V]) Get(key K) V {
	m.mx.RLock()
	defer m.mx.RUnlock()
	return m.mp[key]
}

func (m *Map[K, V]) Set(key K, value V) {
	m.mx.Lock()
	defer m.mx.Unlock()
	m.mp[key] = value
}

func (m *Map[K, V]) Delete(key K) {
	m.mx.Lock()
	defer m.mx.Unlock()
	delete(m.mp, key)
}

func (m *Map[K, V]) Contains(key K) bool {
	m.mx.RLock()
	defer m.mx.RUnlock()
	_, exists := m.mp[key]
	return exists
}
