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
  Until triallang is registered, add `*.trial linguist-language=Text` to
  `.gitattributes` to keep `.trial` files from affecting language statistics.

A tree-sitter grammar remains on the roadmap. The TextMate grammar provides
highlighting, and the LSP provides diagnostics, hover text, and completion.

## The language server: `trial counsel`

`trial counsel` speaks LSP 3.x on stdio. Diagnostics use the same Gregor lexer,
parser, and code generator as the CLI. The server also provides hover text that
cites the reference manual and completion for required phrases.

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

Filings that `INCORPORATE BY REFERENCE` receive parse-level diagnostics only,
because the language server does not fetch enactments.

## The gallery: `trial watch`

`trial watch` is a live dashboard of cases, their current attention, and
consumer lag. Use `--once` for one snapshot or `--interval` to set the refresh
period.
