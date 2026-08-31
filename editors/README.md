# Editor support: the surveyor's instruments

You never get inside the Castle. You get better instruments for
surveying it.

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

- **GitHub rendering**: linguist ships grammars for registered
  languages only; a toy language about paperwork appreciates the irony
  of the registration requirement. Until then,
  `*.trial linguist-language=Text` in `.gitattributes` keeps the
  statistics honest.

A tree-sitter grammar remains open on the roadmap; the TextMate
grammar covers editors and the LSP covers meaning, which is the split
that matters.

## The language server: `trial counsel`

`trial counsel` speaks LSP 3.x on stdio. Diagnostics come from Gregor
itself — the same lexer, parser, and codegen that will judge the
filing when you file it, so the editor and the Court never disagree —
plus hover text that cites the reference manual and completion for
the required phrasing.

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

Filings that `INCORPORATE BY REFERENCE` receive parse-level counsel
only: the clerk cannot fetch enactments from inside an editor, and
counsel does not guess at law it has not read.

## The gallery: `trial watch`

`trial watch` is the live docket dashboard: every matter before the
court, where its attention rests, and how far behind the proceedings
it has fallen — consumer lag, rendered as what it is here. `--once`
for a single look; `--interval` to set the sweep.
