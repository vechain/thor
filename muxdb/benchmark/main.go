package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	tmpDir := os.TempDir()
	{
		path := filepath.Join(tmpDir, "trie-db")
		fmt.Println("benchmark non-optimized:", path)
		b := bench{path, false}
		if err := b.Run(); err != nil {
			return err
		}
	}
	{
		path := filepath.Join(tmpDir, "trie-db-optimized")
		fmt.Println("benchmark optimized:", path)
		b := bench{path, true}
		if err := b.Run(); err != nil {
			return err
		}
	}
	return nil
}
