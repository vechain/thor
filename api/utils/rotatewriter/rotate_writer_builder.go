package rotatewriter

import "fmt"

type BuilderOptionsFunc func(*Writer) error

func New(opts ...BuilderOptionsFunc) (RotateWriter, error) {
	r := &Writer{
		dirPath:         "",
		fileBaseName:    "",
		maxFileSize:     0,
		maxNumFiles:     0,
		currentFile:     nil,
		currentSize:     0,
		numFilesRotated: 0,
	}
	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}
	if r.dirPath == "" || r.fileBaseName == "" || r.maxFileSize == 0 || r.maxNumFiles == 0 {
		return nil, fmt.Errorf("missing base setting(s)")
	}

	return r, nil
}

func WithMaxNumberFiles(i int) BuilderOptionsFunc {
	return func(w *Writer) error {
		w.maxNumFiles = i
		return nil
	}
}

func WithFileMaxSize(i int64) BuilderOptionsFunc {
	return func(w *Writer) error {
		w.maxFileSize = i
		return nil
	}
}

func WithFileBaseName(s string) BuilderOptionsFunc {
	return func(w *Writer) error {
		w.fileBaseName = s
		return nil
	}
}

func WithDir(s string) BuilderOptionsFunc {
	return func(w *Writer) error {
		w.dirPath = s
		return nil // TODO check if dir is writable
	}
}
