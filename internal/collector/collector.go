package collector

import (
	"context"

	"github.com/avvvet/aura/internal/model"
)

// Collector defines the interface every resource collector must implement
type Collector interface {
	Collect(ctx context.Context, snapshot *model.ClusterSnapshot) error
}
