# Security Policy

## Supported Versions

Only the latest commit on `main` receives security fixes. There are no versioned releases at this time.

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities by email to **mustafasercansak@gmail.com** with the subject line `[SECURITY] ZeroTrust`.

Include:
- A description of the vulnerability and its potential impact
- Steps to reproduce or a proof-of-concept (even partial)
- Affected components (backend, frontend, infra, etc.)
- Any suggested mitigations if you have them

You will receive an acknowledgement within **48 hours** and a status update within **7 days**.

## Disclosure Policy

- Vulnerabilities are fixed on a private branch and released without advance notice for critical issues.
- Credit is given in the commit message and changelog unless you prefer to remain anonymous.
- We follow coordinated disclosure: please allow reasonable time to fix before publishing.

## Scope

In scope:
- Authentication and session management logic
- JWT issuance, validation, and revocation
- CSRF protection
- RBAC / permission enforcement
- SQL injection, XSS, SSRF, and other OWASP Top 10 categories
- Docker / infra configuration that weakens security guarantees

Out of scope:
- Findings from automated scanners without demonstrated impact
- Theoretical vulnerabilities with no practical exploit path
- Issues in third-party dependencies (report those upstream)
