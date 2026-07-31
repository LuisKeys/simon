package simon

import (
	"context"

	"github.com/LuisKeys/simon/internal/agent"
	internalmodel "github.com/LuisKeys/simon/internal/model"
	"github.com/LuisKeys/simon/internal/router"
	"github.com/LuisKeys/simon/model"
)

// buildModelOverride returns the internal model.Model a Session should be
// pinned to via agent.WithModelOverride, or nil if the Session should use
// Simon's own default per-prompt router (the common case: no custom
// Model/ModelRouter configured on the Runtime).
func (rt *Runtime) buildModelOverride(ctx context.Context) (internalmodel.Model, error) {
	if rt.modelOverride != nil {
		return model.ToInternal(rt.modelOverride), nil
	}
	if rt.customRouter != nil {
		provider, modelName := rt.customRouter.Resolve(ctx, "", "")
		choice := router.Choice{Provider: router.Provider(provider), Model: modelName}
		return agent.BuildProviderModel(rt.settings, choice)
	}
	return nil, nil
}
