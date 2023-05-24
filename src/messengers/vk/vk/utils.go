package vk

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
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
