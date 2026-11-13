package client

// RuntimeInfo is the parsed form of GET /api/runtime.
type RuntimeInfo struct {
	Version         string // build.version
	GitRevision     string // build.gitRevision
	GatewayMode     string // ui.gatewayMode
	ConfigStoreMode string // ui.configStoreMode
	ConfigReloadOK  bool   // configReload.synchronized
	ConfigReloadErr string // configReload.lastError
}
