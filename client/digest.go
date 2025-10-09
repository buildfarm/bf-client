package client

import (
	"encoding/hex"
	"fmt"
	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	"github.com/golang/protobuf/proto"
	"strconv"
	"strings"
)

func FromDigest(d bfpb.Digest) reapi.Digest {
	return reapi.Digest{Hash: d.Hash, SizeBytes: d.Size}
}

func ToDigest(d reapi.Digest, df reapi.DigestFunction_Value) bfpb.Digest {
	return bfpb.Digest{Hash: d.Hash, Size: d.SizeBytes, DigestFunction: df}
}

func DigestString(d bfpb.Digest) string {
	// optional prefix
	prefix := ""
	if d.DigestFunction == reapi.DigestFunction_BLAKE3 {
		prefix = "blake3/"
	}
	return fmt.Sprintf("%s%s/%d", prefix, d.Hash, d.Size)
}

func parseDigestFunction(s string) reapi.DigestFunction_Value {
	switch s {
	case "blake3":
		return reapi.DigestFunction_BLAKE3
	}
	return reapi.DigestFunction_UNKNOWN
}

func inferDigestFunction(h string) reapi.DigestFunction_Value {
	switch len(h) / 2 * 8 {
	case 128:
		return reapi.DigestFunction_MD5
	case 160:
		return reapi.DigestFunction_SHA1
	case 256:
		return reapi.DigestFunction_SHA256
	case 384:
		return reapi.DigestFunction_SHA384
	case 512:
		return reapi.DigestFunction_SHA512
	}
	return reapi.DigestFunction_UNKNOWN
}

func ParseDigest(s string) bfpb.Digest {
	c := strings.Split(s, "/")
	var df reapi.DigestFunction_Value
	if len(c) == 3 {
		df = parseDigestFunction(c[0])
		c = c[1:]
	} else {
		df = inferDigestFunction(c[0])
	}
	hash := c[0]
	size, _ := strconv.ParseInt(c[1], 10, 64)
	return bfpb.Digest{
		DigestFunction: df,
		Hash:           hash,
		Size:           size,
	}
}

func HasherFromDigestFunction(df reapi.DigestFunction_Value) Hasher {
	hashers := map[reapi.DigestFunction_Value]Hasher{
		reapi.DigestFunction_MD5:    MD5,
		reapi.DigestFunction_SHA1:   SHA1,
		reapi.DigestFunction_SHA256: SHA256,
		reapi.DigestFunction_SHA384: SHA384,
		reapi.DigestFunction_SHA512: SHA512,
		reapi.DigestFunction_BLAKE3: BLAKE3,
	}

	return hashers[df]
}

func DigestFromBlob(blob []byte, hasher Hasher) bfpb.Digest {
	h := hasher.New()
	h.Write(blob)
	arr := h.Sum(nil)

	dfs := map[Hasher]reapi.DigestFunction_Value{
		MD5:    reapi.DigestFunction_MD5,
		SHA1:   reapi.DigestFunction_SHA1,
		SHA256: reapi.DigestFunction_SHA256,
		SHA384: reapi.DigestFunction_SHA384,
		SHA512: reapi.DigestFunction_SHA512,
		BLAKE3: reapi.DigestFunction_BLAKE3,
	}

	df := dfs[hasher]
	return bfpb.Digest{Hash: hex.EncodeToString(arr[:]), Size: int64(len(blob)), DigestFunction: df}
}

func DigestFromMessage(msg proto.Message, hasher Hasher) (bfpb.Digest, error) {
	blob, err := proto.Marshal(msg)
	if err != nil {
		Empty := DigestFromBlob([]byte{}, hasher)
		return Empty, err
	}
	return DigestFromBlob(blob, hasher), nil
}
