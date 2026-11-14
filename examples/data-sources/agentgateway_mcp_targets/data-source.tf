# Virtual API key metadata. The data source never returns key material.
data "agentgateway_llm_api_keys" "all" {}

output "key_owners" {
  value = [
    for key in data.agentgateway_llm_api_keys.all.keys : {
      name  = key.name
      user  = key.user
      group = key.group
    }
  ]
}
