package renderer

import "github.com/avvvet/steered/internal/model"

// Renderer defines the interface for all output renderers
type Renderer interface {
	Render(snapshot *model.ClusterSnapshot) error
}
