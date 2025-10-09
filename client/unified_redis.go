package client

import (
	"context"
	redis "github.com/redis/go-redis/v9"
	"time"
)

type UnifiedRedis struct {
	client  *redis.Client
	cluster *redis.ClusterClient
}

func (r *UnifiedRedis) connect(host string) {
	cluster := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        []string{host},
		Password:     "",
		DialTimeout:  time.Second,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	})
	if cluster.ClusterInfo(context.Background()).Err() != nil {
		cluster.Close()
		r.cluster = nil
		r.client = redis.NewClient(&redis.Options{
			Addr:         host,
			Password:     "",
			DialTimeout:  time.Second,
			ReadTimeout:  time.Second,
			WriteTimeout: time.Second,
		})
	} else {
		r.cluster = cluster
		r.client = nil
	}
}

func (r *UnifiedRedis) ZCard(ctx context.Context, key string) *redis.IntCmd {
	if r.client != nil {
		return r.client.ZCard(ctx, key)
	}
	return r.cluster.ZCard(ctx, key)
}

func (r *UnifiedRedis) LLen(ctx context.Context, key string) *redis.IntCmd {
	if r.client != nil {
		return r.client.LLen(ctx, key)
	}
	return r.cluster.LLen(ctx, key)
}

func (r *UnifiedRedis) HLen(ctx context.Context, key string) *redis.IntCmd {
	if r.client != nil {
		return r.client.HLen(ctx, key)
	}
	return r.cluster.HLen(ctx, key)
}

func (r *UnifiedRedis) ZRange(ctx context.Context, key string, start, stop int64) *redis.StringSliceCmd {
	if r.client != nil {
		return r.client.ZRange(ctx, key, start, stop)
	}
	return r.cluster.ZRange(ctx, key, start, stop)
}

func (r *UnifiedRedis) LRange(ctx context.Context, key string, start, stop int64) *redis.StringSliceCmd {
	if r.client != nil {
		return r.client.LRange(ctx, key, start, stop)
	}
	return r.cluster.LRange(ctx, key, start, stop)
}

func (r *UnifiedRedis) HScan(ctx context.Context, key string, cursor uint64, match string, count int64) *redis.ScanCmd {
	if r.client != nil {
		return r.client.HScan(ctx, key, cursor, match, count)
	}
	return r.cluster.HScan(ctx, key, cursor, match, count)
}

func (r *UnifiedRedis) ClusterShards(ctx context.Context) *redis.ClusterShardsCmd {
	if r.client != nil {
		return r.client.ClusterShards(ctx)
	}
	return r.cluster.ClusterShards(ctx)
}
