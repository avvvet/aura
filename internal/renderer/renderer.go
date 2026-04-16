package renderer

import "github.com/avvvet/aura/internal/model"

// Renderer defines the interface for all output renderers
type Renderer interface {
	Render(snapshot *model.ClusterSnapshot) error
}
