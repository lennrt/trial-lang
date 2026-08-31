# Go API compatibility

Snapshot date: 2026-08-29

Module: `github.com/lennrt/trial-lang`

Go version: 1.27.0

## External package

`canon` is the only importable package.

```text
package canon
var FS embed.FS
var Order []string
func Files() []string
```

`Files` returns a caller-owned slice. `Order` remains for source
compatibility and is deprecated because callers can mutate it.

All other Go packages are commands or are under `internal`. They are not a
public Go API.

## Wire compatibility

Stored JSON uses `encoding/json` v1 and the existing tags. This review does not
change `omitempty` to `omitzero`.

The MCP surface adds bounded pagination. Valid calls without pagination fields
use a default limit of 100.

Run the external-package compile test:

```console
go test ./canon
make api-check
```

`docs/api.txt` is the generated public API snapshot. `make api-check` fails if
the package documentation and snapshot differ. The snapshot omits the final
empty line from `go doc` output.
