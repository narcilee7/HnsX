package store

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PostgresBackend persists namespace key/value pairs to the store_values table.
// It is safe for concurrent use via Postgres row-level locking.
type PostgresBackend struct {
	db *gorm.DB
}

// NewPostgresBackend constructs a Postgres-backed store using the supplied GORM
// connection.
func NewPostgresBackend(db *gorm.DB) *PostgresBackend {
	return &PostgresBackend{db: db}
}

// Get implements Backend.
func (b *PostgresBackend) Get(ns Namespace, key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	var rec storeValue
	if err := b.db.Where("namespace = ? AND key = ?", string(ns), key).First(&rec).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return rec.Value, nil
}

// Set implements Backend.
func (b *PostgresBackend) Set(ns Namespace, key, value string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if value == "" {
		return ErrInvalidValue
	}
	now := time.Now().UTC()
	return b.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "namespace"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&storeValue{
		Namespace: string(ns),
		Key:       key,
		Value:     value,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error
}

// Delete implements Backend.
func (b *PostgresBackend) Delete(ns Namespace, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	return b.db.Where("namespace = ? AND key = ?", string(ns), key).Delete(&storeValue{}).Error
}

type storeValue struct {
	ID        int64 `gorm:"primaryKey"`
	Namespace string
	Key       string
	Value     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName overrides the default table name.
func (storeValue) TableName() string { return "store_values" }

// Migrate ensures the store_values table exists. It is called by application
// startup after goose migrations have run.
func (b *PostgresBackend) Migrate(ctx context.Context) error {
	return b.db.WithContext(ctx).AutoMigrate(&storeValue{})
}
