package client

import (
	"context"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"
	"io"
)

func Expect(c *grpc.ClientConn, d bfpb.Digest, m proto.Message) error {
	bs := bytestream.NewByteStreamClient(c)

	bsrc, err := bs.Read(context.Background(), &bytestream.ReadRequest{
		ResourceName: "/blobs/" + DigestString(d),
	})
	if err != nil {
		return err
	}

	var b []byte
	for {
		br, err := bsrc.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if int64(len(br.Data)) == d.Size {
			b = br.Data
		}
		err = proto.Unmarshal(b, m)
		if err != nil {
			return err
		}
		return nil
	}
	return io.EOF
}
