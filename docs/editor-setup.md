# Editor setup

`sdsforge` publishes a JSON Schema for `document.yaml` at
[`docs/document.schema.json`](document.schema.json). Point an editor at it and
you get completion and validation while you write a sheet:

- every top-level key, with the reference documentation on hover;
- all 71 GHS hazard codes, each showing its hazard statement;
- the 16 section ids, and for each one **only** the presets it actually has;
- for each subsection, **only** the variants that exist for that subsection —
  `first_aid.skin` offers `corrosive` and `default`, not every variant name in
  the library;
- `replace`/`append` constrained to the content kind that subsection declares,
  so a table cannot be dropped into a prose subsection;
- the 51 state Right-to-Know codes as boolean keys, named on hover;
- a diagnostic on any key the format does not have.

That last one matters more than it looks. `sdsforge` itself ignores unknown
keys, so a document with `hazard_code:` instead of `hazard_codes:` renders
happily — as an unclassified sheet. The schema is the only thing that catches
it.

## Which schema file to use

| Situation | Use |
| --- | --- |
| You have the repo checked out | `<checkout>/docs/document.schema.json` |
| You don't | `https://raw.githubusercontent.com/isaacvarg/sdsforge/main/docs/document.schema.json` |
| You use a custom content library | generate your own — see [below](#custom-content-libraries) |

The committed file describes the **built-in** library. If you have
`custom_variants = true` in your config, your own variants and presets are not
in it; generate a schema that knows about them instead.

## What consumes it

[yaml-language-server](https://github.com/redhat-developer/yaml-language-server)
— the same language server behind the Red Hat YAML extension in VS Code, the
`yamlls` LSP in Neovim, and YAML support in Helix and Zed. Any editor that runs
it can use this schema; only the wiring differs.

Documents live at `~/.local/share/sdsforge/documents/<id>/document.yaml`, which
is outside any project directory, so the file association has to be a glob on
the absolute path rather than a workspace-relative one.

## Neovim (LazyVim)

Save this as `~/.config/nvim/lua/plugins/sdsforge.lua`:

```lua
-- sdsforge: schema-driven completion and validation for document.yaml.
--
-- Prefers a local checkout so an unreleased schema is picked up, and falls back
-- to the published one so this file works unchanged on a machine with no clone.
local schema = "https://raw.githubusercontent.com/isaacvarg/sdsforge/main/docs/document.schema.json"
for _, dir in ipairs({ "~/forge/sdsforge", "~/src/sdsforge", "~/code/sdsforge", "~/projects/sdsforge" }) do
  local path = vim.fn.expand(dir .. "/docs/document.schema.json")
  if vim.uv.fs_stat(path) then
    schema = path
    break
  end
end

return {
  -- Brings in yaml-language-server (via mason) and SchemaStore. Equivalent to
  -- ticking `lang.yaml` in :LazyExtras; done here so this one file is enough.
  { import = "lazyvim.plugins.extras.lang.yaml" },

  {
    "neovim/nvim-lspconfig",
    opts = {
      servers = {
        yamlls = {
          settings = {
            yaml = {
              schemas = {
                [schema] = {
                  "**/sdsforge/documents/*/document.yaml",
                  "**/sdsforge/documents/*/versions/*/document.yaml",
                },
              },
            },
          },
        },
      },
    },
  },
}
```

Then `:Lazy sync` and `:MasonInstall yaml-language-server` if mason does not
pull it in on its own.

LazyVim's YAML extra merges SchemaStore into `settings.yaml.schemas` rather than
overwriting it, so the entry above survives alongside the schemas for
`.github/workflows` and friends.

If you would rather enable the extra the usual way, run `:LazyExtras`, tick
`lang.yaml`, and drop the `{ import = ... }` line from the file above.

## Neovim (plain nvim-lspconfig, no LazyVim)

```lua
vim.lsp.config("yamlls", {
  settings = {
    yaml = {
      schemas = {
        ["/path/to/sdsforge/docs/document.schema.json"] = {
          "**/sdsforge/documents/*/document.yaml",
          "**/sdsforge/documents/*/versions/*/document.yaml",
        },
      },
    },
  },
})
vim.lsp.enable("yamlls")
```

On Neovim 0.10 and earlier, the equivalent is
`require("lspconfig").yamlls.setup({ settings = { ... } })`. Either way
`yaml-language-server` has to be on `PATH` — `npm i -g yaml-language-server`, or
your package manager's build of it.

## VS Code

Install the Red Hat **YAML** extension, then in `settings.json`:

```json
{
  "yaml.schemas": {
    "/path/to/sdsforge/docs/document.schema.json": [
      "**/sdsforge/documents/*/document.yaml",
      "**/sdsforge/documents/*/versions/*/document.yaml"
    ]
  }
}
```

## Helix and Zed

Both run `yaml-language-server` and take its settings verbatim. In Helix's
`languages.toml`:

```toml
[language-server.yaml-language-server.config.yaml.schemas]
"/path/to/sdsforge/docs/document.schema.json" = ["**/sdsforge/documents/*/document.yaml"]
```

Zed takes the same block under `lsp.yaml-language-server.settings` in
`settings.json`.

## Any editor: the inline modeline

If you would rather not configure anything, yaml-language-server also honours a
schema declared in the file itself. Put this on the **first line** of a
`document.yaml`:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/isaacvarg/sdsforge/main/docs/document.schema.json
```

A `file:///absolute/path` URI works too. This travels with the document, which
is handy on a machine you do not control — at the cost of one line in every
document, and a line that is specific to whoever wrote it.

## Custom content libraries

The committed schema describes the shipped library. If yours is layered on top:

```sh
sdsforge schema --custom -o ~/.local/share/sdsforge/document.schema.json
```

Point your editor at that path instead. Re-run it whenever you add a variant or
a preset — the enums are generated from the files on disk, so a schema that
predates your new variant will flag it as invalid.

Without `--custom`, `sdsforge schema` reads no config file at all and describes
only the built-in library. That is deliberate: it is what keeps
`docs/document.schema.json` reproducible on every machine, and it is checked in
CI.

## Troubleshooting

**Nothing happens.** `:LspInfo` (or `:checkhealth lsp`) should list `yamlls` as
attached. If it is not, the server is not installed — `:Mason` and install
`yaml-language-server`.

**Attached, but no completion.** The glob did not match. The patterns above
require the file to sit under a directory literally named `sdsforge/documents/`;
if your `XDG_DATA_HOME` puts documents elsewhere, adjust the glob to match the
real absolute path. `:lua =vim.lsp.get_clients({name="yamlls"})[1].settings` will
show what the server was actually given.

**Everything is flagged red.** You are probably pointing at a stale schema
against a newer library, or at the built-in schema while running a custom
library. Regenerate with `sdsforge schema --custom -o <path>`.

**A valid variant is rejected.** Run `sdsforge sections list <section-id>` to see
what the library really offers, then regenerate. If the two disagree, that is a
bug — the schema is generated from those same files.
