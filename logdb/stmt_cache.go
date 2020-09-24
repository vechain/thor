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
	cached, _ := sc.m.Load(query)
	if cached == nil {
		stmt, err := sc.db.Prepare(query)
		if err != nil {
			return nil, err
		}
		sc.m.Store(query, stmt)
		cached = stmt
	}
	return cached.(*sql.Stmt), nil
}

func (sc *stmtCache) MustPrepare(query string) *sql.Stmt {
	stmt, err := sc.Prepare(query)
	if err != nil {
		panic(err)
	}
	return stmt
}

func (sc *stmtCache) Clear() {
	sc.m.Range(func(k, v interface{}) bool {
		_ = v.(*sql.Stmt).Close()
		sc.m.Delete(k)
		return true
	})
}
