package repository

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	pb "github.com/hnsx-io/hnsx/server/proto/gen/go/hnsx/v1"
	"github.com/hnsx-io/hnsx/server/internal/worker/model"
	"github.com/hnsx-io/hnsx/server/pkg/db"
)

const workerDefaultTenantUUID = "00000000-0000-0000-0000-000000000000"

// PostgresRepository persists worker aggregates to Postgres using GORM.
type PostgresRepository struct {
	db *gorm.DB
}

// NewPostgresRepository constructs a Postgres-backed worker repository.
func NewPostgresRepository(database *db.DB) *PostgresRepository {
	if database == nil || database.GormDB == nil {
		return &PostgresRepository{}
	}
	return &PostgresRepository{db: database.GormDB}
}

// Save implements Repository.
func (r *PostgresRepository) Save(w *model.Worker) error {
	if r.db == nil {
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

	now := time.Now().UTC()

	return r.db.Transaction(func(tx *gorm.DB) error {
		var rec RuntimeRecord
		err := tx.Where("tenant_id = ? AND runtime_id = ?", workerDefaultTenantUUID, w.ID).
			Take(&rec).Error
		isNew := errors.Is(err, gorm.ErrRecordNotFound)

		rec.TenantID = workerDefaultTenantUUID
		rec.RuntimeID = w.ID
		rec.Version = version
		rec.Region = region
		rec.Capabilities = capabilitiesJSON
		rec.LastHeartbeatAt = lastSeen
		rec.Status = string(w.State)
		rec.UpdatedAt = now
		if isNew {
			rec.CreatedAt = now
			return tx.Create(&rec).Error
		}
		return tx.Save(&rec).Error
	})
}

// ByID implements Repository.
func (r *PostgresRepository) ByID(id string) (*model.Worker, error) {
	if r.db == nil {
		return nil, model.ErrWorkerNotFound
	}

	var rec RuntimeRecord
	if err := r.db.Where("tenant_id = ? AND runtime_id = ?", workerDefaultTenantUUID, id).
		Take(&rec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrWorkerNotFound
		}
		return nil, err
	}

	return r.toModel(rec)
}

// All implements Repository.
func (r *PostgresRepository) All() ([]*model.Worker, error) {
	if r.db == nil {
		return nil, nil
	}

	var records []RuntimeRecord
	if err := r.db.Where("tenant_id = ?", workerDefaultTenantUUID).Find(&records).Error; err != nil {
		return nil, err
	}

	out := make([]*model.Worker, 0, len(records))
	for _, rec := range records {
		w, err := r.toModel(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

// Delete implements Repository.
func (r *PostgresRepository) Delete(id string) error {
	if r.db == nil {
		return nil
	}

	return r.db.Where("tenant_id = ? AND runtime_id = ?", workerDefaultTenantUUID, id).
		Delete(&RuntimeRecord{}).Error
}

func (r *PostgresRepository) toModel(rec RuntimeRecord) (*model.Worker, error) {
	w := &model.Worker{
		ID:    rec.RuntimeID,
		Info:  &pb.WorkerInfo{Version: rec.Version, Region: rec.Region},
		State: model.State(rec.Status),
	}
	if rec.LastHeartbeatAt != nil {
		w.LastSeen = *rec.LastHeartbeatAt
	}
	if len(rec.Capabilities) > 0 {
		var caps map[string]any
		_ = json.Unmarshal(rec.Capabilities, &caps)
		// Best-effort decode; protobuf details are not fully reconstructed.
	}
	return w, nil
}

var _ Repository = (*PostgresRepository)(nil)
