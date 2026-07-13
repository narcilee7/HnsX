package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/api"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
	"go.uber.org/zap"
)

func seedFromDir(log *zap.Logger, s*api.Server, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Warn("seed cannot read directory", zap.String("dir", dir), zap.Error(err))
		return
	}
	registered := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := fmt.Sprintf("%s/%s/domain.yaml", dir, e.Name())
		ds, err := domain.LoadFile(path)
		if err != nil {
			log.Warn("seed skip file", zap.String("path", path), zap.Error(err))
			continue
		}
		s.RegisterBootstrapDomain(tenant.DefaultID, ds)
		registered++
	}
	if registered > 0 {
		log.Info("seed registered domains", zap.Int("count", registered), zap.String("dir", dir))
	} else {
		log.Warn("seed loaded zero.")
	}
}

func IsCleanShutdown(err error) bool {
	return err == nil || errors.Is(err, context.Canceled)
}
