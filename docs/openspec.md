# Work with OpenSpec

OpenSpec stores requirements and change plans beside the code. This repository
uses [OpenSpec 1.13.2](https://github.com/Fission-AI/OpenSpec) with its `spec-driven`
schema. The tool needs Node.js 20.19.0 or later and npm. Go builds do not need Node.js.

## Find the contract

The language, grammar, bytecode, and stored records remain defined in `spec/`.
OpenSpec adds capability requirements and acceptance scenarios. It does not
replace those detailed references.

| Path | Content |
| --- | --- |
| [openspec/config.yaml](../openspec/config.yaml) | Project context and artifact rules |
| [openspec/specs](../openspec/specs) | Current capability requirements |
| [openspec/changes](../openspec/changes) | Proposed deltas, designs, and tasks |
| [spec](../spec) | Normative language and storage references |

The [local example change](../openspec/changes/improve-local-examples/proposal.md)
shows a complete proposal in this format. It stays open while its draft PR is
under review. Do not archive a proposal to imply maintainer acceptance.

## Propose and implement a change

From the repository root, create a named change. Use a short name that describes
the resulting behavior. Then read the artifact instructions for that change.

```console
npx --yes @fission-ai/openspec@1.13.2 new change describe-the-change
npx --yes @fission-ai/openspec@1.13.2 instructions proposal --change describe-the-change
```

Write `proposal.md`, the affected capability deltas, `design.md`, and `tasks.md`.
Give each requirement a concrete `WHEN` and `THEN` scenario. State failure
behavior as well as success. Keep references to the existing language rules.

Add an ADR when a change affects a boundary listed in [CONTRIBUTING.md](../CONTRIBUTING.md).
Update the code and normative references together. Mark tasks complete only
after their implementation and required checks are done. Report unavailable
checks as unavailable.

## Validate and archive

Run the pinned validator before review. It checks the structure and requirement
deltas. It does not execute the Go tests or prove that implementation matches a spec.

```console
npx --yes @fission-ai/openspec@1.13.2 validate --all --strict --no-interactive
```

`make spec-check` runs the same command. CI runs it in a separate Node.js job.
`make verify` runs Go and documentation checks. Run both when a change edits
OpenSpec files.

After the maintainer accepts the change, archive it with the pinned CLI. The
archive operation applies its deltas to the current capability specs. Review
that diff before committing it.

```console
npx --yes @fission-ai/openspec@1.13.2 archive describe-the-change
```

Tool-specific skills are optional. To generate them locally, run the pinned
CLI's `init --tools <tool>` command. Follow the upstream list of supported tools.
Review generated files before including them in a patch. Set
`OPENSPEC_TELEMETRY=0` to disable the CLI's optional telemetry.
