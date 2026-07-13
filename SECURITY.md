# Security Policy

sqlglot-go parses and analyzes SQL, and its scope/qualify/lineage APIs are used in
**security-adjacent** settings (column lineage, access/gating decisions). A parsing or classification
error can have security consequences downstream, so we take reports seriously.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Report privately via GitHub's **[Private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)**:
the repository **Security** tab → **Report a vulnerability**. (Maintainers: enable this under
Settings → Code security.)

Please include: affected version/commit, a minimal reproducing SQL input and dialect, the incorrect
behavior vs. what you expected, and the security impact.

We aim to acknowledge a report within a few business days, agree on a disclosure timeline, and credit
reporters who wish to be named once a fix ships.

## Scope

In scope: incorrect parse/generate results, mis-scoping, or source mis-classification that could lead a
downstream consumer to a wrong access/lineage decision; panics or unbounded resource use on adversarial
input.

Out of scope: behavior that faithfully mirrors upstream sqlglot (report those upstream too), and
intentional divergences already documented in [DEVIATIONS.md](./DEVIATIONS.md).

## Supported versions

This project is pre-1.0; only the **latest released minor** receives fixes. Pin a version and upgrade to
receive security fixes.
