# Split traffic between two concrete models by relative weight.
resource "agentgateway_llm_virtual_model" "weighted" {
  name = "smart"

  weighted {
    targets {
      model  = "gpt-4o-primary"
      weight = 90
    }

    targets {
      model  = "gpt-4o-fallback"
      weight = 10
    }
  }
}

# Prefer the lower priority group and fail over when a model degrades.
resource "agentgateway_llm_virtual_model" "failover" {
  name = "resilient"

  failover {
    targets {
      model    = "claude-primary"
      priority = 0
    }

    targets {
      model    = "claude-backup"
      priority = 1
    }
  }
}

# Select a target by request context. The final target may omit `when`; it is
# the fallback when no expression matches.
resource "agentgateway_llm_virtual_model" "conditional" {
  name = "adaptive"

  conditional {
    targets {
      model = "openai-fast"
      when  = "request.headers[\"x-tier\"] == \"free\""
    }

    targets {
      model = "openai-smart"
      when  = "request.headers[\"x-tier\"] == \"pro\""
    }

    targets {
      model = "openai-smart"
    }
  }
}
