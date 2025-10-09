package client

import (
	"crypto"
	"hash"
	"lukechampine.com/blake3"
	"strconv"
)

type Hasher uint

const (
	MD5 Hasher = 1 + iota
	SHA1
	SHA256
	SHA384
	SHA512
	BLAKE3
	maxHash
)

var hashers = make([]func() hash.Hash, maxHash)

func (h Hasher) New() hash.Hash {
	if h > 0 && h < maxHash {
		f := hashers[h]
		if f != nil {
			return f()
		}
	}
	panic("hasher: requested hash function #" + strconv.Itoa(int(h)) + " is unavailable")
}

func RegisterHash(h Hasher, f func() hash.Hash) {
	if h >= maxHash {
		panic("hasher: RegisterHash of unknown hash function")
	}
	hashers[h] = f
}

func init() {
	RegisterHash(MD5, crypto.MD5.New)
	RegisterHash(SHA1, crypto.SHA1.New)
	RegisterHash(SHA256, crypto.SHA256.New)
	RegisterHash(SHA384, crypto.SHA256.New)
	RegisterHash(SHA512, crypto.SHA512.New)
	RegisterHash(BLAKE3, func() hash.Hash {
		return blake3.New(32, nil)
	})
}
