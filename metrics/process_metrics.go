// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

//go:build linux

package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

// IOCollector collects process-level I/O metrics from the /proc filesystem.
// It implements prometheus.Collector interface.
// It gathers I/O statistics that are not provided by Prometheus' default ProcessCollector.
//
// Note: CPU, memory, and open FD metrics are already provided by prometheus.NewProcessCollector()
// which is registered by default in prometheus.DefaultRegisterer.
//
// References:
//   - /proc/[pid]/io:   https://man7.org/linux/man-pages/man5/proc_pid_io.5.html
type IOCollector struct {
	pid int

	// Metric descriptors for I/O statistics
	readSyscallsDesc  *prometheus.Desc
	writeSyscallsDesc *prometheus.Desc
	readBytesDesc     *prometheus.Desc
	writeBytesDesc    *prometheus.Desc
}

// NewIOCollector creates a new IOCollector for the current process.
func NewIOCollector() *IOCollector {
	return &IOCollector{
		pid: os.Getpid(),

		readSyscallsDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "process", "read_syscalls_total"),
			"Total number of read I/O operations (syscalls such as read and pread).",
			nil, nil,
		),
		writeSyscallsDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "process", "write_syscalls_total"),
			"Total number of write I/O operations (syscalls such as write and pwrite).",
			nil, nil,
		),
		readBytesDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "process", "read_bytes_total"),
			"Total number of bytes read from the storage layer.",
			nil, nil,
		),
		writeBytesDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "process", "write_bytes_total"),
			"Total number of bytes written to the storage layer.",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *IOCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.readSyscallsDesc
	ch <- c.writeSyscallsDesc
	ch <- c.readBytesDesc
	ch <- c.writeBytesDesc
}

// Collect implements prometheus.Collector.
// It reads I/O metrics from /proc and sends them to the provided channel.
func (c *IOCollector) Collect(ch chan<- prometheus.Metric) {
	// Collect I/O data
	if io, err := c.getIOStats(); err == nil {
		ch <- prometheus.MustNewConstMetric(
			c.readSyscallsDesc,
			prometheus.CounterValue,
			float64(io.readSyscalls),
		)
		ch <- prometheus.MustNewConstMetric(
			c.writeSyscallsDesc,
			prometheus.CounterValue,
			float64(io.writeSyscalls),
		)
		ch <- prometheus.MustNewConstMetric(
			c.readBytesDesc,
			prometheus.CounterValue,
			float64(io.readBytes),
		)
		ch <- prometheus.MustNewConstMetric(
			c.writeBytesDesc,
			prometheus.CounterValue,
			float64(io.writeBytes),
		)
	}
}

// ioData holds data parsed from /proc/[pid]/io.
//
// Reference: https://man7.org/linux/man-pages/man5/proc_pid_io.5.html
//
// Fields:
//   - syscr: Number of read syscalls (e.g., read, pread).
//   - syscw: Number of write syscalls (e.g., write, pwrite).
//   - read_bytes: Number of bytes fetched from the storage layer.
//   - write_bytes: Number of bytes sent to the storage layer.
type ioData struct {
	readSyscalls  int64
	writeSyscalls int64
	readBytes     int64
	writeBytes    int64
}

// getIOStats parses /proc/[pid]/io to extract I/O statistics.
//
// Reference: https://man7.org/linux/man-pages/man5/proc_pid_io.5.html
//
// Fields:
//   - syscr: Number of read I/O operations (syscalls like read, pread).
//   - syscw: Number of write I/O operations (syscalls like write, pwrite).
//   - read_bytes: Number of bytes which this process really fetched from storage.
//   - write_bytes: Number of bytes which this process caused to be sent to storage.
func (c *IOCollector) getIOStats() (*ioData, error) {
	ioPath := fmt.Sprintf("/proc/%d/io", c.pid)
	file, err := os.Open(ioPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	result := &ioData{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}

		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			logger.Warn("unable to parse io value", "line", line, "err", err)
			continue
		}

		switch {
		case strings.HasPrefix(line, "syscr:"):
			result.readSyscalls = value
		case strings.HasPrefix(line, "syscw:"):
			result.writeSyscalls = value
		case strings.HasPrefix(line, "read_bytes:"):
			result.readBytes = value
		case strings.HasPrefix(line, "write_bytes:"):
			result.writeBytes = value
		}
	}

	return result, scanner.Err()
}

var registered atomic.Bool

// registerIOCollector registers the ProcessCollector with Prometheus.
func registerIOCollector() {
	if registered.CompareAndSwap(false, true) {
		prometheus.MustRegister(NewIOCollector())
	}
}
