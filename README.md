# sdsforge

A command line tool for producing Safety Data Sheets.

A new product is described by a YAML files by primarily listing triggered GHS hazard category codes. SDSForge generates section 2 from the triggers, derives the wording of the other relevant sections and generates the result as a PDF. Documents can be archived as numbered versions so the documents can be easily updated.

Only the OSHA 16-section format is currently available.

## Disclaimer
*USE AT YOUR OWN RISK.*

This tool is provided as-is, and the developer(s) are not liable for any errors, inaccuracies, or issues that may arise from its use. It is essential to exercise your own due diligence and verify the results obtained through this tool. The developer(s) make no guarantees about the accuracy, completeness, or reliability of the calculated data.

> **Review before production use.** The bundled content library is generic
> boilerplate and the GHS reference tables are transcribed data. Both carry
> legal and safety weight and must be reviewed by a qualified person before any
> sheet produced with them is issued.

## Requirements

A Chrome-based browser, to print. sdsforge searches `PATH` for `chromium`,
`chromium-browser`, `google-chrome`, `google-chrome-stable`, `brave`,
`brave-browser`, `microsoft-edge` and `microsoft-edge-stable`. Nothing is
bundled or downloaded. Everything except printing works without one.

Nothing else: the binary carries its own templates and content library, and Go
is needed only if you build it yourself.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/isaacvarg/sdsforge/main/install.sh | sh
```

That puts `sdsforge` in `~/.local/bin` (no sudo, override with
`SDSFORGE_INSTALL_DIR`) and tells you if anything is left to do. Read it first
if you would rather see what it does:
[install.sh](https://github.com/isaacvarg/sdsforge/blob/main/install.sh).

Or take an archive for your platform straight from the
[releases page](https://github.com/isaacvarg/sdsforge/releases), including
Windows, and put `sdsforge` on your `PATH` yourself.

With a Go 1.27 toolchain:

```sh
go install github.com/isaacvarg/sdsforge@latest
```

## Build from source

```sh
git clone https://github.com/isaacvarg/sdsforge.git
cd sdsforge
go build -o sdsforge .
```

Then put the binary somewhere on your `PATH`:

```sh
install -Dm755 sdsforge ~/.local/bin/sdsforge
```

Run the tests with `go test ./...`.

## Setup

Company details, emergency contacts and print settings are the same on every
sheet, so they live in one config file rather than in each document. Create it:

```sh
sdsforge config init
```

That writes a fully commented `~/.config/sdsforge/config.toml`. Fill in
`[company]` and at least one `[[emergency.contacts]]` block; section 1 is
written from them afterwards. `[logo]` and `[pdf]` are optional.

To see what sdsforge actually reads, including which browser it found and how
big your logo will print:

```sh
sdsforge config show
```

## Making a sheet

Create a document. You get an annotated `document.yaml` listing every section
with its available presets and variants, plus version 1.0.0:

```sh
sdsforge document create "Sodium Chloride"
```

The command prints the path to the file. Open it and fill in the product's
details. The important line is the hazard codes:

```yaml
product_name: Sodium Chloride
hazard_codes: [H315, H319]
```

Check what those codes produce before rendering anything. This writes nothing:

```sh
sdsforge document classify 1
```

You get the hazard statements, signal word, pictograms and precautionary
statements headed for section 2, plus which variant every other section derived
and why.

Print the sheet:

```sh
sdsforge document generate 1
```

The PDF lands in the document's directory and is overwritten on every run. Pass
`--html` to write the intermediate markup instead, or `-o` to choose a path.

When the sheet is ready to go out, record it as a revision:

```sh
sdsforge document version create 1 --minor -m "Added skin irritation hazard"
```

That archives the YAML, the HTML and the PDF into a snapshot directory and adds
a row to the sheet's own revision history in section 16. Pick the bump
deliberately: `--patch` for a correction that does not change hazard
information, `--minor` for new or reworded content, `--major` for a
reclassification or a new product identity.

## Commands

| Command | What it does |
| --- | --- |
| `document create <name>` | Create a document. `--minimal` skips the annotated template |
| `document list` | List documents by id |
| `document generate <id>` | Render the sheet. `--html`, `-o <path>` |
| `document classify <id>` | Show what the hazard codes produce |
| `document version create <id>` | Issue a revision. `--major`/`--minor`/`--patch`/`--label`, `-m` |
| `document version list <id>` | List issued revisions |
| `document version show <id> <version>` | Show one revision and its files |
| `document version restore <id> <version>` | Put an archived document.yaml back as the live one |
| `sections list [section-id]` | Inspect the content library |
| `sections validate` | Check the whole library for errors |
| `config path` / `init` / `show` | Locate, create and inspect the config file |

`document` also answers to `doc`, `docs` and `documents`.

## Where things live

Both locations honour the XDG environment variables.

```
~/.config/sdsforge/config.toml            the config file
~/.local/share/sdsforge/documents/
  index.yaml                              id to name
  1/
    document.yaml                         the live document
    sodium-chloride.pdf                   the last generate
    versions.yaml                         revision history
    versions/<snapshot>/                  archived yaml, html and pdf
```

## Custom content

The built-in library is compiled into the binary, but you can layer your own
wording over it. Point the config at a directory and turn it on:

```toml
[library]
custom_variants = true
custom_dir = "/home/you/sds-content"
```

Files there shadow the built-in ones of the same path, for example
`/home/you/sds-content/osha/04_first_aid/inhalation/site_specific.yaml`. Use
`sdsforge sections list` to see what exists and `sdsforge sections validate` to
check your additions.

## References

- [Globally Harmonized System of Classification and Labelling of Chemicals (Rev.11)](https://unece.org/transport/documents/2025/09/standards/globally-harmonized-system-classification-and-labelling)
























