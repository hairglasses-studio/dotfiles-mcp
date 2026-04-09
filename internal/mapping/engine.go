package mapping

import "sync"

// ---------------------------------------------------------------------------
// Rule index — pre-compiled for O(1) source lookup
// ---------------------------------------------------------------------------

// RuleIndex is a pre-compiled lookup structure for fast rule matching.
type RuleIndex struct {
	bySource     map[string][]*MappingRule
	appOverrides map[string]map[string][]*MappingRule // source -> window_class -> rules
}

// BuildRuleIndex compiles a MappingProfile into a RuleIndex.
func BuildRuleIndex(p *MappingProfile) *RuleIndex {
	idx := &RuleIndex{
		bySource:     make(map[string][]*MappingRule),
		appOverrides: make(map[string]map[string][]*MappingRule),
	}

	for i := range p.Mappings {
		r := &p.Mappings[i]
		idx.bySource[r.Input] = append(idx.bySource[r.Input], r)
	}

	for _, override := range p.AppOverrides {
		for i := range override.Mappings {
			r := &override.Mappings[i]
			if idx.appOverrides[r.Input] == nil {
				idx.appOverrides[r.Input] = make(map[string][]*MappingRule)
			}
			idx.appOverrides[r.Input][override.WindowClass] = append(
				idx.appOverrides[r.Input][override.WindowClass], r,
			)
		}
	}

	return idx
}

// Resolve finds the best matching rule for a given input source and state.
// deviceID is used for layer-aware filtering — pass "" to skip layer checks.
func (idx *RuleIndex) Resolve(source string, state *EngineState, deviceID string) *MappingRule {
	state.mu.RLock()
	defer state.mu.RUnlock()

	activeLayer := state.ActiveLayer[deviceID]

	// App-specific overrides take priority.
	if appMap, ok := idx.appOverrides[source]; ok {
		if rules, ok := appMap[state.ActiveApp]; ok {
			if r := matchBest(rules, state, activeLayer); r != nil {
				return r
			}
		}
	}

	// Fall back to default mappings.
	if rules, ok := idx.bySource[source]; ok {
		return matchBest(rules, state, activeLayer)
	}

	return nil
}

// matchBest finds the highest-priority rule whose modifiers, conditions, and layer match.
func matchBest(rules []*MappingRule, state *EngineState, activeLayer int) *MappingRule {
	var best *MappingRule
	for _, r := range rules {
		if r.Layer != 0 && r.Layer != activeLayer {
			continue
		}
		if !modifiersMatch(r.Modifiers, state.ActiveModifiers) {
			continue
		}
		if r.Condition != nil && !r.Condition.Evaluate(state.Variables) {
			continue
		}
		if best == nil || r.Priority > best.Priority {
			best = r
		}
	}
	return best
}

func modifiersMatch(required []string, active map[string]bool) bool {
	for _, mod := range required {
		if !active[mod] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Engine state
// ---------------------------------------------------------------------------

// EngineState holds all mutable runtime state for the mapping engine.
type EngineState struct {
	mu sync.RWMutex

	ActiveModifiers map[string]bool
	ActiveApp       string
	ActiveLayer     map[string]int // deviceID -> layer index
	Variables       map[string]any
	PickupState     map[string]float64 // input source -> last output value
	FaderCrossed    map[string]bool
}

// NewEngineState creates an initialized engine state.
func NewEngineState() *EngineState {
	return &EngineState{
		ActiveModifiers: make(map[string]bool),
		ActiveLayer:     make(map[string]int),
		Variables:       make(map[string]any),
		PickupState:     make(map[string]float64),
		FaderCrossed:    make(map[string]bool),
	}
}

// SetActiveLayer sets the active layer for a device.
func (s *EngineState) SetActiveLayer(deviceID string, layer int) {
	s.mu.Lock()
	s.ActiveLayer[deviceID] = layer
	s.mu.Unlock()
}

// GetActiveLayer returns the active layer for a device (0 = default).
func (s *EngineState) GetActiveLayer(deviceID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ActiveLayer[deviceID]
}

// SetActiveApp updates the active application under a write lock.
func (s *EngineState) SetActiveApp(app string) {
	s.mu.Lock()
	s.ActiveApp = app
	s.mu.Unlock()
}

// SetModifier sets or clears a modifier key.
func (s *EngineState) SetModifier(mod string, active bool) {
	s.mu.Lock()
	if active {
		s.ActiveModifiers[mod] = true
	} else {
		delete(s.ActiveModifiers, mod)
	}
	s.mu.Unlock()
}

// SetVariable sets a variable value.
func (s *EngineState) SetVariable(name string, value any) {
	s.mu.Lock()
	s.Variables[name] = value
	s.mu.Unlock()
}

// GetVariable retrieves a variable value.
func (s *EngineState) GetVariable(name string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.Variables[name]
	return v, ok
}

// GetActiveApp returns the current active application.
func (s *EngineState) GetActiveApp() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ActiveApp
}

// RangeVariables iterates over all variables under a read lock.
func (s *EngineState) RangeVariables(fn func(key string, value any) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, v := range s.Variables {
		if !fn(k, v) {
			break
		}
	}
}
