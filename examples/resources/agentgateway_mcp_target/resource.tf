# A streamable HTTP MCP target reached by host, port and path. Exactly one of
# `mcp`, `sse`, `stdio` or `openapi` must be set. `name` is the resource id, so
# renaming updates in place; tools are prefixed with it when multiplexing.
resource "agentgateway_mcp_target" "docs" {
  name      = "docs"
  condition = "request.headers[\"x-tenant\"] == \"acme\""

  mcp = {
    host = "mcp.example.com"
    port = 8080
    path = "/mcp"
  }

  # Transport policies for reaching the backend, as JSON. Not supported on
  # stdio targets.
  policies_json = jsonencode({
    cors = {
      allowOrigins = ["https://app.example.com"]
    }
  })
}

# The same transport can reference a backend defined in the gateway config
# instead of spelling out host and port. `host` and `backend` never combine.
resource "agentgateway_mcp_target" "files" {
  name = "files"

  mcp = {
    backend = "files-backend"
    path    = "/mcp"
  }
}

# An SSE target, using the same host/backend shape.
resource "agentgateway_mcp_target" "events" {
  name = "events"

  sse = {
    host = "https://events.example.com"
    path = "/sse"
  }
}

# A local process target. `clear_env` starts the process with an empty
# environment, so only `env` reaches it.
resource "agentgateway_mcp_target" "local" {
  name = "local"

  stdio = {
    cmd       = "npx"
    args      = ["-y", "@modelcontextprotocol/server-filesystem", "/srv/data"]
    env       = { LOG_LEVEL = "info" }
    clear_env = false
  }
}

# An OpenAPI target: every operation in the schema becomes an MCP tool. The
# gateway accepts the schema as a URL, a file path, or an inline document, and
# requires one; `schema_json` is a JSON leaf, so it is omitted from the payload
# when unset.
resource "agentgateway_mcp_target" "weather" {
  name = "weather"

  openapi = {
    host = "api.example.com"
    port = 443
    path = "/openapi.json"

    schema_json = jsonencode({
      url = "https://api.example.com/openapi.json"
    })
  }
}
