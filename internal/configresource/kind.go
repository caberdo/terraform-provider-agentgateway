package configresource

// Kind identifies a config resource kind.
type Kind string

const (
	KindModelCatalog    Kind = "modelCatalog"
	KindLLMProvider     Kind = "llm.provider"
	KindLLMModel        Kind = "llm.model"
	KindLLMVirtualModel Kind = "llm.virtualModel"
	KindLLMAPIKey       Kind = "llm.apiKey"
	KindLLMPolicy       Kind = "llm.policy"
	KindLLMSettings     Kind = "llm.settings"
	KindMCPTarget       Kind = "mcp.target"
	KindMCPPolicy       Kind = "mcp.policy"
	KindMCPSettings     Kind = "mcp.settings"
	KindTrafficGateway  Kind = "traffic.gateway"
	KindTrafficRoute    Kind = "traffic.route"
	KindTrafficTCPRoute Kind = "traffic.tcpRoute"
	KindUIPolicy        Kind = "ui.policy"
	KindFrontendPolicy  Kind = "frontend.policy"
)

// All returns the API-visible kind list, in the order the docs use.
func All() []Kind {
	return []Kind{
		KindModelCatalog,
		KindLLMProvider,
		KindLLMModel,
		KindLLMVirtualModel,
		KindLLMAPIKey,
		KindLLMPolicy,
		KindLLMSettings,
		KindMCPTarget,
		KindMCPPolicy,
		KindMCPSettings,
		KindTrafficGateway,
		KindTrafficRoute,
		KindTrafficTCPRoute,
		KindUIPolicy,
		KindFrontendPolicy,
	}
}

// IsPolicy reports whether the kind is written through the single-resource path.
func (k Kind) IsPolicy() bool {
	switch k {
	case KindLLMPolicy, KindMCPPolicy, KindUIPolicy, KindFrontendPolicy:
		return true
	}
	return false
}

// IsSingleton reports whether the kind has exactly one resource with id "default".
func (k Kind) IsSingleton() bool {
	switch k {
	case KindLLMSettings, KindMCPSettings, KindModelCatalog:
		return true
	}
	return false
}
