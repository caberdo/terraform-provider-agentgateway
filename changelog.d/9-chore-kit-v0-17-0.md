The CI, release and coverage tooling moves to `caberdo/terraform-provider-kit` at `v0.17.0`,
whose weekly maintenance check no longer asks the supported-versions matrix for stable agentgateway
releases below its declared floor. The matrix stays at `v1.7.0`, the first release with the full
config resource API, instead of demanding `v1.4.0`–`v1.6.0`, which the acceptance suite cannot pass.
