package httpapi

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/ziyue67/tms/go-backend/internal/config"
)

func NewRedis(ctx context.Context, cfg config.Redis) (*redis.Client, error) {
	var options *redis.Options
	var err error
	if cfg.URL != "" {
		options, err = redis.ParseURL(cfg.URL)
		if err != nil {
			return nil, fmt.Errorf("parse REDIS_URL: %w", err)
		}
	} else {
		options = &redis.Options{
			Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
			Password:     cfg.Password,
			DB:           cfg.Database,
			DialTimeout:  cfg.Timeout,
			ReadTimeout:  cfg.Timeout,
			WriteTimeout: cfg.Timeout,
		}
		if cfg.TLS {
			options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
	}
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
