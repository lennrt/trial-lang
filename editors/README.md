# Editor support

This directory contains a TextMate grammar, editor configuration, and setup
notes for the triallang language server.

## Syntax highlighting

[`trial.tmLanguage.json`](trial.tmLanguage.json) is a TextMate grammar
for `.trial` filings (and it tolerates `.deposition` files). It works
anywhere TextMate grammars do:

- **VS Code**: drop a minimal extension folder into
  `~/.vscode/extensions/triallang/` containing the grammar and this
  `package.json`:

  ```json
  {
    "name": "triallang",
    "displayName": "triallang",
    "version": "2.0.0",
    "engines": { "vscode": "^1.0.0" },
    "contributes": {
      "languages": [{
        "id": "trial",
        "extensions": [".trial", ".deposition"],
        "configuration": "./language-configuration.json"
      }],
      "grammars": [{
        "language": "trial",
        "scopeName": "source.trial",
        "path": "./trial.tmLanguage.json"
      }]
    }
  }
  ```

  with [`language-configuration.json`](language-configuration.json)
  beside it (comments and bracket behavior).

- **GitHub rendering**: Linguist ships grammars for registered languages only.
  In a repository that embeds triallang files, add
  `*.trial linguist-language=Text` to that repository's `.gitattributes` if
  they should be excluded from language statistics.

A tree-sitter grammar remains on the roadmap. The TextMate grammar provides
highlighting, and the LSP provides diagnostics, hover text, completion, and document outlines.

## The language server: `trial counsel`

`trial counsel` speaks LSP 3.x on stdio. Diagnostics use the same Gregor lexer,
parser, and code generator as the CLI. The server also provides hover text that
cites the reference manual and completion for required phrases.

The client must send full-document updates with exactly one text change.
An explicitly empty string clears a document; missing or null text leaves the
last accepted document unchanged. Positions use the LSP default
[UTF-16 encoding](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#position).
Columns exclude CRLF line endings, and a character offset past a line clamps
to that line's end.

- **Neovim** (0.10+):

  ```lua
  vim.filetype.add({ extension = { trial = "trial" } })
  vim.api.nvim_create_autocmd("FileType", {
    pattern = "trial",
    callback = function()
      vim.lsp.start({ name = "trial-counsel", cmd = { "trial", "counsel" } })
    end,
  })
  ```

- **VS Code / others**: point any generic LSP client at the command
  `trial counsel` for the language id `trial`.

By default, filings that `INCORPORATE BY REFERENCE` receive syntax-only
diagnostics. Use `trial counsel --canon` to compile documents whose imports all
come from the bundled canon. In Neovim, change the command above to
`cmd = { "trial", "counsel", "--canon" }`.

Canon mode resolves transitive imports and catches errors such as a missing
article, a wrong number of arguments, or a duplicate office. With only `--canon`,
a custom or unknown import keeps the whole document on syntax-only checks.

For custom statutes, supply each file explicitly, with dependencies first:

```lua
cmd = { "trial", "counsel", "--canon",
  "--enact", "/path/to/project/statutes/base.trial",
  "--enact", "/path/to/project/statutes/project.trial" }
```

Once `--enact` is supplied, missing imports and invalid dependency text are
errors. Relative paths use the server's working directory. The limit is 100
distinct paths, 4 MiB per file, and 256 MiB total. Standard input (`-`) cannot be
a statute path because it carries LSP messages.

Open buffers for configured files override their saved contents. Edits refresh
diagnostics in importing documents; closing a configured buffer reloads its
saved file. Restart the server after external changes to closed files. File URI
escaping is normalized, and Windows paths match without regard to case. Symlinks
are not resolved; opening a second URI alias for one configured file is rejected
until the first closes. Unconfigured buffers never supply imports.

Errors appear at the source file and UTF-16 range that owns the rejection, with
related importing locations when available. Bundled source locations use
`trial-canon:///` virtual URIs, which an editor may display without opening;
a contextual diagnostic also appears in the importing document.
Fixed diagnostics clear across files, and closing one importer does not clear
errors still needed by another. Versioned diagnostics follow accepted edits;
out-of-order changes are ignored.

Counsel reads only configured files, never discovers paths from import names,
never contacts Kafka, and never executes documents. See the
[interface contract](../docs/interfaces.md#counsel-lsp) for error and lifecycle
details.

## Docket view: `trial watch`

`trial watch` is a live dashboard of cases, their current attention, and
consumer lag. Use `--once` for one snapshot or `--interval` to set the refresh
period.

Counsel provides a document outline through `textDocument/documentSymbol`.
Editors can navigate named constants/exhibits, articles, offices and office
sections in source order. It uses the current buffer and UTF-16 ranges, without
executing source or loading dependencies. Syntactically invalid buffers return
an empty outline until they can be parsed again.
