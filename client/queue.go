package client

import (
	"context"
	"fmt"
	bfpb "github.com/buildfarm/buildfarm/build/buildfarm/v1test"
	"github.com/golang/protobuf/jsonpb"
	redis "github.com/redis/go-redis/v9"
	"regexp"
	"strings"
)

type Queue struct {
	keys []string
}

func slotRangesContainsSlot(slots []redis.SlotRange, n int) bool {
	for _, r := range slots {
		if r.Start <= int64(n) && r.End >= int64(n) {
			return true
		}
	}
	return false
}

func createHash(p string, n int) string {
	return fmt.Sprintf("%s:%d", p, n)
}

func createName(name string, slots []redis.SlotRange) string {
	h := Hash(name)
	var n int
	for n = 0; !slotRangesContainsSlot(slots, Slot(createHash(h, n))); n++ {
	}
	re := regexp.MustCompile(`{[^}]*}`)
	hash := createHash(h, n)
	if re.MatchString(name) {
		return re.ReplaceAllString(name, "{"+createHash(h, n)+"}")
	}
	return fmt.Sprintf("{%s}%s", hash, name)
}

func NewQueue(ctx context.Context, c *UnifiedRedis, name string) *Queue {
	var keys []string
	result := c.ClusterShards(ctx)
	if result.Err() != nil {
		keys = append(keys, name)
	} else {
		shards := result.Val()
		for _, shard := range shards {
			keys = append(keys, createName(name, shard.Slots))
		}
	}
	return &Queue{
		keys: keys,
	}
}

func rlen(ctx context.Context, c *UnifiedRedis, key string) *redis.IntCmd {
	// hacks
	if strings.HasSuffix(key, "_priority") {
		return c.ZCard(ctx, key)
	}
	return c.LLen(ctx, key)
}

func (q *Queue) Length(ctx context.Context, c *UnifiedRedis) (int64, error) {
	var sum int64 = 0
	for _, name := range q.keys {
		len := rlen(ctx, c, name)
		if len.Err() != nil {
			return -1, len.Err()
		}
		sum += len.Val()
	}
	return sum, nil
}

func ParsePrequeueName(json string) (*Operation, error) {
	ee := &bfpb.ExecuteEntry{}
	err := jsonpb.Unmarshal(strings.NewReader(json), ee)
	if err != nil {
		return nil, err
	}
	return &Operation{
		Name:     ee.OperationName,
		Metadata: ee.RequestMetadata,
	}, nil
}

func ParseQueueName(json string) (*Operation, error) {
	qe := &bfpb.QueueEntry{}
	err := jsonpb.Unmarshal(strings.NewReader(json), qe)
	if err != nil {
		return nil, err
	}
	return &Operation{
		Name:     qe.ExecuteEntry.OperationName,
		Metadata: qe.ExecuteEntry.RequestMetadata,
	}, nil
}

func rrange(ctx context.Context, c *UnifiedRedis, key string, start int64, stop int64) ([]string, error) {
	// hacks
	if strings.HasSuffix(key, "_priority") {
		var entries []string
		z := c.ZRange(ctx, key, start, stop)
		for _, e := range z.Val() {
			entries = append(entries, e[strings.Index(e, ":")+1:])
		}
		return entries, z.Err()
	}
	l := c.LRange(ctx, key, start, stop)
	return l.Val(), l.Err()
}

func (q *Queue) Slice(ctx context.Context, c *UnifiedRedis, start int64, stop int64, cb func(string) (*Operation, error)) []*Operation {
	var ops []*Operation
	for _, key := range q.keys {
		if entries, err := rrange(ctx, c, key, start, stop); err == nil {
			for _, entry := range entries {
				name, err := cb(entry)
				if err != nil {
					panic(err)
				}
				ops = append(ops, name)
			}
			stop -= int64(len(ops))
			if stop <= 0 {
				break
			}
		}
	}
	return ops
}
