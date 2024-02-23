package statsig

import (
	"encoding/json"
	"time"
)

type configSpec struct {
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	Salt         string          `json:"salt"`
	Enabled      bool            `json:"enabled"`
	Rules        []configRule    `json:"rules"`
	DefaultValue json.RawMessage `json:"defaultValue"`
	IDType       string          `json:"idType"`
}

type configRule struct {
	Name           string            `json:"name"`
	ID             string            `json:"id"`
	Salt           string            `json:"salt"`
	PassPercentage float64           `json:"passPercentage"`
	Conditions     []configCondition `json:"conditions"`
	ReturnValue    json.RawMessage   `json:"returnValue"`
	IDType         string            `json:"idType"`
}

type configCondition struct {
	Type             string                 `json:"type"`
	Operator         string                 `json:"operator"`
	Field            string                 `json:"field"`
	TargetValue      interface{}            `json:"targetValue"`
	AdditionalValues map[string]interface{} `json:"additionalValues"`
	IDType           string                 `json:"idType"`
}

type downloadConfigSpecResponse struct {
	HasUpdates     bool         `json:"has_updates"`
	Time           int64        `json:"time"`
	FeatureGates   []configSpec `json:"feature_gates"`
	DynamicConfigs []configSpec `json:"dynamic_configs"`
}

type downloadConfigsInput struct {
	SinceTime       int64           `json:"sinceTime"`
	StatsigMetadata statsigMetadata `json:"statsigMetadata"`
}

type store struct {
	featureGates   map[string]configSpec
	dynamicConfigs map[string]configSpec
	lastSyncTime   int64
	transport      *transport
	ticker         *time.Ticker
}

func newStore(transport *transport) *store {
	store := &store{
		featureGates:   make(map[string]configSpec),
		dynamicConfigs: make(map[string]configSpec),
		transport:      transport,
		ticker:         time.NewTicker(10 * time.Second),
	}

	specs := store.fetchConfigSpecs()
	store.update(specs)
	go store.pollForChanges()
	return store
}

func (s *store) StopPolling() {
	s.ticker.Stop()
}

func (s *store) update(specs downloadConfigSpecResponse) {
	if specs.HasUpdates {
		newGates := make(map[string]configSpec)
		for _, gate := range specs.FeatureGates {
			newGates[gate.Name] = gate
		}

		newConfigs := make(map[string]configSpec)
		for _, config := range specs.DynamicConfigs {
			newConfigs[config.Name] = config
		}

		s.featureGates = newGates
		s.dynamicConfigs = newConfigs
	}
}

func (s *store) fetchConfigSpecs() downloadConfigSpecResponse {
	input := &downloadConfigsInput{
		SinceTime:       s.lastSyncTime,
		StatsigMetadata: s.transport.metadata,
	}
	var specs downloadConfigSpecResponse
	s.transport.postRequest("/download_config_specs", input, &specs)
	s.lastSyncTime = specs.Time
	return specs
}

func (s *store) pollForChanges() {
	for range s.ticker.C {
		specs := s.fetchConfigSpecs()
		s.update(specs)
	}
}
