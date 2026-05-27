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
		HTTP     HTTP
		Postgres Postgres
		Redis    Redis
		Service  Service
		Log      Log
	}
	Service struct {
		LoadTimeout time.Duration `env:"SERVICE_LOAD_TIMEOUT" envDefault:"1s"`
	}
	HTTP struct {
		Host            string        `env:"HTTP_HOST"`
		Port            int           `env:"HTTP_PORT" envDefault:"7000"`
		InternalPort    int           `env:"HTTP_INTERNAL_PORT" envDefault:"8081"`
		ReadTimeout     time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"5s"`
		WriteTimeout    time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"10s"`
		IdleTimeout     time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"120s"`
		ShutdownTimeout time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"10s"`
		RequestTimeout  time.Duration `env:"HTTP_REQUEST_TIMEOUT" envDefault:"2s"`

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
		MaxOpenConns    int           `env:"PG_MAX_OPEN_CONNS" envDefault:"25"`
		MaxIdleConns    int           `env:"PG_MAX_IDLE_CONNS" envDefault:"5"`
		ConnMaxLifetime time.Duration `env:"PG_CONN_MAX_LIFETIME" envDefault:"30m"`
	}
	Redis struct {
		Host     string        `env:"REDIS_HOST,required"`
		Port     int           `env:"REDIS_PORT,required"`
		Password Secret        `env:"REDIS_PASSWORD,required,unset"`
		DB       int           `env:"REDIS_DB" envDefault:"0"`
		TTL      time.Duration `env:"REDIS_CACHE_TTL" envDefault:"10s"`
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

func (r Redis) Address() string {
	return net.JoinHostPort(r.Host, strconv.Itoa(r.Port))
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
