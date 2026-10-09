package app

import (
	"fmt"

	"github.com/ShadowSmallBaby/ClawProxyHub/internal/action"
)

type extensionImpact struct {
	Extensions []string `json:"extensions"`
	Plugins    []string `json:"plugins"`
	Tasks      []int64  `json:"tasks"`
	Pages      []string `json:"pages"`
	Actions    []string `json:"actions"`
}

func (a *App) extensionImpact(id string) (extensionImpact, error) {
	result := extensionImpact{Extensions: []string{}, Plugins: []string{}, Tasks: []int64{}, Pages: []string{}, Actions: []string{}}
	for _, state := range a.extensions.List() {
		if _, depends := state.Manifest.Dependencies[id]; depends && state.Enabled {
			result.Extensions = append(result.Extensions, state.Manifest.ID)
		}
		if state.Manifest.ID == id {
			for _, page := range state.Manifest.Pages {
				result.Pages = append(result.Pages, id+"/"+page.ID)
			}
		}
	}
	for _, d := range a.actions.List(action.Principal{Role: "admin"}) {
		if d.Owner == id {
			result.Actions = append(result.Actions, d.ID)
		}
	}
	if id == "lua-runtime" {
		for _, p := range a.plugins.Installed() {
			if p.Runtime == "lua" {
				result.Plugins = append(result.Plugins, p.Name)
			}
		}
		if len(result.Plugins) > 0 {
			err := a.db.Table("task_rules AS r").Joins("JOIN plugins AS p ON p.id=r.plugin_id").Where("p.name IN ? AND r.enabled = ?", result.Plugins, true).Pluck("r.id", &result.Tasks).Error
			if err != nil {
				return result, err
			}
		}
	}
	return result, nil
}
func (a *App) guardExtensionRemoval(id string) error {
	if id != "lua-runtime" {
		return nil
	}
	impact, err := a.extensionImpact(id)
	if err != nil {
		return err
	}
	if len(impact.Tasks) > 0 {
		return fmt.Errorf("Lua runtime is required by enabled task rules %v; disable those rules first", impact.Tasks)
	}
	return nil
}
