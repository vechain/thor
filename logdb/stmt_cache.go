// Copyright (c) 2020 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"database/sql"
	"sync"
)

// to cache prepared sql statement, which maps query string to stmt.
type stmtCache struct {
	db *sql.DB
	m  sync.Map
}

func newStmtCache(db *sql.DB) *stmtCache {
	return &stmtCache{db: db}
}

func (sc *stmtCache) Prepare(query string) (*sql.Stmt, error) {
	// Fast path: check if already cached
	if cached, ok := sc.m.Load(query); ok {
		return cached.(*sql.Stmt), nil
	}

	// Slow path: prepare new statement
	stmt, err := sc.db.Prepare(query)
	if err != nil {
		return nil, err
	}

	// Use LoadOrStore to handle race condition:
	// If another goroutine stored first, use theirs and close ours
	actual, loaded := sc.m.LoadOrStore(query, stmt)
	if loaded {
		// Another goroutine won the race, close our duplicate
		// Ignore error since not used anymore
		stmt.Close()
	}
	return actual.(*sql.Stmt), nil
}

func (sc *stmtCache) MustPrepare(query string) *sql.Stmt {
	stmt, err := sc.Prepare(query)
	if err != nil {
		panic(err)
	}
	return stmt
}

func (sc *stmtCache) Clear() {
	sc.m.Range(func(k, v any) bool {
		_ = v.(*sql.Stmt).Close()
		sc.m.Delete(k)
		return true
	})
}
