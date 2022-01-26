package envoy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

const (
	ownerRW              = os.FileMode(0o600)
	ownerRX              = os.FileMode(0o500)
	maxExpandedEnvoySize = 1 << 30
)

type hashReader struct {
	hash.Hash
	r io.Reader
}

func (hr *hashReader) Read(p []byte) (n int, err error) {
	n, err = hr.r.Read(p)
	_, _ = hr.Write(p[:n])
	return n, err
}

func extract(dstName string) (err error) {
	checksum, err := hex.DecodeString(strings.Fields(rawChecksum)[0])
	if err != nil {
		return err
	}

	hr := &hashReader{
		Hash: sha256.New(),
		r:    bytes.NewReader(rawBinary),
	}

	dst, err := os.OpenFile(dstName, os.O_CREATE|os.O_WRONLY, ownerRX)
	if err != nil {
		return err
	}
	defer func() { err = dst.Close() }()

	if _, err = io.Copy(dst, io.LimitReader(hr, maxExpandedEnvoySize)); err != nil {
		return err
	}

	sum := hr.Sum(nil)
	if !bytes.Equal(sum, checksum) {
		return fmt.Errorf("expected %x, got %x checksum", checksum, sum)
	}
	return nil
}
