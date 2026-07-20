# ankix

Generate Anki cards from two sources: Kindle vocabulary builder highlights,
and YouTube video transcripts.

## Requirements

- [Ollama](https://ollama.com/) — both the `kindle` and `youtube` commands use
  the same `ankindle` model (`ollama/vocab/Modelfile`)
- [Anki](https://apps.ankiweb.net/) running with [AnkiConnect](https://ankiweb.net/shared/info/2055492159) installed
- Go to build; `yt-dlp` for the `youtube` command

## Build

```
mise run build       # builds bin/ankix
mise run setup       # creates the Ollama model
```

## `ankix kindle` — Kindle vocabulary builder

```
ankix kindle vocab --db ~/Documents/vocab.db --lang en --deck "Kindle Vocab"
```

Find `vocab.db` on your Kindle at `system/vocabulary/vocab.db`, or in a
backup/export of the device.

Each note uses Anki's built-in `Basic` note type: `Front` is the highlighted
sentence from the book with the looked-up word in **bold**; `Back` is the
definition.

Flags:

- `--db` — path to vocab.db (required)
- `--lang` — language prefix to filter words by, e.g. `en`, `es` (default `en`)
- `--deck` — Anki deck to sync into (default `Kindle Vocab`)
- `--model` — Ollama model used to define words (default `ankindle`)
- `--tag` — tags applied to new notes (default `AnkiX::Source::Kindle`)
- `--dry-run` — preview without writing to Anki
- `--mastered` — filter out words already marked Mastered in `vocab.db`
  from this sync (by default they're included), and mark words that end
  up in Anki as Mastered (sets `WORDS.category` to `1`), removing them
  from the Kindle's Vocabulary Builder review queue. Opens `--db`
  read-write, so point it at the device itself rather than a copy if you
  want the change to take effect on the device. Before writing anything,
  `vocab.db` is copied to `vocab.db.bak` alongside it.
- `--ankiconnect-url` — AnkiConnect endpoint (default `http://localhost:8765`)

Each new note is tagged `AnkiX::Word::<word>`; re-running `sync` checks
AnkiConnect for that tag in the target deck to skip words already synced,
so it's safe to re-run.

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

- `--deck` — Anki deck name (default `Spanish::AnkiX`)
- `--ankiconnect-url` — AnkiConnect URL (default `http://localhost:8765`)
- `--ollama-url` — Ollama URL (default `http://localhost:11434`)
- `--ollama-model` — Ollama gloss model name (default `ankindle`)
- `--sub-lang` — subtitle language code (default `es`)
- `--cache-dir` — subtitle cache directory
- `--no-gloss` — skip Ollama gloss lookups
