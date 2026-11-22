package cache

import (
	"context"
	"crypto/rand"
	"encoding/json/v2"
	"fmt"
	"time"
	"uuid"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/valkey-io/valkey-go"
)

const (
	tagPayload   byte = 'p'
	tagTombstone byte = 't'
	tagMissing   byte = 'n'
)

type (
	// Config sets how long products and absences stay cached and how long one Valkey call may take.
	Config struct {
		TTL    time.Duration
		NegTTL time.Duration
		// Timeout caps each call so a hung Valkey falls through to the database.
		Timeout time.Duration
	}
	Valkey struct {
		client valkey.Client
		cfg    Config
	}
	Entry struct {
		Product domain.Product
		Hit     bool
		Found   bool
		// exists tells a missing key from an empty value, whose token may be nil
		exists bool
		token  []byte
	}
	cacheEntry struct {
		ID      string     `json:"id"`
		Name    string     `json:"name"`
		Stock   int64      `json:"stock"`
		Version int64      `json:"version"`
		Price   moneyEntry `json:"price"`
	}
	moneyEntry struct {
		MinorAmount int64           `json:"minorAmount"`
		Currency    domain.Currency `json:"currency"`
	}
)

// New wraps an open Valkey client per the given config.
func New(client valkey.Client, cfg Config) *Valkey {
	return new(Valkey{client: client, cfg: cfg})
}

func (v *Valkey) Set(ctx context.Context, key string, p domain.Product, prev Entry) error {
	payload, err := encodeProduct(p)
	if err != nil {
		return fmt.Errorf("marshal cache value for key %q: %w", key, err)
	}
	return v.guardedSet(ctx, key, payload, v.cfg.TTL, prev)
}

func (v *Valkey) SetMissing(ctx context.Context, key string, prev Entry) error {
	return v.guardedSet(ctx, key, []byte{tagMissing}, v.cfg.NegTTL, prev)
}

func (v *Valkey) Get(ctx context.Context, key string) (Entry, error) {
	raw, err := v.doWithTimeout(ctx, v.client.B().Get().Key(key).Build()).AsBytes()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return Entry{}, nil
		}
		return Entry{}, fmt.Errorf("get cache key %q: %w", key, err)
	}
	return classify(raw), nil
}

func (v *Valkey) Invalidate(ctx context.Context, key string) error {
	cmd := v.client.B().Set().Key(key).
		Value(valkey.BinaryString(newTombstone())).
		PxMilliseconds(v.cfg.TTL.Milliseconds()).
		Build()
	if err := v.doWithTimeout(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("invalidate cache key %q: %w", key, err)
	}
	return nil
}

func (v *Valkey) guardedSet(
	ctx context.Context, key string, value []byte, ttl time.Duration, prev Entry,
) error {
	err := v.doWithTimeout(ctx, v.setCmd(key, value, ttl, prev)).Error()
	if err != nil && !valkey.IsValkeyNil(err) {
		return fmt.Errorf("set cache key %q: %w", key, err)
	}
	return nil
}

func (v *Valkey) doWithTimeout(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	ctx, cancel := context.WithTimeout(ctx, v.cfg.Timeout)
	defer cancel()
	return v.client.Do(ctx, cmd)
}

func (v *Valkey) setCmd(key string, value []byte, ttl time.Duration, prev Entry) valkey.Completed {
	set := v.client.B().Set().Key(key).Value(valkey.BinaryString(value))
	ms := ttl.Milliseconds()
	if !prev.exists {
		return set.Nx().PxMilliseconds(ms).Build()
	}
	return set.Ifeq(valkey.BinaryString(prev.token)).PxMilliseconds(ms).Build()
}

func classify(raw []byte) Entry {
	if len(raw) > 0 {
		switch raw[0] {
		case tagPayload:
			if p, ok := decodeProduct(raw[1:]); ok {
				return Entry{Product: p, Hit: true, Found: true, exists: true}
			}
		case tagMissing:
			return Entry{Hit: true, exists: true}
		}
	}
	return Entry{exists: true, token: raw}
}

func newTombstone() []byte {
	return append([]byte{tagTombstone}, rand.Text()...)
}

func encodeProduct(p domain.Product) ([]byte, error) {
	raw, err := json.Marshal(cacheEntry{
		ID:      p.ID.String(),
		Name:    p.Name,
		Stock:   p.Stock,
		Version: p.Version,
		Price: moneyEntry{
			MinorAmount: p.Price.MinorAmount,
			Currency:    p.Price.Currency,
		},
	})
	if err != nil {
		return nil, err
	}
	return append([]byte{tagPayload}, raw...), nil
}

func decodeProduct(raw []byte) (domain.Product, bool) {
	var e cacheEntry
	if err := json.Unmarshal(raw, &e); err != nil {
		return domain.Product{}, false
	}
	id, err := uuid.Parse(e.ID)
	if err != nil {
		return domain.Product{}, false
	}
	return domain.Product{
		ID:      domain.ProductID(id),
		Name:    e.Name,
		Stock:   e.Stock,
		Version: e.Version,
		Price: domain.Money{
			MinorAmount: e.Price.MinorAmount,
			Currency:    e.Price.Currency,
		},
	}, true
}
