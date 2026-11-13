# Attach LLM traffic to gateways. This is the recommended shape: it works in
# hybrid storage mode and can be read back from the database.
resource "agentgateway_llm_settings" "default" {
  gateways = ["default", "llm/listener"]
}

# The deprecated alternative binds LLM traffic to a dedicated port with its own
# TLS listener. It conflicts with `gateways`, so use one or the other:
#
# resource "agentgateway_llm_settings" "default" {
#   port = 8080
#
#   tls_json = jsonencode({
#     cert = "/etc/tls/tls.crt"
#     key  = "/etc/tls/tls.key"
#   })
# }
#
# In hybrid mode a field the config file also sets is file-owned: a differing
# write fails with a diagnostic telling you to remove it from the file.
