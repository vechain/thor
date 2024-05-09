package rotatewriter

import (
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRotatingLogWriter(t *testing.T) {
	temp, err := os.MkdirTemp("", "*")
	require.NoError(t, err)

	// Create a rotating log writer with a maximum size of 1 MB
	writer, err := New(
		WithDir(temp),
		WithFileBaseName("derp"),
		WithFileMaxSize(1024*1024),
		WithMaxNumberFiles(5),
	)
	assert.Nil(t, err)

	require.NoError(t, writer.Start())

	createdFile := writer.(*Writer).currentFile.Name()

	// Use the rotating log writer with the standard log package
	data := make([]byte, 1024*1024)
	rand.Read(data) //nolint: gosec,staticcheck
	_, err = writer.Write(data)
	assert.Nil(t, err)

	// Use the rotating log writer with the standard log package
	rand.Read(data) //nolint: gosec,staticcheck
	_, err = writer.Write(data)
	assert.Nil(t, err)

	assert.NotEqual(t, createdFile, writer.(*Writer).currentFile.Name())

	// make sure the milliseconds dont colide
	createdFile = writer.(*Writer).currentFile.Name()
	// Use the rotating log writer with the standard log package
	rand.Read(data) //nolint: gosec,staticcheck
	_, err = writer.Write(data)
	assert.Nil(t, err)

	assert.NotEqual(t, createdFile, writer.(*Writer).currentFile.Name())
}

func TestStdoutWriter(t *testing.T) {
	writer := StdoutWriter()
	data := make([]byte, 1024*1024)
	rand.Read(data) //nolint: gosec,staticcheck
	i, err := writer.Write(data)
	assert.Nil(t, err)
	assert.Equal(t, len(data), i)
}
