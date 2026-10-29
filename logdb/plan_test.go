// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package logdb

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vechain/thor/v2/block"
	"github.com/vechain/thor/v2/thor"
	"github.com/vechain/thor/v2/tx"
)

const scanPlan = "USING INTEGER PRIMARY KEY (rowid>? AND rowid<?)"

func TestEventQueryPlanPerCriteriaShape(t *testing.T) {
	db, err := NewMem()
	require.NoError(t, err)
	defer db.Close()

	addr := randAddress()
	t0, t1, t2, t3, t4 := randBytes32(), randBytes32(), randBytes32(), randBytes32(), randBytes32()
	crit := func(a *thor.Address, topics ...*thor.Bytes32) *EventCriteria {
		c := &EventCriteria{Address: a}
		copy(c.Topics[:], topics)
		return c
	}
	var six []*EventCriteria
	for range 6 {
		x := randBytes32()
		six = append(six, crit(nil, &x))
	}

	narrow := &Range{From: 100, To: 100}
	wide := &Range{From: 0, To: 25_000_000}

	type expect struct{ narrow, wide, nilRange string }
	shapes := []struct {
		name string
		set  []*EventCriteria
		exp  expect
	}{
		{"topic0", []*EventCriteria{crit(nil, &t0)}, expect{scanPlan, "INDEX event_i1 ", "INDEX event_i1 "}},
		{"six topic0", six, expect{scanPlan, "MULTI-INDEX OR", "MULTI-INDEX OR"}},
		{"address", []*EventCriteria{crit(&addr)}, expect{"INDEX event_i0 ", "INDEX event_i0 ", "INDEX event_i0 "}},
		{"address+topic0", []*EventCriteria{crit(&addr, &t0)}, expect{"INDEX event_i1 ", "INDEX event_i1 ", "INDEX event_i1 "}},
		{"topic1", []*EventCriteria{crit(nil, nil, &t1)}, expect{scanPlan, "INDEX event_i2 ", "INDEX event_i2 "}},
		{"topic4", []*EventCriteria{crit(nil, nil, nil, nil, nil, &t4)}, expect{scanPlan, "INDEX event_i5 ", "INDEX event_i5 "}},
		{"topic0+topic1", []*EventCriteria{crit(nil, &t0, &t1)}, expect{scanPlan, "INDEX event_i2 ", "INDEX event_i2 "}},
		{"topic0+topic2", []*EventCriteria{crit(nil, &t0, nil, &t2)}, expect{scanPlan, "INDEX event_i3 ", "INDEX event_i3 "}},
		{"topic0+topic3", []*EventCriteria{crit(nil, &t0, nil, nil, &t3)}, expect{scanPlan, "INDEX event_i4 ", "INDEX event_i4 "}},
		{"address+topic0+topic1", []*EventCriteria{crit(&addr, &t0, &t1)}, expect{"INDEX event_i2 ", "INDEX event_i2 ", "INDEX event_i2 "}},
		{"mixed", []*EventCriteria{crit(nil, &t0), crit(&addr, &t0)}, expect{scanPlan, "MULTI-INDEX OR", "MULTI-INDEX OR"}},
	}

	plan := func(t *testing.T, set []*EventCriteria, r *Range) string {
		sub, args, err := buildEventFilterSubQuery(&EventFilter{CriteriaSet: set, Range: r}, narrowRange(r))
		require.NoError(t, err)
		rows, err := db.readDB.Query("EXPLAIN QUERY PLAN "+sub, args...)
		require.NoError(t, err)
		defer rows.Close()
		var details []string
		for rows.Next() {
			var id, parent, notused int
			var detail string
			require.NoError(t, rows.Scan(&id, &parent, &notused, &detail))
			details = append(details, detail)
		}
		require.NoError(t, rows.Err())
		return strings.Join(details, "\n")
	}

	for _, s := range shapes {
		for _, r := range []struct {
			name string
			rng  *Range
			want string
		}{{"narrow", narrow, s.exp.narrow}, {"wide", wide, s.exp.wide}, {"nil", nil, s.exp.nilRange}} {
			t.Run(s.name+"/"+r.name, func(t *testing.T) {
				p := plan(t, s.set, r.rng)
				assert.Contains(t, p, r.want, p)
				if r.want == scanPlan {
					assert.NotContains(t, p, "USE TEMP B-TREE FOR ORDER BY", p)
				}
			})
		}
	}

	t.Run("boundary", func(t *testing.T) {
		set := []*EventCriteria{crit(nil, &t0)}
		p := plan(t, set, &Range{From: 0, To: maxScanBlocks - 1})
		assert.Contains(t, p, scanPlan, p)
		p = plan(t, set, &Range{From: 0, To: maxScanBlocks})
		assert.Contains(t, p, "INDEX event_i1 ", p)
		assert.NotContains(t, p, scanPlan, p)
	})
}

func TestNarrowRange(t *testing.T) {
	for _, c := range []struct {
		name string
		rng  *Range
		want bool
	}{
		{"nil", nil, false},
		{"inverted", &Range{From: 10, To: 5}, false},
		{"narrow", &Range{From: 0, To: maxScanBlocks - 1}, true},
		{"wide", &Range{From: 0, To: maxScanBlocks}, false},
		{"single block", &Range{From: 5, To: 5}, true},
	} {
		t.Run(c.name, func(t *testing.T) { assert.Equal(t, c.want, narrowRange(c.rng)) })
	}
}

func TestEventFilterScanEquivalence(t *testing.T) {
	db, err := NewMem()
	require.NoError(t, err)
	defer db.Close()

	topics := []thor.Bytes32{randBytes32(), randBytes32(), randBytes32()}
	topics1 := []thor.Bytes32{randBytes32(), randBytes32()}
	addrs := []thor.Address{randAddress(), randAddress()}

	b := new(block.Builder).Build()
	for n := range 20 {
		bb := new(block.Builder).ParentID(b.Header().ID())
		var receipts tx.Receipts
		for j := range 4 {
			bb.Transaction(newTx(tx.TypeLegacy))
			k := n*4 + j
			receipts = append(receipts, &tx.Receipt{Outputs: []*tx.Output{{Events: tx.Events{
				{Address: addrs[k%2], Topics: []thor.Bytes32{topics[k%3], topics1[k%2]}, Data: []byte{1}},
				{Address: addrs[(k+1)%2], Topics: []thor.Bytes32{topics[(k+1)%3], topics1[(k+1)%2]}, Data: []byte{2}},
			}}}})
		}
		b = bb.Build()
		w := db.NewWriter()
		require.NoError(t, w.Write(b, receipts))
		require.NoError(t, w.Commit())
	}

	type key [3]uint32
	crit := func(a *thor.Address, ts ...*thor.Bytes32) *EventCriteria {
		c := &EventCriteria{Address: a}
		copy(c.Topics[:], ts)
		return c
	}

	// index path: the same subquery with the range scan disabled
	indexPath := func(f *EventFilter) []key {
		sub, args, err := buildEventFilterSubQuery(f, false)
		require.NoError(t, err)
		require.NotContains(t, sub, "+topic")
		rows, err := db.readDB.Query(sub, args...)
		require.NoError(t, err)
		defer rows.Close()
		var out []key
		for rows.Next() {
			var s sequence
			require.NoError(t, rows.Scan(&s))
			out = append(out, key{s.BlockNumber(), s.TxIndex(), s.LogIndex()})
		}
		require.NoError(t, rows.Err())
		return out
	}

	sets := map[string][]*EventCriteria{
		"topic0":        {crit(nil, &topics[0])},
		"two topic0":    {crit(nil, &topics[1]), crit(nil, &topics[2])},
		"three topic0":  {crit(nil, &topics[0]), crit(nil, &topics[1]), crit(nil, &topics[2])},
		"topic0+topic1": {crit(nil, &topics[0], &topics1[0])},
		"topic1":        {crit(nil, nil, &topics1[1])},
		"mixed":         {crit(nil, &topics[0]), crit(&addrs[0], &topics[1])},
	}
	for name, set := range sets {
		for _, order := range []Order{ASC, DESC} {
			for _, rng := range []Range{{5, 5}, {3, 12}, {0, 100}} {
				for _, opt := range []Options{{0, 1000}, {2, 5}, {1000, 5}} {
					f := &EventFilter{CriteriaSet: set, Range: &rng, Options: &opt, Order: order}
					t.Run(fmt.Sprintf("%s/%v/%v/%v", name, order, rng, opt), func(t *testing.T) {
						require.True(t, narrowRange(f.Range))
						sub, _, err := buildEventFilterSubQuery(f, true)
						require.NoError(t, err)
						require.Contains(t, sub, "+topic")

						evs, err := db.FilterEvents(context.Background(), f)
						require.NoError(t, err)
						var got []key
						for _, e := range evs {
							got = append(got, key{e.BlockNumber, e.TxIndex, e.LogIndex})
						}
						assert.Equal(t, indexPath(f), got)
						if rng == (Range{0, 100}) && opt == (Options{0, 1000}) {
							assert.NotEmpty(t, got)
						}
					})
				}
			}
		}
	}
}
