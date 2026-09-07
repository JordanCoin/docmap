# 🗺️ docmap

> **docmap — instant documentation structure for LLMs and humans.**
> Navigate massive docs without burning tokens.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go](https://img.shields.io/badge/go-1.21+-00ADD8.svg)

## The Problem

Documentation files are everywhere — READMEs, design docs, changelogs, API references, PDFs. But:

- LLMs can't open large markdown files or PDFs (token limits)
- Humans have to open each file to see what's inside
- There's no "file tree" for documentation *content*
- And once you find a section, there's no way to say "show me the Python code block" or "find the warning callout"

## The Solution

```bash
docmap .
```

```
╭──────────────────────── docs/ ────────────────────────╮
│         22 files │ 645 sections │ ~109k tokens        │
│  18 callouts · 41 code blocks · 7 tables · 2 math     │
│  33 tasks (19 done) · 4 wiki · 6 embeds               │
╰────────────────────────────────────────────────────────╯

├── README.md (3.8k, 18 §) · 14 code · 1 tables
├── docs/ARCHITECTURE.md (7.7k, 33 §) · 6 code · 3 callouts
├── docs/API.md (12.1k, 47 §) · 21 code · 4 tables
└── CHANGELOG.md (15.2k, 41 §) · 2 callouts
```

One command. Full inventory. No LLM needed.

## Install the CLI

```bash
# macOS/Linux
brew tap JordanCoin/tap && brew install docmap

# Windows
scoop bucket add docmap https://github.com/JordanCoin/scoop-docmap
scoop install docmap
```

> Other options: [Releases](https://github.com/JordanCoin/docmap/releases) | `go install github.com/JordanCoin/docmap@latest`

## Install the Claude Code skill

docmap ships a Claude Code skill (`SKILL.md`) that teaches Claude when and how to use the CLI — the drill-downs, line lookups, search across notables, and `--since` git integration. Pick whichever install method fits your workflow.

### Option A — Plugin marketplace (recommended)

Inside Claude Code, add this repo as a marketplace and install the plugin:

```text
/plugin marketplace add JordanCoin/docmap
/plugin install docmap@docmap
```

That's it. Claude Code clones the repo, picks up `.claude-plugin/marketplace.json`, and installs the `docmap` plugin from `plugins/docmap/`. The skill becomes available as `/docmap` and Claude will auto-invoke it when you're working with markdown docs.

Update later with `/plugin marketplace update`.

### Option B — Personal user skill

Drop the SKILL.md into your personal Claude skills folder so it's available across every project, no marketplace needed:

```bash
mkdir -p ~/.claude/skills/docmap
curl -o ~/.claude/skills/docmap/SKILL.md \
  https://raw.githubusercontent.com/JordanCoin/docmap/main/plugins/docmap/skills/docmap/SKILL.md
```

### Option C — Project-scoped (commit to your repo)

If you want every contributor on a specific project to auto-load the docmap skill while working in that repo, commit it to `.claude/skills/`:

```bash
mkdir -p .claude/skills/docmap
curl -o .claude/skills/docmap/SKILL.md \
  https://raw.githubusercontent.com/JordanCoin/docmap/main/plugins/docmap/skills/docmap/SKILL.md
git add .claude/skills/docmap/SKILL.md
```

### Browse the skill

Read the shipped SKILL.md on GitHub before installing: [plugins/docmap/skills/docmap/SKILL.md](./plugins/docmap/skills/docmap/SKILL.md).

## MCP server (Cursor / Claude Code)

docmap ships a stdio [MCP](https://modelcontextprotocol.io/) server so agents can call structured tools instead of shelling out to the CLI.

```bash
docmap mcp
```

Tools: `docmap_brief`, `docmap_tree`, `docmap_since`, `docmap_stale`, `docmap_search`, `docmap_at_line`.

### Cursor

This repo includes `.mcp.json` so Cursor can auto-enable the server when the project is open (requires `docmap` on your `PATH`). Or add it manually in Cursor MCP settings:

```json
{
  "mcpServers": {
    "docmap": {
      "command": "docmap",
      "args": ["mcp"]
    }
  }
}
```

From a local checkout without installing globally:

```json
{
  "mcpServers": {
    "docmap": {
      "command": "go",
      "args": ["run", ".", "mcp"],
      "cwd": "/absolute/path/to/docmap"
    }
  }
}
```

### Claude Code

Same `.mcp.json` shape works for Claude Code project MCP config. After `brew install` / `go install`, `docmap mcp` is the server entrypoint.

## Usage

```bash
docmap .                            # Map everything in a directory
docmap README.md                    # Deep dive single file
docmap report.pdf                   # PDF document structure
docmap config.yaml                  # YAML file structure

docmap README.md --section "API"    # Filter to section
docmap README.md --expand "API"     # Raw section source with file:L-L
docmap . --brief                    # Session start: counts + recent docs (skips stale)
docmap . --brief --stale            # Brief plus stale claim count
docmap . --stale                    # Flag stale path/binary/date/env claims
docmap . --stale --remote --json    # Also check URLs/config values (network)

docmap file.md --type code          # List every code block
docmap file.md --type code --lang python   # Only Python code blocks
docmap file.md --type callout --kind warning  # Only warning callouts
docmap file.md --type table         # Every table with its headers

docmap file.md --at 154             # What's at line 154?
docmap file.md --since HEAD~5       # Constructs on lines changed since a git ref
docmap . --since HEAD --json        # Same, machine-readable (includes deletions)
docmap . --mentions --since HEAD    # Docs that mention files git says changed

docmap file.md --search "auth"      # Search titles, content, and notables
docmap dirA dirB --search "auth" --compact  # Search multiple roots
docmap docs/ --terms-file queries.txt --compact # One query per line
docmap . --refs                     # Cross-references between docs
docmap . --all                      # Include node_modules, vendor and .gitignore'd files
docmap file.md --json               # Full typed AST as JSON
```

Multiple roots are supported for search only. File paths in search results are
relative to the root they came from, so duplicate names from different roots
are retained as separate hits even when their displayed paths are identical.
When both `--search` and `--terms-file` are supplied, the
explicit search query runs first, followed by terms in file order; blank lines
and lines beginning with `#` are ignored.

Use `--compact` for one `file > section` line per hit (with `## term` headings
when multiple queries run). Add `--json` to get an array such as
`[{"term":"auth","file":"api.md","section":"Authentication","tokens":42}]`.

## Output

### Single file deep dive

```bash
docmap docs/ARCHITECTURE.md
```

```
╭──────────────── ARCHITECTURE.md ─────────────────╮
│           Sections: 33 │ ~7.7k tokens            │
│    3 callouts · 6 code blocks · 2 tables         │
╰───────────────────────────────────────────────────╯

├── System Design (2.1k) · core, actor model
│   ├── Vision (214)
│   ├── Core Principles (412) · Headless-first, Plan before execute
│   └── Architecture Overview (789) · go :42-68, mermaid :74-92
├── Components (3.2k)
│   ├── Scheduler (892) · note :118 Schedulers run in their own actor
│   ├── Orchestrator (1.1k) · 2 tables :156 Name, :172 State
│   └── Memory (RAG) (1.2k) · go :214-245, sql :250-268
└── Security (1.4k)
    └── (empty heading)
```

Every section shows its token count plus a dense inline annotation of what's inside it. Code blocks, callouts, tables, and math blocks all carry `:line` jump targets.

### Drilling into one construct type

Want every Python code block? Every warning? Every table?

```bash
docmap file.md --type code --lang python
```

```
╭──── file.md — code blocks ────╮
│    3 code blocks in 2 sections  │
╰─────────────────────────────────╯

Installation > Setup (214)
  :34-41    python    

Usage > Examples (1.1k)
  :120-134  python    
  :142-156  python    
```

Each hit carries the exact line range and breadcrumb. Drop that into a grep, an editor, or hand it to another agent.

### What's at line N?

```bash
docmap file.md --at 154
```

```
╭──── file.md — line 154 ────╮
│          code_block           │
╰──────────────────────────────╯

Section: Installation > Setup
Node:    code L154-156  lang=python
```

### Changed since a git ref

```bash
docmap README.md --since HEAD~10
```

```
╭──── README.md — since HEAD~10 ────╮
│  49 changed lines across 8 sections  │
╰─────────────────────────────────────╯

docmap > Usage (110)
  code L64-71  lang=bash

docmap > PDF Support (231)
  code L132-133  lang=bash
  code L136-145  lang=(none)
```

Uses `git diff --unified=0` from the file's repository root, so it works even when your shell is not inside the repo. New files (untracked, or added after the ref) count as fully changed. Directory mode (`docmap . --since main`) prints only the docs that actually changed, lists `deleted:` / `renamed:` for docs that moved since the ref, and ignores non-doc paths. Binary docs (e.g. PDFs) light up as fully changed. Combine with `--json` to get `changed_lines`, `change` (`A`/`M`/`D`/`R`), and `old_path` for renames. `docmap . --mentions --since HEAD` uses git's changed paths (including deletes) as the mention needles.

### Stale claims

```bash
docmap . --stale
docmap . --stale --days 30 --check-flags
docmap . --stale --remote --allow-domains example.com,github.com --json
```

Local checks (zero network): missing backticked paths, binaries not on `PATH`, status-heading dates older than `--days` (default 90), and `$ENV` keys absent from `.env.example` / compose / config files.

`--remote` adds HEAD checks for http(s) URLs, `tool 1.2.3` vs `tool --version`, and `KEY=value` claims that disagree with root config. Output is `file > section: reason` plus a count; `--json` emits the finding array. Default `--brief` skips stale checks (prints a hint); `--brief --stale` adds a one-line `stale: N` summary.

### PDF support

PDFs with outlines show document structure; tokens are estimated. PDFs without outlines fall back to page-by-page. Scanned/image-only PDFs show a page count but no text.

### YAML support

YAML files map keys to sections with nested children. Sequences use `name`/`id`/`title` fields for titles when available.

### References mode

See how docs link to each other:

```bash
docmap . --refs
```

## What docmap recognizes

Full CommonMark + GitHub Flavored Markdown + Obsidian extensions:

- **Headings** — ATX and Setext (underline) style, all 6 levels
- **Frontmatter** — YAML, TOML, JSON at file start
- **Callouts** — GFM alerts: `> [!NOTE]` / `[!TIP]` / `[!IMPORTANT]` / `[!WARNING]` / `[!CAUTION]`
- **Tables** — with column alignment and inline content
- **Code blocks** — fenced with language tag, indented, tilde-fenced, with attributes
- **Lists & tasks** — ordered/unordered, nested, tight/loose, GFM task checkboxes
- **Blockquotes** — plain, nested, lazy continuation
- **Math** — inline `$…$`, block `$$…$$` and `\[…\]`
- **Footnotes** — references and multi-paragraph definitions
- **Definition lists** — Pandoc-style
- **HTML blocks** — `<div>`, `<details>`, `<kbd>`, comments, entities
- **Link references** — `[label]: url "title"`, reference-style links and images
- **Autolinks** — angle-bracket URLs, GFM bare URLs, email addresses
- **GFM extras** — `@mentions`, `#issues`, commit SHA autolinks, `:emoji:` shortcodes
- **Obsidian** — `[[wiki links]]`, `[[Page|alias]]`, `[[Page#header]]`, `[[Page#^block]]`, `![[embeds]]` with sizing

## Why docmap?

| Before | After |
|--------|-------|
| "Read this 100k token doc" | `docmap --type code file.md` → 8 jump targets |
| Open 20 files to find something | `docmap .` inventory header |
| Scroll through giant CHANGELOGs | `--at 2400` → which section am I in? |
| Guess what's in each doc | Every file has a one-line notable digest |
| grep for "warning" in callouts | `--type callout --kind warning` |

## Sister tool

docmap is the documentation companion to [codemap](https://github.com/JordanCoin/codemap):

```bash
codemap .   # code structure
docmap .    # doc structure
```

Together: complete spatial awareness of any repository.

## How it works

**Markdown:** parsed with [goldmark](https://github.com/yuin/goldmark) (CommonMark + GFM extensions) plus post-passes for math blocks, GFM callouts, Obsidian wiki links, HTML entities, `@mentions`, `#issue` refs, commit SHAs, and emoji shortcodes. The result is a typed AST with 40+ node kinds that the renderer compresses into the dense tree view.

**PDF:** outline/bookmarks parsed by `ledongthuc/pdf`, falling back to per-page structure if no outline exists.

**YAML:** parsed by `yaml.v3` with keys mapped to sections.

No API calls. Just fast, local parsing.

**Performance:** parsed documents are cached under `<repo>/.docmap/cache` (git root of each file), keyed by absolute path + mtime + size. Set `DOCMAP_NO_CACHE=1` to disable.

## JSON output

`docmap file.md --json` emits the full typed AST alongside the legacy sections tree:

```json
{
  "documents": [{
    "filename": "file.md",
    "summary": {
      "callouts": 5, "tables": 4, "code_blocks": 8,
      "tasks": 6, "tasks_checked": 3, "wiki_links": 4
    },
    "sections": [...],
    "nodes": [
      { "kind": "frontmatter", "format": "yaml", "raw": "..." },
      { "kind": "heading", "level": 1, "title": "..." },
      { "kind": "code_block", "language": "python",
        "line_start": 154, "line_end": 156, "code": "..." },
      { "kind": "callout", "variant": "warning",
        "line_start": 228, "line_end": 230 }
    ]
  }]
}
```

Pipe it into `jq`, another tool, or hand it to an agent.

## Contributing

1. Fork → 2. Branch → 3. Commit → 4. PR

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

MIT
