# Improve authoring workflows and execution safety

## Why

Independent investigations reproduced crashes on malformed stored execution
data, a Kafka client limit below the documented record bound, and MCP pages
discarded after JSON escaping. Authors lack source outlines, bounded syntax
depth, structured deposition results and selective suite execution. A reachable
standard-library advisory also requires the next Go patch.

## What Changes

- Reject structurally invalid stored state and instruction counts before unsafe operations.
- Configure Kafka producers for the existing record bound, with explicit broker limits.
- Return observe pages that fit the encoded MCP envelope and preserve resume offsets.
- Add AST-derived Counsel document outlines and positioned syntax-depth limits.
- Add JSON Lines deposition reports and independently useful suite selection/listing.
- Update the pinned compiler for the observed security advisory.

## Capabilities

### New Capabilities

- `execution-validation`: Safe rejection of malformed stored execution data.
- `kafka-record-bounds`: Producer limits consistent with accepted record sizes.
- `mcp-observe-pages`: Successful encoded-size-bounded output pagination.
- `counsel-outline`: Source-ordered outlines for open documents.
- `syntax-depth`: Positioned rejection of excessive syntax and AST depth.
- `deposition-reporting`: Machine-readable per-test outcomes and summaries.
- `deposition-selection`: Filtered, listed, nonduplicated deposition suites.

### Modified Capabilities

The pending release-preparation proposal also gains module-version compatibility
and command-lifecycle checks. The public canon documentation remains part of the
same consolidated PR. No stored JSON shape or public Go signature changes.

## Impact

See [ADR 0014](../../../docs/adr/0014-bounded-authoring-and-storage-validation.md).
Language resource bounds and CLI/LSP capabilities gain documentation. Existing
evaluation order, bytecode tags, storage topics and default human output remain
compatible. The two unmerged release/docs improvements and these eight benefits
form one ten-improvement PR, rather than ten arbitrary file changes.
