// Copyright (c) 2018 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package runtime

import (
	"crypto/ecdsa"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"

	"github.com/ethereum/go-ethereum/crypto"
	ethlog "github.com/ethereum/go-ethereum/log"
	"github.com/mattn/go-isatty"
	"github.com/vechain/thor/v2/log"
)

func initLogger(lvl int) *slog.LevelVar {
	logLevel := log.FromLegacyLevel(lvl)
	var level slog.LevelVar
	level.Set(logLevel)
	output := io.Writer(os.Stdout)
	useColor := (isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())) && os.Getenv("TERM") != "dumb"
	handler := log.NewTerminalHandlerWithLevel(output, &level, useColor)
	log.SetDefault(log.NewLogger(handler))
	ethlog.Root().SetHandler(&ethLogger{
		logger: log.WithContext("pkg", "geth"),
	})

	return &level
}

type ethLogger struct {
	logger log.Logger
}

func (h *ethLogger) Log(r *ethlog.Record) error {
	switch r.Lvl {
	case ethlog.LvlCrit:
		h.logger.Crit(r.Msg)
	case ethlog.LvlError:
		h.logger.Error(r.Msg)
	case ethlog.LvlWarn:
		h.logger.Warn(r.Msg)
	case ethlog.LvlInfo:
		h.logger.Info(r.Msg)
	case ethlog.LvlDebug:
		h.logger.Debug(r.Msg)
	case ethlog.LvlTrace:
		h.logger.Trace(r.Msg)
	default:
		break
	}
	return nil
}

func loadOrGenerateKeyFile(keyFile string) (key *ecdsa.PrivateKey, err error) {
	if !filepath.IsAbs(keyFile) {
		if keyFile, err = filepath.Abs(keyFile); err != nil {
			return nil, err
		}
	}

	// try to load from file
	if key, err = crypto.LoadECDSA(keyFile); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		return key, nil
	}

	// no such file, generate new key and write in
	key, err = crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	if err := crypto.SaveECDSA(keyFile, key); err != nil {
		return nil, err
	}
	return key, nil
}

func defaultKeyFile() string {
	return filepath.Join(mustHomeDir(), ".thor-disco.key")
}

func mustHomeDir() string {
	// try to get HOME env
	if home := os.Getenv("HOME"); home != "" {
		return home
	}

	if user, err := user.Current(); err == nil {
		if user.HomeDir != "" {
			return user.HomeDir
		}
	}

	return filepath.Base(os.Args[0])
}

func readIntFromUInt64Flag(val uint64) (int, error) {
	i := int(val)

	if i < 0 {
		return 0, fmt.Errorf("invalid value %d ", val)
	}

	return i, nil
}
