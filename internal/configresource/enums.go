package configresource

// BuiltinProviders are the provider names built into agentgateway.
var BuiltinProviders = []string{
	"openai",
	"openAI",
	"gemini",
	"vertex",
	"anthropic",
	"bedrock",
	"azure",
	"copilot",
}

// ProviderPresets are the provider names shipped as presets.
var ProviderPresets = []string{
	"cohere",
	"ollama",
	"baseten",
	"cerebras",
	"deepinfra",
	"deepseek",
	"groq",
	"huggingface",
	"mistral",
	"openrouter",
	"togetherai",
	"xai",
	"fireworks",
	"meta",
	"perplexity",
	"typesafe",
	"nebius",
}

// LLMPolicyKeys are the valid names for llm.policy resources.
var LLMPolicyKeys = []string{
	"oidc",
	"jwtAuth",
	"authorization",
	"extAuthz",
	"extProc",
	"cors",
	"transformations",
	"basicAuth",
	"apiKey",
	"guardrails",
	"localRateLimit",
	"remoteRateLimit",
	"timeout",
}

// MCPPolicyKeys are the valid names for mcp.policy resources.
var MCPPolicyKeys = []string{
	"mcpAuthentication",
	"mcpAuthorization",
	"mcpGuardrails",
	"a2a",
	"ai",
	"authorization",
	"cors",
	"jwtAuth",
	"oidc",
	"basicAuth",
	"apiKey",
	"extAuthz",
	"extProc",
	"backendTLS",
	"backendTunnel",
	"backendAuth",
	"localRateLimit",
	"remoteRateLimit",
	"requestHeaderModifier",
	"responseHeaderModifier",
	"requestRedirect",
	"urlRewrite",
	"requestMirror",
	"directResponse",
}

// UIPolicyKeys are the valid names for ui.policy resources.
var UIPolicyKeys = []string{
	"oidc",
	"jwtAuth",
	"cors",
	"csrf",
	"authorization",
	"basicAuth",
	"apiKey",
	"extAuthz",
}

// FrontendPolicyKeys are the valid names for frontend.policy resources.
var FrontendPolicyKeys = []string{
	"http",
	"tls",
	"tcp",
	"networkAuthorization",
	"networkExtAuthz",
	"substrateEgressActorResolution",
	"proxyProtocol",
	"connect",
	"accessLog",
	"tracing",
}
