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

## Install

```
brew install --cask joshgummersall/ankix/ankix
```

## Requirements

- [Ollama](https://ollama.com/) — the `kindle`, `youtube`, and `web` commands
  all use the same `ankix` model (`ollama/vocab/Modelfile`)
- [Anki](https://apps.ankiweb.net/) running with [AnkiConnect](https://ankiweb.net/shared/info/2055492159) installed
- `yt-dlp` for the `youtube` command

After installing, build the local Ollama model once:

```
ankix install
```

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
- `--model` — Ollama model used to define words (default `ankix`)
- `--tag` — tags applied to new notes (default `AnkiX::Source::Kindle`)
- `--dry-run` — preview without writing to Anki
- `--ankiconnect-url` — AnkiConnect endpoint (default `http://localhost:8765`)

Only words not already marked Mastered in `vocab.db` are considered, and any
word that ends up in Anki (added, or already there) is marked Mastered (sets
`WORDS.category` to `1`), removing it from the Kindle's Vocabulary Builder
review queue — that's what tracks sync progress across runs, no separate
watermark is kept. This opens `vocab.db` read-write (except for a headless
`--dry-run`), so point it at the device itself rather than a copy if you want
the change to take effect on the device. Before writing anything, `vocab.db`
is copied to `vocab.db.bak` alongside it.

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

Flags (persistent across both subcommands):

- `--deck` — Anki deck name (default `AnkiX`)
- `--ankiconnect-url` — AnkiConnect URL (default `http://localhost:8765`)
- `--ollama-url` — Ollama URL (default `http://localhost:11434`)
- `--ollama-model` — Ollama gloss model name (default `ankix`)
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
- `--ollama-model` — Ollama gloss model name (default `ankix`)
- `--no-gloss` — skip Ollama gloss lookups

## Using a different language

Everything except the Ollama model itself is language-agnostic — the
`--lang`/`--sub-lang` flags just select a language code, and Kindle/subtitle
parsing don't assume any particular language. `ollama/vocab/Modelfile` is the
one piece that's Spanish-specific: its system prompt and few-shot examples
are written for Spanish-to-English glossing.

To study another language, fork `ollama/vocab/Modelfile` (or replace it in
place) with a system prompt and examples for that language, then either:

- run `mise run setup` (or `ankix install --model <name>`) to build it under
  a new Ollama model name, and pass `--model`/`--ollama-model <name>` when
  running `kindle`/`youtube`, or
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
