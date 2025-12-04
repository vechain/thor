# VeChain Thor PebbleDB Logs Backend

This document provides a comprehensive technical overview of the PebbleDB logs backend implementation for VeChain Thor developers.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Key Schema Design](#key-schema-design)
3. [Posting-List Optimization](#posting-list-optimization)
4. [Query Engine](#query-engine)
5. [Data Flow & Write Path](#data-flow--write-path)
6. [Performance Characteristics](#performance-characteristics)
7. [Development Guide](#development-guide)
8. [Migration from SQLite](#migration-from-sqlite)

## Architecture Overview

### Design Philosophy

The PebbleDB logs backend represents a complete architectural shift from SQLite to a **streaming, posting-list based architecture** optimized for blockchain log queries. The system leverages PebbleDB's LSM-tree structure and key-value operations to achieve 5-10x performance improvements over SQLite.

### Key Components

- **PebbleDBLogDB** (`pebbledb.go`): Main database interface implementing `logsdb.LogsDB`
- **StreamingQueryEngine** (`query_engine.go`): Central query processor handling complex filter logic
- **Stream-based Iterator System**: Core performance optimization using streaming iteration
- **Binary Encoding System** (`encoding.go`): Custom encoding replacing RLP for 20-30% storage reduction
- **Multi-level Index Architecture**: Comprehensive indexing strategy for fast lookups

### Core Design Decisions

1. **LSM-Tree Optimization**: Uses PebbleDB's LSM-tree structure for sequential writes and range queries
2. **Streaming Architecture**: Eliminates in-memory joins and large result set buffering  
3. **Key-Value Model**: Optimized key layout for lexicographic ordering and range operations
4. **Dense vs Sparse Indexing**: Hybrid approach with dense sequence indexes and sparse attribute indexes

## Key Schema Design

### Primary Storage Layout

```
Events:    E/<sequence> → EventRecord (binary encoded)
Transfers: T/<sequence> → TransferRecord (binary encoded)
```

### Complete Index Structure

```
Event Indexes:
- EA/<address>/<sequence> → ∅     (Address index)
- ET0/<topic>/<sequence> → ∅      (Topic 0 index)
- ET1/<topic>/<sequence> → ∅      (Topic 1 index)  
- ET2/<topic>/<sequence> → ∅      (Topic 2 index)
- ET3/<topic>/<sequence> → ∅      (Topic 3 index)
- ET4/<topic>/<sequence> → ∅      (Topic 4 index)

Transfer Indexes:
- TS/<sender>/<sequence> → ∅      (Sender index)
- TR/<recipient>/<sequence> → ∅   (Recipient index)
- TO/<txOrigin>/<sequence> → ∅    (Transaction origin index)

Dense Sequence Indexes:
- ES/<sequence> → ∅               (Event sequence index)
- TSX/<sequence> → ∅              (Transfer sequence index)
```

### Sequence Encoding (`sequence.go`)

The sequence is a **63-bit value** encoding chronological order:

```go
// Bit allocation: [28 bits: blockNum][15 bits: txIndex][20 bits: logIndex]
// Max values: 268M blocks, 32K txs/block, 1M logs/tx
sequence = (blockNum << 43) | (txIndex << 20) | logIndex
```

**Critical Design Feature**: Uses **big-endian encoding** to ensure lexicographic key order matches chronological order, enabling efficient range queries.

### Key Construction (`keys.go`)

```go
// Event address key: "EA" + address + sequence
func eventAddressKey(addr thor.Address, seq Sequence) []byte
    
// Event topic key: "ET0" + topic + sequence  
func eventTopicKey(topicPos int, topic thor.Bytes32, seq Sequence) []byte

// Transfer sender key: "TS" + sender + sequence
func transferSenderKey(sender thor.Address, seq Sequence) []byte
```

**Key Format Strategy**:
- **Prefix-based Organization**: Different record types use distinct prefixes
- **Hierarchical Structure**: Keys constructed for optimal range scanning
- **Zero-copy Construction**: Reusable buffers minimize allocations

## Posting-List Optimization

### StreamIterator - Core Streaming Primitive (`stream_iterator.go`)

The `StreamIterator` is the fundamental building block providing:

- **Bounds-only approach**: Relies on PebbleDB's LowerBound/UpperBound for efficiency
- **Bidirectional traversal**: Supports both ASC and DESC iteration  
- **Lazy evaluation**: Values computed on-demand, not pre-loaded
- **Automatic filtering**: Primary iterators filter out index keys inline

```go
type StreamIterator struct {
    iter         *pebble.Iterator
    lowerBound   []byte
    upperBound   []byte
    reverse      bool
    filterPrefix byte // For primary iterators only
}
```

### Boundary Handling

```go
// Precise bound calculation for range queries
lowerBound := eventAddressKey(addr, minSeq)
upperBound := eventAddressKey(addr, maxSeq.Next()) // Exclusive upper bound

// Special handling for MaxSequenceValue overflow
if maxSeq == MaxSequenceValue {
    upperBound = []byte("ET") // Next prefix after "ES"
}
```

### Fast-Path Optimizations

1. **Single-criterion Fast Path**: Bypasses union logic for single address/sender queries
2. **Dense Sequence Indexes**: ES/ and TSX/ provide O(1) access for range-only queries
3. **Prefix Filtering**: Primary iterators distinguish between E/<seq> and EA/<addr>/<seq> keys

### Iterator Types

- **General StreamIterator**: For index-based queries
- **EventPrimaryStreamIterator**: Filters only E/<seq> keys (0x45 prefix)
- **TransferPrimaryStreamIterator**: Filters only T/<seq> keys (0x54 prefix)

## Query Engine

### Query Processing Pipeline (`query_engine.go`)

1. **Criterion Construction**: Each filter criterion becomes a `StreamIntersector`
2. **Stream Building**: Create iterators for each attribute (address, topics, sender, etc.)
3. **Intersection Phase**: AND logic within each criterion using leapfrog intersection
4. **Union Phase**: OR logic across criteria using k-way merge with heap
5. **Materialization**: Convert sequences to actual records

### StreamIntersector - AND Logic (`stream_intersector.go`)

Uses **leapfrog intersection algorithm**:

```go
// Core intersection logic
func (si *StreamIntersector) Next() (Sequence, error) {
    for {
        // Find maximum sequence across all streams
        maxSeq := si.findMaxSequence()
        
        // Advance all streams to this sequence
        if si.advanceAllStreamsTo(maxSeq) {
            return maxSeq, nil // All streams matched
        }
        // Continue if streams don't align
    }
}
```

**Why Leapfrog**: This algorithm minimizes iterator operations by intelligently skipping to potential matches rather than scanning linearly.

### StreamUnion - OR Logic (`stream_union.go`)

Implements **k-way merge with deduplication**:

```go
type StreamUnion struct {
    heap     []*streamUnionItem
    lastSeq  Sequence // For deduplication
    reverse  bool
}
```

- Min-heap (ASC) or max-heap (DESC) for ordering
- Automatic deduplication using `lastSeq` tracking  
- Lazy stream advancement to minimize memory usage

### Advanced Query Optimizations

1. **Single-intersector Fast Path**: Avoids union overhead for simple queries
2. **Context Cancellation**: Integrated cancellation support for long-running queries
3. **Offset/Limit Optimization**: Applied at sequence level before materialization
4. **Exhausted Iterator Detection**: Prevents unnecessary work on completed streams

## Data Flow & Write Path

### Write Path Architecture (`writer.go`)

```
Block → Receipts → Events/Transfers → Sequence Generation → Multi-index Writing
```

### Writer Operations

```go
func (w *Writer) Write(blk *block.Block, receipts tx.Receipts) error {
    for txIndex, receipt := range receipts {
        for logIndex, event := range receipt.Events {
            // 1. Generate sequence
            seq := NewSequence(blk.Header().Number(), txIndex, logIndex)
            
            // 2. Create and encode record
            record := &EventRecord{ /* populate fields */ }
            encodedData := record.Encode()
            
            // 3. Write primary storage
            w.batch.Set(eventKey(seq), encodedData, nil)
            
            // 4. Write all indexes
            w.batch.Set(eventAddressKey(event.Address, seq), nil, nil)
            for i, topic := range event.Topics {
                w.batch.Set(eventTopicKey(i, topic, seq), nil, nil)
            }
            w.batch.Set(eventSequenceKey(seq), nil, nil) // Dense index
        }
    }
}
```

### Binary Encoding Process (`encoding.go`)

The custom binary encoding provides significant improvements over RLP:

```go
type EventRecord struct {
    // Fixed-size fields
    BlockNumber uint32
    TxIndex     uint32
    LogIndex    uint32
    TxOrigin    thor.Address
    Address     thor.Address
    
    // Variable-size fields  
    Topics      []thor.Bytes32
    Data        []byte
}

func (er *EventRecord) Encode() []byte {
    // Header: flags(1) + fieldMask(4) for extensibility
    // Fixed fields in consistent order
    // Variable sections with length prefixes
    // Zero-copy design minimizes allocations
}
```

### Migration Support

Special migration methods preserve original metadata:

```go
func (w *Writer) WriteMigrationEvents(events []*MigrationEventRecord) error
func (w *Writer) WriteMigrationTransfers(transfers []*MigrationTransferRecord) error
```

## Performance Characteristics

### Why 5-10x Faster Than SQLite

#### 1. Elimination of JOIN Operations
- **SQLite**: Complex JOINs between logs, addresses, topics tables
- **PebbleDB**: Direct key-value lookups with posting lists

#### 2. Optimized Storage Layout  
- **SQLite**: Row-based storage with B-tree indexes
- **PebbleDB**: LSM-tree optimized for sequential writes and range queries

#### 3. Streaming Architecture
- **SQLite**: Loads full result sets into memory
- **PebbleDB**: Streams results with minimal memory footprint

#### 4. Binary vs RLP Encoding
- 20-30% smaller storage footprint
- Faster encoding/decoding without reflection
- Schema evolution support with field masks

### Specific Performance Optimizations

#### Memory Management (`materialization.go`)
```go
// Object pools for record reuse
var eventRecordPool = sync.Pool{
    New: func() any { return &EventRecord{} },
}

func getEventRecord() *EventRecord {
    return eventRecordPool.Get().(*EventRecord)
}
```

#### Key Construction Optimization  
```go
// Reusable buffer for key construction
keyBuffer := make([]byte, 64) // Pre-allocated buffer
// Reset and reuse instead of new allocations
```

#### Batch Operations
- Configurable batch sizes for write optimization
- Chunked truncation (10K items) prevents massive batches  
- Strategic commit points during bulk operations

#### Iterator Efficiency
- **Bounds-only approach**: Leverages PebbleDB's native bounds
- **Lazy evaluation**: Computes values only when needed
- **Exhaustion detection**: Stops iteration early when possible
- **Context integration**: Respects cancellation for long queries

### Debug Metrics (`metrics_debug.go`)

Built-in metrics track performance:

```go
type DebugMetrics struct {
    IteratorNextCount   int64
    IteratorSeekCount   int64  
    HeapPushCount      int64
    HeapPopCount       int64
}
```

Enable with build tag: `go build -tags=debug`

## Development Guide

### Configuration Options

```go
type Options struct {
    // Bulk load mode optimizations
    DisableWAL   bool
    MemtableSize int
    
    // Metrics collection  
    EnableMetrics bool
}
```

**Two operational modes**:
- **Default**: Optimized for normal operations (32MB memtable)
- **Bulk Load**: Optimized for migration (128MB memtable, disabled WAL)

### Testing Infrastructure

The implementation includes comprehensive tests:

- **Unit Tests** (`pebbledb_test.go`): Core functionality
- **Encoding Tests** (`encoding_test.go`): Binary encoding/decoding
- **Benchmark Tests** (`benchmark_test.go`): Performance validation
- **Integration Tests**: Full query scenarios

### Adding New Index Types

To add a new index (e.g., for a new event field):

1. **Define key format** in `keys.go`:
```go
func eventNewFieldKey(field SomeType, seq Sequence) []byte
```

2. **Update writer** in `writer.go`:
```go
// In Write method
w.batch.Set(eventNewFieldKey(record.NewField, seq), nil, nil)
```

3. **Add query support** in `query_engine.go`:
```go
// Create iterator for new field filter
if criteria.NewField != nil {
    streams = append(streams, w.createNewFieldIterator(*criteria.NewField, ...))
}
```

### Debugging Query Performance

Enable debug metrics and analyze:

```go
// Check iterator efficiency
fmt.Printf("Iterator ops: %d Next, %d Seek\n", 
    metrics.IteratorNextCount, metrics.IteratorSeekCount)
    
// Check intersection efficiency  
fmt.Printf("Heap ops: %d Push, %d Pop\n",
    metrics.HeapPushCount, metrics.HeapPopCount)
```

High Seek/Next ratios may indicate inefficient query patterns.

## Migration from SQLite

The migration process is handled by the `logsdb/migrate` package, which:

1. **Preserves Original Sequences**: Maintains chronological order from SQLite
2. **Bulk Load Optimization**: Uses optimized PebbleDB settings
3. **Data Verification**: Ensures migration integrity
4. **Progress Tracking**: Provides detailed migration statistics


---

The architecture specifically targets blockchain query patterns (address-based filtering, chronological ordering, large result sets) while maintaining full compatibility with the existing `logsdb.LogsDB` interface.