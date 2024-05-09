// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rotatewriter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type RotateWriter interface {
	Start() error
	Write(p []byte) (int, error)
}

type Writer struct {
	dirPath         string
	fileBaseName    string
	maxFileSize     int64
	maxNumFiles     int
	currentFile     *os.File
	currentSize     int64
	numFilesRotated int
}

func (w *Writer) Start() error {
	// Open the current log file
	err := w.openNextFile()
	if err != nil {
		return fmt.Errorf("unable to open next file")
	}

	return nil
}

func (w *Writer) Write(p []byte) (int, error) {
	if w.currentFile == nil {
		return 0, io.ErrClosedPipe
	}

	// Rotate the log file if it exceeds the maximum file size
	if w.currentSize+int64(len(p)) > w.maxFileSize {
		err := w.rotateLogFile()
		if err != nil {
			return 0, err
		}
	}

	// Write the log message to the current log file
	n, err := w.currentFile.Write(p)
	w.currentSize += int64(n)
	return n, err
}

func (w *Writer) openNextFile() error {
	if w.currentFile != nil {
		err := w.currentFile.Close()
		if err != nil {
			fmt.Println(err)
		}
	}

	w.currentSize = 0
	w.numFilesRotated = 0

	// Construct the file name for the next log file
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	filename := w.fileBaseName + "-" + timestamp + ".log"
	filePath := filepath.Join(w.dirPath, filename)

	_, err := os.Stat(filePath)
	if err == nil { // File exists, append ms
		timestamp = time.Now().Format("2006-01-02T15-04-05.000")
		filename = w.fileBaseName + "-" + timestamp + ".log"
		filePath = filepath.Join(w.dirPath, filename)
	}

	// Open the next log file
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}

	w.currentFile = file
	return nil
}

func (w *Writer) rotateLogFile() error {
	err := w.openNextFile()
	if err != nil {
		return err
	}

	w.numFilesRotated++

	// Delete old log files if the maximum number of files has been exceeded
	if w.maxNumFiles > 0 && w.numFilesRotated >= w.maxNumFiles {
		err = w.deleteOldLogFiles()
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *Writer) deleteOldLogFiles() error {
	// List all log files in the directory
	files, err := filepath.Glob(filepath.Join(w.dirPath, w.fileBaseName+"-*.log"))
	if err != nil {
		return err
	}

	// Sort the log files by name (oldest first)
	sort.Strings(files)

	// Delete the oldest log files
	for i := 0; i < len(files)-w.maxNumFiles+1; i++ {
		err := os.Remove(files[i])
		if err != nil {
			return err
		}
	}

	return nil
}
