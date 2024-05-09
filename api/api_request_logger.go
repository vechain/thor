package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/otherview/filerotatewriter"
)

// RequestLogger logs requests to an output
type RequestLogger struct {
	enabled      bool
	writerChan   chan entry
	stopChan     chan bool
	outputWriter filerotatewriter.FileRotateWriter
}

func NewRequestLogger(enabled bool, fileRotate filerotatewriter.FileRotateWriter) *RequestLogger {
	return &RequestLogger{
		enabled:      enabled,
		outputWriter: fileRotate,
		writerChan:   make(chan entry, 100_000),
		stopChan:     make(chan bool),
	}
}

func (l *RequestLogger) Enabled() bool {
	return l.enabled
}

// Handle returns an http handler to ensure requests are syphoned into the writer
func (l *RequestLogger) Handle(handler http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		// Read and log the body (note: this can only be done once)
		// Ensure you don't disrupt the request body for handlers that need to read it
		var bodyBytes []byte
		var err error
		if r.Body != nil {
			bodyBytes, err = io.ReadAll(r.Body)
			if err != nil {
				log.Warn("unexpected body read error", "err", err)
				return // don't pass bad request to the next handler
			}
			r.Body = io.NopCloser(io.Reader(bytes.NewReader(bodyBytes)))
		}

		l.writerChan <- entry{
			Timestamp: time.Now(),
			URI:       r.URL.String(),
			Method:    r.Method,
			Body:      string(bodyBytes),
		}

		// call the original http.Handler we're wrapping
		handler.ServeHTTP(w, r)
	}

	// start the writer
	l.start()

	// http.HandlerFunc wraps a function so that it
	// implements http.Handler interface
	return http.HandlerFunc(fn)
}

func (l *RequestLogger) Stop() {
	if !l.enabled {
		return
	}

	// make sure any pending message is written
	l.stopChan <- true
	<-l.stopChan
}

func (l *RequestLogger) start() {
	go func() {
		for {
			select {
			case item := <-l.writerChan:
				marshal, err := json.Marshal(item)
				if err != nil {
					log.Warn("unable to marshal api request entry", "err", err)
					continue
				}
				_, err = l.outputWriter.Write(marshal)
				if err != nil {
					log.Warn("unable to write api request entry", "err", err)
				}
			case <-l.stopChan:
				close(l.stopChan)
				return
			}
		}
	}()
}

type entry struct {
	Timestamp time.Time `json:"timestamp"`
	URI       string    `json:"uri"`
	Method    string    `json:"method"`
	Body      string    `json:"body"`
}
