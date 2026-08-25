package store

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/alkmc/storefront/internal/event"
	"github.com/jackc/pgx/v5"
)

type (
	// outboxRow is a pending event awaiting publication.
	outboxRow struct {
		id        int64
		attempts  int32
		createdAt time.Time
		record    event.Record
	}
	// failedRow pairs a row with the publish error that prevented its delivery.
	failedRow struct {
		row outboxRow
		err error
	}
)

// DrainBatch claims, publishes, and settles due outbox rows in one tx, the row locks are the claim.
// The bool reports whether the claim hit batchSize, signalling more rows likely wait.
func (pg *Postgres) DrainBatch(
	ctx context.Context,
	batchSize, maxAttempts int,
	publish func(context.Context, event.Record) error,
) (int, bool, error) {
	tx, err := pg.pool.Begin(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("outbox begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	batch, err := claimOutbox(ctx, tx, batchSize)
	if err != nil || len(batch) == 0 {
		return 0, false, err
	}
	full := len(batch) == batchSize

	sent, failed := publishBatch(ctx, batch, publish)
	bump, dead, transientErr := classifyFailures(failed, maxAttempts)

	if err := deleteOutbox(ctx, tx, sent); err != nil {
		return 0, false, err
	}
	if err := bumpAttempts(ctx, tx, bump); err != nil {
		return 0, false, err
	}
	if err := deadLetter(ctx, tx, dead); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, fmt.Errorf("outbox commit: %w", err)
	}
	return len(sent), full, transientErr
}

// insertOutbox stores one pending event and schedules a relay wakeup within the caller's tx.
func insertOutbox(ctx context.Context, tx pgx.Tx, e event.Event) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	if _, err := tx.Exec(ctx, queryInsertOutbox, e.EventID, e.Type, payload); err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	// NOTIFY is delivered only after commit, so the relay always wakes to a visible row.
	if _, err := tx.Exec(ctx, queryNotifyOutbox); err != nil {
		return fmt.Errorf("notify outbox: %w", err)
	}
	return nil
}

// emitProductEvent builds and stores an outbox event inside the caller's tx.
func emitProductEvent(ctx context.Context, tx pgx.Tx, eventType string, p domain.Product) error {
	e, err := event.New(eventType, p)
	if err != nil {
		return err
	}
	return insertOutbox(ctx, tx, e)
}

// claimOutbox locks and returns up to n due rows, skipping rows another relay holds.
func claimOutbox(ctx context.Context, tx pgx.Tx, n int) ([]outboxRow, error) {
	rows, err := tx.Query(ctx, queryClaimOutbox, n)
	if err != nil {
		return nil, fmt.Errorf("outbox claim: %w", err)
	}
	defer rows.Close()

	var batch []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(
			&row.id, &row.record.MessageID, &row.record.Type,
			&row.record.Payload, &row.attempts, &row.createdAt,
		); err != nil {
			return nil, fmt.Errorf("outbox scan: %w", err)
		}
		batch = append(batch, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox rows: %w", err)
	}
	return batch, nil
}

// publishBatch publishes rows in order, only a transient failure stops the batch (keeps the tx short).
func publishBatch(
	ctx context.Context, batch []outboxRow, publish func(context.Context, event.Record) error,
) (sent []int64, failed []failedRow) {
	for _, row := range batch {
		err := publish(ctx, row.record)
		if err == nil {
			sent = append(sent, row.id)
			continue
		}
		failed = append(failed, failedRow{row: row, err: err})
		if !errors.Is(err, event.ErrUndeliverable) {
			break
		}
	}
	return sent, failed
}

// classifyFailures spares the transient error from the attempts count and returns it.
func classifyFailures(failed []failedRow, maxAttempts int) ([]int64, []failedRow, error) {
	var (
		bump      []int64
		dead      []failedRow
		transient error
	)
	for _, f := range failed {
		switch {
		case !errors.Is(f.err, event.ErrUndeliverable):
			transient = f.err
		case int(f.row.attempts)+1 >= maxAttempts:
			dead = append(dead, f)
		default:
			bump = append(bump, f.row.id)
		}
	}
	return bump, dead, transient
}

// deleteOutbox removes the published rows.
func deleteOutbox(ctx context.Context, tx pgx.Tx, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, queryDeleteOutbox, ids); err != nil {
		return fmt.Errorf("outbox delete: %w", err)
	}
	return nil
}

// bumpAttempts backs off the next retry exponentially and increments the counter.
func bumpAttempts(ctx context.Context, tx pgx.Tx, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, queryBumpOutbox, ids); err != nil {
		return fmt.Errorf("outbox attempts: %w", err)
	}
	return nil
}

// deadLetter moves poison rows to outbox_dead and removes them from outbox.
func deadLetter(ctx context.Context, tx pgx.Tx, rows []failedRow) error {
	ids := make([]int64, 0, len(rows))
	for _, f := range rows {
		r := f.row
		if _, err := tx.Exec(
			ctx, queryDeadOutbox,
			r.id, r.record.MessageID, r.record.Type, r.record.Payload,
			r.attempts+1, r.createdAt, f.err.Error(),
		); err != nil {
			return fmt.Errorf("outbox dead-letter: %w", err)
		}
		ids = append(ids, r.id)
	}
	return deleteOutbox(ctx, tx, ids)
}
