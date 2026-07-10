package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
	"github.com/hnsx-io/hnsx/server/internal/worker/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const workerDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists worker aggregates to Postgres.
type PostgresRepository struct {
	db *db.DB
}

// NewPostgresRepository constructs a Postgres-backed worker repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	return &PostgresRepository{db: database}
}

// Save implements Repository.
func (r *PostgresRepository) Save(w *model.Worker) error {
	if r.db == nil || r.db.IsNoDB() {
		return errors.New("worker/postgres: no database configured")
	}
	if w == nil || w.ID == "" {
		return model.ErrWorkerNotFound
	}

	var version, region string
	var capabilitiesJSON []byte
	var lastSeen *time.Time
	if w.Info != nil {
		version = w.Info.Version
		region = w.Info.Region
		caps := map[string]any{}
		if w.Info.Capacity != nil {
			caps["capacity"] = w.Info.Capacity
		}
		if len(w.Info.Labels) > 0 {
			caps["labels"] = w.Info.Labels
		}
		if w.Info.Hostname != "" {
			caps["hostname"] = w.Info.Hostname
		}
		if w.Info.Pid != "" {
			caps["pid"] = w.Info.Pid
		}
		var err error
		capabilitiesJSON, err = json.Marshal(caps)
		if err != nil {
			return err
		}
	}
	if !w.LastSeen.IsZero() {
		lastSeen = &w.LastSeen
	}

	ctx := context.Background()
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO runtimes (
			tenant_id, runtime_id, version, region, capabilities,
			last_heartbeat_at, status, updated_at
		)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6, $7, NOW())
		ON CONFLICT (tenant_id, runtime_id) DO UPDATE
		SET version = EXCLUDED.version,
		    region = EXCLUDED.region,
		    capabilities = EXCLUDED.capabilities,
		    last_heartbeat_at = EXCLUDED.last_heartbeat_at,
		    status = EXCLUDED.status,
		    updated_at = NOW()
	`, workerDefaultTenantUUID, w.ID, version, region, string(capabilitiesJSON), lastSeen, string(w.State))
	return err
}

// ByID implements Repository.
func (r *PostgresRepository) ByID(id string) (*model.Worker, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, model.ErrWorkerNotFound
	}

	ctx := context.Background()
	row := r.db.Pool.QueryRow(ctx, `
		SELECT runtime_id, version, region, capabilities, last_heartbeat_at, status
		FROM runtimes
		WHERE tenant_id = $1::uuid AND runtime_id = $2
		LIMIT 1
	`, workerDefaultTenantUUID, id)

	return r.scanWorker(row)
}

// All implements Repository.
func (r *PostgresRepository) All() ([]*model.Worker, error) {
	if r.db == nil || r.db.IsNoDB() {
		return nil, nil
	}

	ctx := context.Background()
	rows, err := r.db.Pool.Query(ctx, `
		SELECT runtime_id, version, region, capabilities, last_heartbeat_at, status
		FROM runtimes
		WHERE tenant_id = $1::uuid
	`, workerDefaultTenantUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*model.Worker
	for rows.Next() {
		w, err := r.scanWorker(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Delete implements Repository.
func (r *PostgresRepository) Delete(id string) error {
	if r.db == nil || r.db.IsNoDB() {
		return nil
	}

	ctx := context.Background()
	_, err := r.db.Pool.Exec(ctx, `
		DELETE FROM runtimes
		WHERE tenant_id = $1::uuid AND runtime_id = $2
	`, workerDefaultTenantUUID, id)
	return err
}

func (r *PostgresRepository) scanWorker(row interface {
	Scan(dest ...any) error
}) (*model.Worker, error) {
	var w model.Worker
	w.Info = &pb.WorkerInfo{}
	var capabilitiesJSON []byte
	var lastSeen *time.Time
	var status string
	err := row.Scan(&w.ID, &w.Info.Version, &w.Info.Region, &capabilitiesJSON, &lastSeen, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrWorkerNotFound
		}
		return nil, err
	}
	if lastSeen != nil {
		w.LastSeen = *lastSeen
	}
	w.State = model.State(status)
	if len(capabilitiesJSON) > 0 {
		var caps map[string]any
		_ = json.Unmarshal(capabilitiesJSON, &caps)
		// Best-effort decode; protobuf details are not fully reconstructed.
	}
	return &w, nil
}

var _ Repository = (*PostgresRepository)(nil)
