package config

import (
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/caarlos0/env/v11"
)

type (
	Config struct {
		HTTP            HTTP
		GRPC            GRPC
		Postgres        Postgres
		Redis           Redis
		RabbitMQ        RabbitMQ
		Outbox          Outbox
		Service         Service
		Log             Log
		ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
	}
	GRPC struct {
		Host            string        `env:"GRPC_HOST"`
		Port            int           `env:"GRPC_PORT" envDefault:"9090"`
		RequestTimeout  time.Duration `env:"GRPC_REQUEST_TIMEOUT" envDefault:"2s"`
		MaxRequestBytes int64         `env:"GRPC_MAX_REQUEST_BYTES" envDefault:"1048576"` // 1 MiB
		Reflection      bool          `env:"GRPC_REFLECTION" envDefault:"false"`
	}
	Service struct {
		LoadTimeout time.Duration `env:"SERVICE_LOAD_TIMEOUT" envDefault:"1s"`
	}
	HTTP struct {
		Host           string        `env:"HTTP_HOST"`
		Port           int           `env:"HTTP_PORT" envDefault:"7000"`
		InternalPort   int           `env:"HTTP_INTERNAL_PORT" envDefault:"8081"`
		ReadTimeout    time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"5s"`
		WriteTimeout   time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"10s"`
		IdleTimeout    time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"120s"`
		RequestTimeout time.Duration `env:"HTTP_REQUEST_TIMEOUT" envDefault:"2s"`

		MaxBodyBytes     int64 `env:"HTTP_MAX_BODY_BYTES" envDefault:"1048576"` // 1 MiB
		CompressMinBytes int   `env:"HTTP_COMPRESS_MIN_BYTES" envDefault:"1024"`

		CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`
		CORSMaxAge         int      `env:"CORS_MAX_AGE" envDefault:"600"`
		HSTSEnabled        bool     `env:"HSTS_ENABLED" envDefault:"false"`
		HSTSMaxAge         int      `env:"HSTS_MAX_AGE" envDefault:"31536000"`
	}
	Postgres struct {
		Host            string        `env:"PG_HOST,required"`
		Port            int           `env:"PG_PORT,required"`
		User            string        `env:"PG_USER,required"`
		Password        Secret        `env:"PG_PASSWORD,required,unset"`
		Database        string        `env:"PG_DB,required"`
		SSLMode         string        `env:"PG_SSLMODE" envDefault:"disable"`
		MaxOpenConns    int32         `env:"PG_MAX_OPEN_CONNS" envDefault:"25"`
		MaxIdleConns    int32         `env:"PG_MAX_IDLE_CONNS" envDefault:"5"`
		ConnMaxLifetime time.Duration `env:"PG_CONN_MAX_LIFETIME" envDefault:"30m"`
	}
	Redis struct {
		Host     string        `env:"REDIS_HOST,required"`
		Port     int           `env:"REDIS_PORT,required"`
		Password Secret        `env:"REDIS_PASSWORD,required,unset"`
		DB       int           `env:"REDIS_DB" envDefault:"0"`
		TTL      time.Duration `env:"REDIS_CACHE_TTL" envDefault:"5m"`
		NegTTL   time.Duration `env:"REDIS_CACHE_NEG_TTL" envDefault:"1m"`
	}
	RabbitMQ struct {
		Host     string `env:"RABBITMQ_HOST,required"`
		Port     int    `env:"RABBITMQ_PORT,required"`
		User     string `env:"RABBITMQ_USER,required"`
		Password Secret `env:"RABBITMQ_PASSWORD,required,unset"`
	}
	Outbox struct {
		BatchSize      int           `env:"OUTBOX_BATCH_SIZE" envDefault:"25"`
		PollInterval   time.Duration `env:"OUTBOX_POLL_INTERVAL" envDefault:"5s"`
		PublishTimeout time.Duration `env:"OUTBOX_PUBLISH_TIMEOUT" envDefault:"2s"`
		MaxAttempts    int           `env:"OUTBOX_MAX_ATTEMPTS" envDefault:"10"`
	}
	Log struct {
		Level slog.Level `env:"LOG_LEVEL" envDefault:"INFO"`
	}
)

func (h HTTP) Address() string {
	return net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
}

func (h HTTP) InternalAddress() string {
	return net.JoinHostPort(h.Host, strconv.Itoa(h.InternalPort))
}

func (g GRPC) Address() string {
	return net.JoinHostPort(g.Host, strconv.Itoa(g.Port))
}

func (r Redis) Address() string {
	return net.JoinHostPort(r.Host, strconv.Itoa(r.Port))
}

// URL renders an amqp:// address on the default vhost.
func (r RabbitMQ) URL() string {
	u := url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(r.User, r.Password.Reveal()),
		Host:   net.JoinHostPort(r.Host, strconv.Itoa(r.Port)),
		Path:   "/",
	}
	return u.String()
}

func (p Postgres) DSN() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.User, p.Password.Reveal()),
		Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		Path:   p.Database,
	}
	q := u.Query()
	q.Set("sslmode", p.SSLMode)
	u.RawQuery = q.Encode()

	return u.String()
}

func Load() (Config, error) {
	return env.ParseAs[Config]()
}

func LoadPostgres() (Postgres, error) {
	return env.ParseAs[Postgres]()
}
