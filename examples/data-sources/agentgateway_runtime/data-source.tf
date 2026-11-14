# Build, mode and config-reload information reported by the gateway's
# admin API. Use it to pin a Terraform run to the release it was written for.
data "agentgateway_runtime" "current" {}

output "gateway_build_version" {
  value = data.agentgateway_runtime.current.build_version
}

output "gateway_storage_mode" {
  value = data.agentgateway_runtime.current.config_store_mode
}
