# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| `main` branch | ✅ Active development |
| Latest release tag | ✅ When published |
| Older releases | ⚠️ Best effort |

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Use one of these channels:

1. **GitHub Security Advisories (preferred):** [Report a vulnerability](https://github.com/kantik001/mcp-gateway/security/advisories/new) on this repository.
2. **Private contact:** Open a [private security advisory](https://github.com/kantik001/mcp-gateway/security/advisories) or contact the maintainer via GitHub ([@kantik001](https://github.com/kantik001)).

Include as much detail as possible:

- Description of the vulnerability
- Steps to reproduce
- Affected components (HTTP API, MCP stdio transport, Docker config)
- Potential impact
- Suggested fix (if any)

We aim to acknowledge reports within **72 hours** and provide a status update within **7 days**.

## Disclosure policy

- We will confirm receipt and work on a fix.
- We will coordinate disclosure timing with you.
- Credit will be given in the advisory unless you prefer to remain anonymous.

## Security model overview

`mcp-gateway` proxies HTTP requests to local MCP server processes over stdio (JSON-RPC 2.0).
Those MCP servers may access filesystems, networks, or databases depending on configuration.

### Trust boundaries

| Component | Notes |
|-----------|-------|
| **HTTP API** | Unauthenticated by default; set `API_KEY` for simple shared-secret auth |
| **MCP subprocesses** | Inherit host/container privileges — treat `config/servers.yaml` as trusted config |
| **`/metrics`** | Unauthenticated by default — restrict via network policy in production |
| **PostgreSQL / Redis** | Reserved for future use in MVP; change default credentials before production |

### Out of scope for this repository

- Vulnerabilities inside third-party MCP servers (`@modelcontextprotocol/*`, `uvx` packages, etc.)
- Misconfiguration by deployers (exposed ports, overly broad filesystem roots)
- Issues requiring physical access to the host

We **do** accept reports for:

- Authentication bypass when `API_KEY` is configured
- Unsafe command injection via configuration parsing
- Secrets logged or exposed in HTTP responses
- Path/process isolation failures introduced by the gateway itself

## Secure deployment checklist

Before production:

- [ ] Set a strong `API_KEY` and require it for `/v1/*`
- [ ] Restrict filesystem MCP roots to the minimum required paths
- [ ] Do not expose `/metrics` publicly without a reverse proxy ACL
- [ ] Change default Postgres credentials in `docker-compose.yml`
- [ ] Run the gateway as a non-root user (Docker image already does)
- [ ] Review which MCP servers are `enabled: true`

## Dependencies

Report supply-chain concerns through the same private channels above.
