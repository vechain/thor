// Copyright (c) 2026 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package main

import (
	"fmt"
	"strings"
	"time"
)

const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
)

// submitColWidth is the visible-character width reserved for the submit
// column so the receipt column aligns across rows regardless of whether
// submit prints "OK" (2 chars) or "ERR(<detail>)" (up to ~45).
const submitColWidth = 45

func colorize(code, s string) string { return code + s + ansiReset }

// padColored wraps text in an ANSI color code and right-pads the result to
// `width` VISIBLE columns. fmt.Printf's %-*s counts bytes, not runes, so ANSI
// escape sequences would otherwise consume the padding budget and visibly
// shift the next column leftward.
func padColored(text, colorCode string, width int) string {
	colored := colorize(colorCode, text)
	pad := max(width-len(text), 0)
	return colored + strings.Repeat(" ", pad)
}

// Row captures everything printed for one (spec, submit, receipt) triple.
type Row struct {
	Spec    Spec
	Submit  SubmitResult
	Receipt ReceiptResult
}

// shortHex returns 0xabc...def0 for any hex longer than 13 chars; original otherwise.
func shortHex(s string) string {
	if len(s) <= 13 {
		return s
	}
	return s[:7] + "..." + s[len(s)-4:]
}

// PrintBatchHeader prints the batch number and a column header line.
func PrintBatchHeader(batch int, t time.Time) {
	fmt.Printf("\n=== Batch %d @ %s ===\n", batch, t.UTC().Format(time.RFC3339))
	fmt.Println("type    clauses   path   txid                                                           submit       receipt")
}

// PrintRow prints one row with ANSI colors:
//   - submit OK    → green "OK"
//   - submit ERR   → red "ERR(<detail>)"
//   - receipt OK   → green "OK(blk=<n>)"
//   - receipt MISS → yellow "MISS" (not yet mined — often transient)
//   - receipt ERR  → red "ERR(<detail>)"
//
// The txid is truncated to 6-hex-prefix+"..."+4-hex-suffix.
func PrintRow(r Row) {
	var typeStr string
	switch r.Spec.Type {
	case 0x00:
		typeStr = "0x00"
	case 0x51:
		typeStr = "0x51"
	case 0x02:
		typeStr = "0x02"
	default:
		typeStr = fmt.Sprintf("0x%02x", r.Spec.Type)
	}

	txid := "—"
	if r.Submit.TxID != "" {
		txid = shortHex(r.Submit.TxID)
	}

	var submitCol string
	if r.Submit.Err != nil {
		submitCol = padColored(fmt.Sprintf("ERR(%s)", r.Submit.Err.Error()), ansiRed, submitColWidth)
	} else {
		submitCol = padColored("OK", ansiGreen, submitColWidth)
	}

	var receiptCol string
	switch {
	case r.Receipt.Err != nil:
		receiptCol = padColored(fmt.Sprintf("ERR(%s)", r.Receipt.Err.Error()), ansiRed, 0)
	case !r.Receipt.Found:
		receiptCol = padColored("MISS", ansiYellow, 0)
	default:
		receiptCol = padColored(fmt.Sprintf("OK(blk=%d)", r.Receipt.Block), ansiGreen, 0)
	}

	fmt.Printf("%-7s %-9d %-6s %-62s %s %s\n",
		typeStr, r.Spec.Clauses, strings.ToUpper(r.Spec.Path), txid, submitCol, receiptCol)
}

// PrintSummary tallies submit/receipt OK counts, errors, and whether the
// V1 invariant holds for this batch (every (Type, Clauses) combo had BOTH
// REST and RPC submit==OK).
func PrintSummary(rows []Row) {
	submitOK, receiptOK, errors := 0, 0, 0
	for _, r := range rows {
		if r.Submit.Err == nil {
			submitOK++
		} else {
			errors++
		}
		if r.Receipt.Err == nil && r.Receipt.Found {
			receiptOK++
		}
		if r.Receipt.Err != nil {
			errors++
		}
	}

	// V1 invariant: every (Type, Clauses) tuple must have submit==OK on BOTH "rest" and "rpc" paths.
	v1 := "HOLDS"
	byKey := map[string]map[string]bool{} // key="type/clauses" -> path -> true
	for _, r := range rows {
		key := fmt.Sprintf("%d/%d", r.Spec.Type, r.Spec.Clauses)
		if byKey[key] == nil {
			byKey[key] = map[string]bool{}
		}
		if r.Submit.Err == nil {
			byKey[key][r.Spec.Path] = true
		}
	}
	for _, paths := range byKey {
		if !paths["rest"] || !paths["rpc"] {
			v1 = colorize(ansiRed, "BROKEN")
			break
		}
	}
	if v1 == "HOLDS" {
		v1 = colorize(ansiGreen, "HOLDS")
	}

	fmt.Printf("=== summary: submit %d/%d  receipt %d/%d  errors %d  invariant V1: %s ===\n",
		submitOK, len(rows), receiptOK, len(rows), errors, v1)
}
