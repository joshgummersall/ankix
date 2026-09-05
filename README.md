# ankix

**Turn words you look up while reading, watching, or browsing into contextual Anki translations.**

Reading on a Kindle and looking up words builds a vocabulary list nobody
reviews. Watching foreign-language YouTube with subtitles surfaces words you
want to remember and then loses them the moment the video ends. Reading an
article in another language means alt-tabbing to a dictionary for every other
word. `ankix` closes those loops: it reads your Kindle's `vocab.db`, a video's
subtitle track, or a web article, defines each word with a local Ollama model
using the sentence it appears in — not just a bare dictionary entry — and
syncs ready-to-study notes straight into Anki via AnkiConnect.

**"Gloss"** turns up throughout this README and in the flag names. It's the
linguistic term for a short translation of a word *as used in one specific
sentence*, as opposed to a dictionary definition, which lists every sense out
of context. That contextual reading is what goes on the card; the word's
dictionary base form travels with it separately, as the *lemma*.

## Install

```
brew install --cask joshgummersall/ankix/ankix
```

## Requirements

- [Ollama](https://ollama.com/) — the `kindle`, `youtube`, and `web` commands
  all use the same `ankix` model (`ollama/vocab/Modelfile`)
- [Anki](https://apps.ankiweb.net/) running with the [AnkiConnect](https://ankiweb.net/shared/info/2055492159)
  add-on installed (Tools → Add-ons → Get Add-ons..., code `2055492159`, then restart Anki)
- `yt-dlp` for the `youtube` command

After installing, build the local Ollama model once:

```
ankix install
```

Re-run it after every `ankix` upgrade — the prompt lives in the model, so a
new release usually needs a new build. `ankix` checks this for you and says
so; see [Keeping the model in sync](#keeping-the-model-in-sync).

By default this builds `ankix` on top of `llama3.2:3b`. To build on a
different Ollama model (it must already be pulled, e.g. via
`ollama pull qwen2.5:14b`), set it in your config file:

```toml
base_model = "qwen2.5:14b"
```

`ankix install` takes no flags — both the name it builds and the model it
builds FROM come from the config file, because what it produces outlives the
command. There's no "just this once" for a build: a base model passed on the
command line would stay installed until the next `ankix install`, which
every upgrade asks you to run, silently put the default back.

This only swaps the base model the same prompt and few-shot examples run
on — see [Using a different language](#using-a-different-language) below if
you want to change the prompt itself.

## Configuration

Flag defaults can be set once in a config file instead of passed on every
invocation. `ankix` looks for it at `$XDG_CONFIG_HOME/ankix/config.toml` (or
`~/.config/ankix/config.toml` if that variable isn't set), then falls back
to the OS-specific default from Go's `os.UserConfigDir()` (e.g.
`~/Library/Application Support/ankix/config.toml` on macOS) if nothing's
found at the first location. A missing file is fine — every setting just
keeps its built-in default.

```toml
deck = "AnkiX"
ankiconnect_url = "http://localhost:8765"
ollama_url = "http://localhost:11434"
ollama_model = "ankix"  # the model ankix install builds and every command looks up
base_model = "llama3.2:3b"  # what ankix install builds that model FROM
ollama_keep_alive = "30m"   # how long Ollama holds the model in memory after a lookup
no_gloss = false
lang = "es"          # seeds --lang (kindle) and --sub-lang (youtube)

[kindle]
lang = "es"           # overrides the top-level lang for kindle only

[youtube]
sub_lang = "es"        # overrides the top-level lang for youtube only
cache_dir = "/path/to/subtitle/cache"

[card]
front = """
<b>{{.Word}}</b><br><br>{{.Before}}<i>{{.Word}}</i>{{.After}}
"""
back = """
{{.Definition}}{{if and .Definition .Attribution}}<br><br>{{end}}{{.Attribution}}
"""
```

### Custom card formatting

`[card].front` and `[card].back` are [Go templates](https://pkg.go.dev/text/template)
that render every note's `Front`/`Back` fields; leaving either unset keeps
the built-in formatting shown above. TOML's multiline `"""..."""` strings
are the natural way to write them — a leading/trailing newline from that
syntax is trimmed automatically. The data available to both templates
(`internal/anki.CardData`):

| Field         | Description                                                                 |
|---------------|------------------------------------------------------------------------------|
| `Word`        | the marked headword/phrase                                                  |
| `Before`      | sentence text before `Word` (valid only if `Highlighted`)                   |
| `After`       | sentence text after `Word` if `Highlighted`, else the whole (unmarked) sentence |
| `Highlighted` | whether `Before`/`Word`/`After` is an actual split of a sentence            |
| `Definition`  | formatted definition or gloss (HTML), `""` if none                          |
| `Lemma`       | dictionary/base-form lemma, e.g. `"to realize"` for `"realized"`; `""` if none or same as `Definition` |
| `Source`      | video/episode/page title, `""` for Kindle                                   |
| `Timestamp`   | e.g. `"3:41"`, `""` if not applicable                                       |
| `Link`        | deep link URL, `""` if none                                                 |
| `LinkLabel`   | `"watch"` / `"listen"` / `"read"`, `""` if `Link` is `""`                   |
| `Attribution` | `Source`, `Timestamp` and `Link` pre-combined into one HTML fragment, e.g. `Title (3:41) — <a href="...">watch</a>` |

A template with bad syntax, or one referencing a field that doesn't exist,
is rejected with an error as soon as `ankix` starts, rather than surfacing
mid-sync or mid-review.

## `ankix kindle` — Kindle vocabulary builder

```
ankix kindle vocab ~/Documents/vocab.db --lang en --deck "Kindle Vocab"
```

Find `vocab.db` on your Kindle at `system/vocabulary/vocab.db`, or in a
backup/export of the device.

Each note uses Anki's built-in `Basic` note type: `Front` is the highlighted
sentence from the book with the looked-up word in **bold**; `Back` is the
definition.

Positional argument: path to `vocab.db`.

Flags:

- `--lang` — language prefix to filter words by, e.g. `en`, `es` (default `en`)
- `--deck` — Anki deck to sync into (default `Kindle Vocab`)
- `--tag` — tags applied to new notes (default `AnkiX::Source::Kindle`)
- `--limit` — only the N most recently looked-up words (0 for no limit)
- `--headless` — sync straight through, skipping the interactive review
- `--dry-run` — preview without writing to Anki (only with `--headless`)
- `--eject` — eject the Kindle's volume after a successful sync (macOS only)
- `--ankiconnect-url` — AnkiConnect endpoint (default `http://localhost:8765`)

Only words not already marked Mastered in `vocab.db` are considered, and any
word that ends up in Anki (added, or already there) is marked Mastered (sets
`WORDS.category` to `1`), removing it from the Kindle's Vocabulary Builder
review queue — that's what tracks sync progress across runs, no separate
watermark is kept. This opens `vocab.db` read-write (except for a headless
`--dry-run`), so point it at the device itself rather than a copy if you want
the change to take effect on the device. Before writing anything, `vocab.db`
is snapshotted into a timestamped backup log under
`$XDG_CONFIG_HOME/ankix/kindle-backups` (or the OS equivalent), namespaced
per source file. Use `ankix kindle vocab db list <vocab.db>` to see past
snapshots and `ankix kindle vocab db restore <vocab.db> <index>` to roll
back to one (which itself snapshots the current file first, so a restore is
always undoable).

The namespace is keyed by the `vocab.db` **path** you pass in, not the
device itself — `vocab.db` has no serial number or account ID to key off.
If the same Kindle ever mounts at a different path, or you copy `vocab.db`
somewhere new, `list`/`restore` will see it as an unrelated file and start a
fresh backup history rather than continuing the old one. Always point
`list`/`restore` at the same path you've been syncing that device with.

Re-running `sync` checks AnkiConnect for an existing note with a matching
headword in the target deck to skip words already synced, so it's safe to
re-run.

Definitions are hydrated through the `dict.Provider` interface
(`internal/dict/dict.go`), so other sources can be added later without
touching sync logic. The only implementation today is `internal/dict/ollama`.

## `ankix youtube` — YouTube transcripts

```
ankix youtube fetch <youtube-url>
ankix youtube review <transcript-file.vtt>
```

`fetch` downloads subtitles via `yt-dlp` and opens the transcript in a
terminal UI for browsing and generating cards; `review` opens an existing
`.vtt` transcript file directly, skipping `yt-dlp`.

Once words are marked, `r` on a marked word re-prompts the model to fix its
translation — see [Fixing a translation](#fixing-a-translation).

Flags (persistent across both subcommands):

- `--deck` — Anki deck name (default `AnkiX`)
- `--ankiconnect-url` — AnkiConnect URL (default `http://localhost:8765`)
- `--ollama-url` — Ollama URL (default `http://localhost:11434`)
- `--ollama-model` — Ollama gloss model name (default `ankix`; see [Keeping the model in sync](#keeping-the-model-in-sync))
- `--ollama-keep-alive` — how long Ollama keeps the model loaded after a lookup (default `30m`; see [Model warm-up](#model-warm-up))
- `--sub-lang` — subtitle language code (default `es`)
- `--cache-dir` — subtitle cache directory
- `--no-gloss` — skip Ollama gloss lookups

## `ankix web` — Web articles

```
ankix web fetch <url>
```

Fetches and extracts the article text from a URL, then opens it in a
terminal UI for browsing and generating cards — the same TUI used by
`ankix youtube`.

Flags:

- `--deck` — Anki deck name (default `AnkiX`)
- `--ankiconnect-url` — AnkiConnect URL (default `http://localhost:8765`)
- `--ollama-url` — Ollama URL (default `http://localhost:11434`)
- `--ollama-model` — Ollama gloss model name (default `ankix`; see [Keeping the model in sync](#keeping-the-model-in-sync))
- `--ollama-keep-alive` — how long Ollama keeps the model loaded after a lookup (default `30m`; see [Model warm-up](#model-warm-up))
- `--no-gloss` — skip Ollama gloss lookups

## Model warm-up

Loading a gloss model costs seconds — more with a larger base model like
Qwen — and that cost lands on whatever request arrives first. Left alone,
that's the first word you pick, after the review screen is already open,
where the wait is the most visible.

So every command that glosses fires a throwaway lookup in the background as
soon as it starts, before fetching the source or building the document. It's
a real lookup rather than a bare load, which makes Ollama evaluate the
Modelfile's system prompt and few-shot examples too; both the weights and
that prompt prefix are then reused by real lookups. Nothing waits on it —
if it fails, the same failure resurfaces on the first genuine lookup, which
reports it properly. `--no-gloss` skips it along with everything else.

Ollama unloads an idle model after 5 minutes by default, which is shorter
than the gaps between lookups when you're reading — so the model would drop
out mid-session and the next word would pay the load cost again. ankix sends
`keep_alive` with every request to widen that window to 30 minutes. Change
it with `--ollama-keep-alive` (or `ollama_keep_alive` in the config file):
a duration like `1h`, a number of seconds, `0` to unload as soon as each
reply is sent, or a negative value to keep the model loaded indefinitely.
Longer keeps memory occupied after ankix exits; shorter gives it back sooner
at the cost of reloading.

## Fixing a translation

Small local models drift: a translation comes back with an extra adjective
("new keys" for `llaves`), or picks the literal sense of a word the sentence
is using figuratively. Rather than delete the word and lose the card, put the
cursor on it in the review screen and press `r`, then type what's wrong with
the answer in plain language:

```
llaves: new keys (key)

  refine: drop the adjective

llaves: keys (key)
```

The instruction is sent to the model as a follow-up turn on its own answer,
so it corrects that line rather than translating a new word — and an explicit
correction outranks the prompt's own "don't include neighboring words" rules,
which is what makes "include the noun it modifies" work. Corrections chain:
each one starts from the current answer, so you can narrow in over a couple
of passes. Re-scoping the phrase with `v` (or deleting it with `d`) discards
the correction, since it no longer describes the same words.

`r` works in the Kindle, YouTube, podcast, web and file review screens, and
is inert under `--no-gloss`. It edits the preview only — cards already synced
to Anki aren't touched.

**This needs the model rebuilt** after upgrading — `ankix` will tell you so
and refuse to run until you do, rather than quietly using the old one. See
[Keeping the model in sync](#keeping-the-model-in-sync).

## Keeping the model in sync

`ankix`'s prompt lives inside the Ollama model, not the binary, so the two
have to agree — a new release with new prompt rules is useless against a
model built by the old one. Rather than detect that after the fact, the two
are bound by name: `ankix install` tags the model with a checksum of the
Modelfile it was built from, and every command asks for exactly that tag.

```
$ ankix install
model "ankix:f12a425b6b17" ready

$ ollama list
ankix:f12a425b6b17    1a243fa51a81    2.0 GB
```

A model built by an older release is therefore not stale so much as
unaddressable — the new binary is asking for a name that doesn't exist yet,
and says so before doing any work:

```
$ ankix web fetch https://example.com
error: the "ankix" model is out of date: found ankix:9c04d1e7f8a2,
  but this version of ankix needs ankix:f12a425b6b17
run `ankix install` to build it (the older build is left alone, and keeps
  working with the older ankix)
```

Old builds are kept, not deleted: they share their weights with the new one
so they cost almost nothing, and an older `ankix` binary keeps working
against the tag it expects. Remove one with `ollama rm ankix:<tag>` when you
no longer want it.

The checksum covers the prompt only, not the base model — which model you
build on is your choice and doesn't change the name (`ollama show` will tell
you which one a build used). That choice lives in the config file's
`base_model`, so the rebuild each upgrade asks for keeps it.

`ollama_model` (and its `--ollama-model` flag) names the model in one place
for both sides — `ankix install` builds it and every other command looks it
up. There's deliberately no separate flag for the name to install under: a
second way to say it would just be a second way to disagree.

To point `ankix` at a model you built yourself, give `--ollama-model` an
explicit tag (`--ollama-model myfork:v1`). A name with a `:` in it is used
verbatim, with no checksum appended and no rebuild prompting — see
[Using a different language](#using-a-different-language).

## Using a different language

Everything except the Ollama model itself is language-agnostic — the
`--lang`/`--sub-lang` flags just select a language code, and Kindle/subtitle
parsing don't assume any particular language. `ollama/vocab/Modelfile` is the
one piece that's Spanish-specific: its system prompt and few-shot examples
are written for Spanish-to-English glossing.

To study another language, fork `ollama/vocab/Modelfile` (or replace it in
place) with a system prompt and examples for that language, then either:

- set `ollama_model = "<name>"` in your config file and run `ankix install`
  — it builds `<name>:<checksum>` and every command looks that up, with
  nothing to pass per invocation (`--ollama-model` overrides it for one
  run), or
- build it by hand (`mise run create`, or `ollama create <name>:<tag> -f
  Modelfile`) and pass the full `--ollama-model <name>:<tag>`, which `ankix`
  uses verbatim, or
- rebuild the default `ankix` model in place if you only need one language
  at a time.

## Development

Requires Go and [mise](https://mise.jdx.dev/).

```
mise run build       # builds bin/ankix
mise run test         # go test ./...
mise run vet          # go vet ./...
mise run setup        # creates the Ollama model locally
mise run install      # builds and copies ankix to ~/.local/bin
```
