# The model catalog prices requests for USD budgets and adds internal models to
# the catalog the gateway ships with. At least one of `base_json` and
# `custom_json` is set; both may be set together.
#
# A base catalog carries `metadata.generatedAt`: the gateway uses the newest
# base catalog and replaces the built-in one with it.
#
# A custom catalog has no `metadata` and is applied as an overlay on top of the
# base catalog. Use it for contracted rates and for models the built-in catalog
# does not know. Rates are per 1M tokens.
resource "agentgateway_model_catalog" "default" {
  base_json = jsonencode({
    metadata = {
      generatedAt = "2026-11-14T00:00:00Z"
    }
    providers = {}
  })

  custom_json = jsonencode({
    providers = {
      internal = {
        models = {
          "acme-large" = {
            name = "ACME Large"
            rates = {
              input  = "1.50"
              output = "6.00"
            }
            limits = {
              context = 128000
              output  = 8192
            }
          }
        }
      }
    }
  })
}

# A base catalog without an overlay is also valid; set only `base_json` then.
# The catalog is a singleton with the fixed id `default` and requires hybrid
# storage: in file mode the gateway does not list it, so it cannot be read back.
