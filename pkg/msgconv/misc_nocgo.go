//go:build !cgo

package msgconv

import "fmt"

func silk2ogg(_ []byte) ([]byte, error) {
	return nil, fmt.Errorf("silk audio decoding requires cgo")
}

func ogg2silk(_ []byte) ([]byte, error) {
	return nil, fmt.Errorf("silk audio encoding requires cgo")
}
