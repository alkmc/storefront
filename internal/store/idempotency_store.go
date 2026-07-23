package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strconv"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PurgeIdempotencyKeys deletes rows past their TTL and reports how many went.
func (pg *Postgres) PurgeIdempotencyKeys(ctx context.Context) (int64, error) {
	tag, err := pg.pool.Exec(ctx, queryPurgeIdempotency)
	if err != nil {
		return 0, mapDBError(err)
	}
	return tag.RowsAffected(), nil
}

// createOrderIdempotent claims the key before the decrement so a duplicate replays instead of buying twice.
func (pg *Postgres) createOrderIdempotent(
	ctx context.Context, o domain.Order, idem domain.IdempotencyKey,
) (domain.Order, bool, error) {
	var replayed bool
	err := pg.withTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(
			ctx, queryInsertIdempotency,
			uuid.UUID(o.UserID), string(idem), hashOrderRequest(o), pg.idempotencyTTL.Seconds(),
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			o, replayed, err = replayIdempotent(ctx, tx, o, idem)
			return err
		}

		if o, err = purchaseTx(ctx, tx, o); err != nil {
			return err
		}
		return updateIdempotencyResult(ctx, tx, uuid.UUID(o.UserID), string(idem), uuid.UUID(o.ID))
	})
	if err != nil {
		return domain.Order{}, false, err
	}
	return o, replayed, nil
}

// replayIdempotent loads the stored order for a conflicting key by joining orders on order_id,
// rejecting a reused key with a different payload. The conflicting row is already committed.
func replayIdempotent(
	ctx context.Context, tx pgx.Tx, o domain.Order, idem domain.IdempotencyKey,
) (domain.Order, bool, error) {
	// fingerprint the caller's payload before the scan overwrites o with the stored order
	reqHash := hashOrderRequest(o)
	var (
		hash    []byte
		orderID uuid.UUID
		pid     uuid.UUID
	)
	if err := tx.QueryRow(
		ctx, querySelectIdempotency, uuid.UUID(o.UserID), string(idem),
	).Scan(
		&hash, &orderID, &pid, &o.Quantity,
		&o.UnitPrice.MinorAmount, &o.UnitPrice.Currency, &o.CreatedAt,
	); err != nil {
		return domain.Order{}, false, err
	}
	if !bytes.Equal(hash, reqHash) {
		return domain.Order{}, false, domain.ErrIdempotencyMismatch
	}
	o.ID = domain.OrderID(orderID)
	o.ProductID = domain.ProductID(pid)
	return o, true, nil
}

// updateIdempotencyResult links the stored key to the order it produced.
func updateIdempotencyResult(
	ctx context.Context, tx pgx.Tx, userID uuid.UUID, key string, orderID uuid.UUID,
) error {
	_, err := tx.Exec(ctx, queryUpdateIdempotencyResult, userID, key, orderID)
	return err
}

// hashOrderRequest fingerprints the order payload so a reused key with a different body is caught.
func hashOrderRequest(o domain.Order) []byte {
	sum := sha256.Sum256([]byte(o.ProductID.String() + "|" + strconv.FormatInt(o.Quantity, 10)))
	return sum[:]
}
