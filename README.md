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

No editor setup either. `document edit` opens `$VISUAL`, then `$EDITOR`, and
falls back to `vi` (`notepad` on Windows), so it works on a machine where
neither has ever been set.

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
written from them afterwards. `[logo]`, `[pdf]`, `[edit]` and `[cd]` are all
optional — the last two are described under
[Editing a document](#editing-a-document) below.

To see what sdsforge actually reads, including which browser and editor it
found and how big your logo will print:

```sh
sdsforge config show
```

## Making a sheet

Create a document. You get an annotated `document.yaml` listing every section
with its available presets and variants, plus version 1.0.0:

```sh
sdsforge document create "Sodium Chloride"
```

The command prints the path to the file. Open it with the id it was given:

```sh
sdsforge document edit 1
```

That opens `document.yaml` in your editor and re-reads it once you close it, so
a broken edit is reported straight away. Fill in the product's details. The
important line is the hazard codes:

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

For every other field — materials, Section 15 disclosures (Prop 65, SARA
311/312, state Right-to-Know), and overriding a section's tables such as
Section 8's exposure limits — see the
[document.yaml reference](docs/document-yaml.md).

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
| `document edit <id>` | Open its document.yaml in your editor. `--classify`, `--generate` |
| `document path [id]` | Print the directory its files live in |
| `document generate <id>` | Render the sheet. `--html`, `-o <path>` |
| `document classify <id>` | Show what the hazard codes produce |
| `document version create <id>` | Issue a revision. `--major`/`--minor`/`--patch`/`--label`, `-m` |
| `document version list <id>` | List issued revisions |
| `document version show <id> <version>` | Show one revision and its files |
| `document version restore <id> <version>` | Put an archived document.yaml back as the live one |
| `sections list [section-id]` | Inspect the content library |
| `sections validate` | Check the whole library for errors |
| `config path` / `init` / `show` | Locate, create and inspect the config file |
| `cd [id]` | Launch a shell in a document's directory |

`document` also answers to `doc`, `docs` and `documents`.

## Editing a document

```sh
sdsforge document edit 1
```

Opens the live `document.yaml`, waits for the editor to close, then re-reads the
file and reports any parse error — so a stray indent is caught while you still
remember making it. The file is left exactly as your editor wrote it either way;
nothing is repaired behind your back, and nothing already issued is touched.

The editor is the first of these that is set:

1. `command` under `[edit]` in the config file
2. `$VISUAL`
3. `$EDITOR`
4. `vi`, or `notepad` on Windows

`sdsforge config show` reports which one will actually open, which is the quick
way to answer "why does it keep starting vi".

Two things can follow a successful edit:

```sh
sdsforge document edit 1 --classify   # show what the hazard codes now produce
sdsforge document edit 1 --generate   # re-render the PDF
```

Turn either on for good under `[edit]`:

```toml
[edit]
command      = "nvim"
args         = ["-c", "set ft=yaml"]
classify     = true
generate     = false
min_duration = "1s"
```

`args` are passed before the filename, one array entry per argument — the place
for anything that would otherwise need quoting. The two flags override the
config for a single run in either direction, so `--classify=false` suppresses a
setting you have turned on.

`min_duration` guards against a graphical editor that hands the file to an
already-running window and exits immediately. sdsforge would then check a file
you have not saved yet and pronounce it fine. If your editor returns faster than
this, you get a warning saying it probably needs a wait flag — `EDITOR="code
--wait"`. Set `"0"` to stop asking.

## Working in a document's directory

A sheet's PDF, its archived versions and its `document.yaml` all sit in one
directory. To get a shell there:

```sh
sdsforge cd 1
```

That starts a shell with its working directory set, which you leave with `exit`.
It is a *new* shell, not a change to the one you typed in — no program can move
the shell that ran it. It runs with `SDSFORGE_SUBSHELL=1` set, plus
`SDSFORGE_DOCUMENT_ID`, so your prompt can show where it is. With no id you land
in the directory holding every document.

To move your current shell instead, substitute the path:

```sh
cd "$(sdsforge document path 1)"
```

`document path` writes a path to stdout and nothing else, which is what makes it
usable in a script or in your own shell function. With no id it prints the
directory holding every document.

The shell is `$SHELL` unless `[cd]` names another:

```toml
[cd]
command = "bash"
args    = ["--login"]
```

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

## Reference

- [document.yaml reference](docs/document-yaml.md) — every field, including
  Prop 65, SARA and state Right-to-Know, and how to override a section's
  content.

## References

- [Globally Harmonized System of Classification and Labelling of Chemicals (Rev.11)](https://unece.org/transport/documents/2025/09/standards/globally-harmonized-system-classification-and-labelling)
























