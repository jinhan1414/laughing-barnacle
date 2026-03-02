package mcp

import (
	"slices"
	"sort"
	"strings"
)

func cloneServices(in []Service) []Service {
	out := make([]Service, len(in))
	for i := range in {
		out[i] = cloneService(in[i])
	}
	return out
}

func cloneService(in Service) Service {
	out := in
	out.Args = slices.Clone(in.Args)
	out.Env = cloneStringMap(in.Env)
	out.Headers = cloneStringMap(in.Headers)
	out.ToolStates = cloneToolStates(in.ToolStates)
	return out
}

func normalizeServiceArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, raw := range args {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneToolStates(in []ServiceToolState) []ServiceToolState {
	if len(in) == 0 {
		return nil
	}
	out := make([]ServiceToolState, len(in))
	copy(out, in)
	return out
}

func normalizeServiceToolStates(states []ServiceToolState) []ServiceToolState {
	if len(states) == 0 {
		return nil
	}

	byName := make(map[string]ServiceToolState, len(states))
	for _, state := range states {
		name := strings.TrimSpace(state.Name)
		if name == "" {
			continue
		}
		// Default is enabled; only store explicit non-default states.
		if state.Enabled {
			delete(byName, name)
			continue
		}
		state.Name = name
		state.Enabled = false
		byName[name] = state
	}
	if len(byName) == 0 {
		return nil
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ServiceToolState, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out
}

func serviceToolEnabled(service Service, toolName string) bool {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return false
	}
	for _, state := range service.ToolStates {
		if strings.TrimSpace(state.Name) == toolName {
			return state.Enabled
		}
	}
	return true
}
