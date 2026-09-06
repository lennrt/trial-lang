## Summary

<!-- What changes and why. Link the issue if one exists. Explain the problem,
     the resulting behavior, and how you validated it. -->

## Checklist

- [ ] `make verify` and `make vuln` pass with Go 1.27.0
- [ ] Behavior changes have deterministic tests with a finite timeout; generated seeds are recorded
- [ ] `CHANGELOG.md` has an entry under the unreleased version
- [ ] CLI changes update `cmd/trial/help.go` and `docs/interfaces.md` (the command-table test enforces the help text)
- [ ] Language or bytecode changes update `spec/` and add or adjust a deposition under `examples/`
- [ ] Public, wire, storage, security, or configuration boundary changes have an ADR under `docs/adr/`
- [ ] `canon` changes update `docs/api.txt` (`make api-check`) and `docs/api-compatibility.md`
- [ ] No credentials, payloads, personal data, or raw identifiers in diagnostics or fixtures

## Compatibility

<!-- Stored JSON, topic layout, bytecode, public Go API: unchanged, or describe the migration. -->
