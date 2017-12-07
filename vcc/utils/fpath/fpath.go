// Package fpath helper function for file path operations
package fpath

import (
	"os"
	"os/user"
	"path/filepath"
)

// HomeDir returns home dir of current user if have, or current working dir
func HomeDir() (string, error) {
	// try to get HOME env
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	//
	user, err := user.Current()
	if err != nil {
		return "", err
	}
	if user.HomeDir != "" {
		return user.HomeDir, nil
	}

	return os.Getwd()
}

// SizeOfDir calculate disk usage in bytes of a dir
func SizeOfDir(path string) (int64, error) {
	size := int64(0)
	if err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return size, nil
}

// PathExists to check if path exists
func PathExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
