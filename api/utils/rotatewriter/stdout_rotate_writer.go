// Copyright (c) 2024 The VeChainThor developers

// Distributed under the GNU Lesser General Public License v3.0 software license, see the accompanying
// file LICENSE or <https://www.gnu.org/licenses/lgpl-3.0.html>

package rotatewriter

import "fmt"

type StdoutWriterImpl struct {
}

func (s StdoutWriterImpl) Start() error {
	return nil
}

func (s StdoutWriterImpl) Write(p []byte) (int, error) {
	fmt.Println(string(p))
	return len(p), nil
}

func StdoutWriter() RotateWriter { // TODO add in io streams
	return &StdoutWriterImpl{}
}
