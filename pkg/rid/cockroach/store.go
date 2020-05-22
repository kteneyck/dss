package cockroach

import (
	"context"
	"database/sql"

	"github.com/dpjacques/clockwork"
	"github.com/interuss/dss/pkg/cockroach"
	"github.com/interuss/dss/pkg/logging"
	"github.com/interuss/dss/pkg/rid/repos"
	"go.uber.org/zap"
)

var (
	// DefaultClock is what is used as the Store's clock, returned from Dial.
	DefaultClock = clockwork.NewRealClock()
)

// Store is an implementation of dss.Store using
// Cockroach DB as its backend store.
type Store struct {
	ISA          repos.ISA
	Subscription repos.Subscription
	*cockroach.DB
}

func recoverRollbackRepanic(ctx context.Context, tx *sql.Tx) {
	if p := recover(); p != nil {
		if err := tx.Rollback(); err != nil {
			logging.WithValuesFromContext(ctx, logging.Logger).Error(
				"failed to rollback transaction", zap.Error(err),
			)
		}
	}
}

// NewStore returns a Store instance connected to a cockroach instance via db.
func NewStore(db *cockroach.DB, logger *zap.Logger) (*Store, error) {
	return &Store{
		ISA:          &ISAStore{db, DefaultClock, logger},
		Subscription: &SubscriptionStore{db, DefaultClock, logger},
		DB:           db,
	}, nil
}

// Close closes the underlying DB connection.
func (s *Store) Close() error {
	return s.DB.Close()
}

// Bootstrap bootstraps the underlying database with required tables.
//
// TODO: We should handle database migrations properly, but bootstrap both us
// *and* the database with this manual approach here.
func (s *Store) Bootstrap(ctx context.Context) error {
	/*
			The following tables correspond to the ASTM Remote ID standard A2.5.2.3:
			a) Cell ID:
					cells_identification_service_areas.cell_id
			 		cells_subscriptions.cell_id
			b) Subscription
				 	i. subscriptions.id
				 ii. subscriptions.owner
				iii. subscriptions.url
				 iv. subscriptions.starts_at and subscriptions.ends_at
				  v. the mapping from cells_subscriptions.subscription_id and cell_id
						 to subscriptions.id
				 vi. subscriptions.notification_index
			c) ISA
		 		 	i. identification_service_areas.id
				 ii. identification_service_areas.owner
				iii. identification_service_areas.url
				 iv. identification_service_areas.starts_at and
						 identification_service_areas.ends_at
				  v. the mapping from
						 cells_identification_service_areas.subscription_id and cell_id
						 to cells_identification_service_areas.id
	*/
	const query = `
	CREATE TABLE IF NOT EXISTS subscriptions (
		id UUID PRIMARY KEY,
		owner STRING NOT NULL,
		url STRING NOT NULL,
		notification_index INT4 DEFAULT 0,
		starts_at TIMESTAMPTZ,
		ends_at TIMESTAMPTZ,
		updated_at TIMESTAMPTZ NOT NULL,
		cells INT64[] NOT NULL CHECK (array_length(cells, 1) > 0 AND array_length(cells, 1) IS NOT NULL),
		INDEX owner_idx (owner),
		INVERTED INDEX cells_idx (cells),
		INDEX starts_at_idx (starts_at),
		INDEX ends_at_idx (ends_at),
		CHECK (starts_at IS NULL OR ends_at IS NULL OR starts_at < ends_at)
	);
	CREATE TABLE IF NOT EXISTS identification_service_areas (
		id UUID PRIMARY KEY,
		owner STRING NOT NULL,
		url STRING NOT NULL,
		starts_at TIMESTAMPTZ,
		ends_at TIMESTAMPTZ,
		updated_at TIMESTAMPTZ NOT NULL,
		cells INT64[] NOT NULL CHECK (array_length(cells, 1) IS NOT NULL),
		INDEX owner_idx (owner),
		INVERTED INDEX cells_idx (cells),
		INDEX starts_at_idx (starts_at),
		INDEX ends_at_idx (ends_at),
		INDEX updated_at_idx (updated_at),
		CHECK (starts_at IS NULL OR ends_at IS NULL OR starts_at < ends_at)
	);
	`
	_, err := s.ExecContext(ctx, query)
	return err
}
