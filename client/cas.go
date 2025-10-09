package client

import (
	"context"
	reapi "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	"google.golang.org/grpc"
	"io"
)

func FetchTree(d bfpb.Digest, i map[string]*reapi.Directory, c *grpc.ClientConn) error {
	cas := reapi.NewContentAddressableStorageClient(c)
	nt := "initial"

	hashFn := HasherFromDigestFunction(d.DigestFunction)

	for t := ""; nt != ""; t = nt {
		rootDigest := FromDigest(d)
		gtc, err := cas.GetTree(context.Background(), &reapi.GetTreeRequest{
			// needs instance name
			RootDigest: &rootDigest,
			// default page size
			PageToken:      t,
			DigestFunction: d.DigestFunction,
		})

		if err != nil {
			return err
		}

		for {
			gtr, err := gtc.Recv()
			if err == io.EOF {
				break
			}
			for _, dir := range gtr.Directories {
				// compute digest of directory
				dirDigest, err := DigestFromMessage(dir, hashFn)
				if err != nil {
					return err
				}

				// insert into map
				i[DigestString(dirDigest)] = dir
			}
			nt = gtr.NextPageToken
		}
	}
	return nil
}
