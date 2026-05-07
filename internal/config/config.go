package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"
)

const (
	defaultHTTPHost        = ""
	defaultHTTPPort        = 7000
	defaultReadTimeout     = 5 * time.Second
	defaultWriteTimeout    = 10 * time.Second
	defaultIdleTimeout     = 120 * time.Second
	defaultShutdownTimeout = 10 * time.Second
	defaultRequestTimeout  = 2 * time.Second

	defaultPostgresSSLMode = "disable"

	defaultRedisHost = "localhost"
	defaultRedisPort = 6379
	defaultRedisDB   = 0
	defaultRedisTTL  = 10 * time.Second
)

type Config struct {
	HTTP     HTTP
	Postgres Postgres
	Redis    Redis
}

type HTTP struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	RequestTimeout  time.Duration
}

func (h HTTP) Address() string {
	return net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
}

type Postgres struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

func (p Postgres) DSN() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.User, p.Password),
		Host:   net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		Path:   p.Database,
	}
	q := u.Query()
	q.Set("sslmode", p.SSLMode)
	u.RawQuery = q.Encode()

	return u.String()
}

type Redis struct {
	Host string
	Port int
	DB   int
	TTL  time.Duration
}

func (r Redis) Address() string {
	return net.JoinHostPort(r.Host, strconv.Itoa(r.Port))
}

func Load() (Config, error) {
	return load(os.LookupEnv)
}

type lookupFunc func(string) (string, bool)

func load(lookup lookupFunc) (Config, error) {
	httpCfg, err := loadHTTP(lookup)
	if err != nil {
		return Config{}, err
	}
	pgCfg, err := loadPostgres(lookup)
	if err != nil {
		return Config{}, err
	}
	redisCfg, err := loadRedis(lookup)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTP:     httpCfg,
		Postgres: pgCfg,
		Redis:    redisCfg,
	}, nil
}

func loadHTTP(lookup lookupFunc) (HTTP, error) {
	port, err := portEnv(lookup, "HTTP_PORT", defaultHTTPPort)
	if err != nil {
		return HTTP{}, err
	}
	readTimeout, err := durationEnv(lookup, "HTTP_READ_TIMEOUT", defaultReadTimeout)
	if err != nil {
		return HTTP{}, err
	}
	writeTimeout, err := durationEnv(lookup, "HTTP_WRITE_TIMEOUT", defaultWriteTimeout)
	if err != nil {
		return HTTP{}, err
	}
	idleTimeout, err := durationEnv(lookup, "HTTP_IDLE_TIMEOUT", defaultIdleTimeout)
	if err != nil {
		return HTTP{}, err
	}
	shutdownTimeout, err := durationEnv(lookup, "HTTP_SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return HTTP{}, err
	}
	requestTimeout, err := durationEnv(lookup, "HTTP_REQUEST_TIMEOUT", defaultRequestTimeout)
	if err != nil {
		return HTTP{}, err
	}

	return HTTP{
		Host:            stringEnv(lookup, "HTTP_HOST", defaultHTTPHost),
		Port:            port,
		ReadTimeout:     readTimeout,
		WriteTimeout:    writeTimeout,
		IdleTimeout:     idleTimeout,
		ShutdownTimeout: shutdownTimeout,
		RequestTimeout:  requestTimeout,
	}, nil
}

func loadPostgres(lookup lookupFunc) (Postgres, error) {
	host, err := requiredEnv(lookup, "PG_HOST")
	if err != nil {
		return Postgres{}, err
	}
	port, err := requiredPortEnv(lookup, "PG_PORT")
	if err != nil {
		return Postgres{}, err
	}
	user, err := requiredEnv(lookup, "PG_USER")
	if err != nil {
		return Postgres{}, err
	}
	password, err := requiredEnv(lookup, "PG_PASSWORD")
	if err != nil {
		return Postgres{}, err
	}
	database, err := requiredEnv(lookup, "PG_DB")
	if err != nil {
		return Postgres{}, err
	}

	return Postgres{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		Database: database,
		SSLMode:  stringEnv(lookup, "PG_SSLMODE", defaultPostgresSSLMode),
	}, nil
}

func loadRedis(lookup lookupFunc) (Redis, error) {
	port, err := portEnv(lookup, "REDIS_PORT", defaultRedisPort)
	if err != nil {
		return Redis{}, err
	}
	db, err := intEnv(lookup, "REDIS_DB", defaultRedisDB)
	if err != nil {
		return Redis{}, err
	}
	ttl, err := durationEnv(lookup, "REDIS_CACHE_TTL", defaultRedisTTL)
	if err != nil {
		return Redis{}, err
	}

	return Redis{
		Host: stringEnv(lookup, "REDIS_HOST", defaultRedisHost),
		Port: port,
		DB:   db,
		TTL:  ttl,
	}, nil
}

func stringEnv(lookup lookupFunc, key, fallback string) string {
	if value, ok := lookup(key); ok && value != "" {
		return value
	}
	return fallback
}

func requiredEnv(lookup lookupFunc, key string) (string, error) {
	value, ok := lookup(key)
	if !ok || value == "" {
		return "", fmt.Errorf("environment variable %q is required", key)
	}
	return value, nil
}

func intEnv(lookup lookupFunc, key string, fallback int) (int, error) {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be non-negative", key)
	}
	return value, nil
}

func portEnv(lookup lookupFunc, key string, fallback int) (int, error) {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	return parsePort(key, raw)
}

func requiredPortEnv(lookup lookupFunc, key string) (int, error) {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return 0, fmt.Errorf("environment variable %q is required", key)
	}
	return parsePort(key, raw)
}

func parsePort(key, raw string) (int, error) {
	value, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if value == 0 {
		return 0, fmt.Errorf("parse %s: port out of range", key)
	}
	return int(value), nil
}

func durationEnv(lookup lookupFunc, key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := lookup(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return value, nil
}
