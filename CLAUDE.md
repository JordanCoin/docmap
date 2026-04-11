# docmap

Run `docmap .` at the start of any session to understand documentation structure.

## When to use

- **Before** reading large markdown files — get the structure first
- **Exploring** a new repository's docs
- **Finding** where specific topics, code blocks, callouts, or tables live
- **Locating** a specific construct by type (`--type code --lang python`)
- **Reverse-lookup** when you have a line number from a diff, grep hit, or error
- **Understanding** what changed in docs since a git ref

## Core commands

```bash
docmap .                            # Directory inventory + per-file digest
docmap README.md                    # Single file: dense tree with notables
docmap README.md --section "API"    # Filter to one section
docmap README.md --expand "API"     # Show raw content of a section
```

## Typed drill-downs

Every markdown construct is addressable by kind:

```bash
docmap file.md --type code                     # Every code block
docmap file.md --type code --lang python       # Only Python code blocks
docmap file.md --type callout                  # Every GFM alert
docmap file.md --type callout --kind warning   # Only warning callouts
docmap file.md --type table                    # Every table with headers
docmap file.md --type math                     # Every math block with TeX preview
docmap file.md --type footnote                 # All footnote definitions
docmap file.md --type task                     # Task list counts per section
docmap file.md --type wiki                     # Obsidian wiki links
docmap file.md --type linkref                  # Reference-style link definitions
docmap file.md --type html                     # HTML blocks (div, details, etc.)
```

Other supported `--type` values: `deflist`, `embed`, `mention`, `issue`, `sha`, `emoji`.

## Line-based navigation

```bash
docmap file.md --at 154                 # What construct is at line 154?
docmap file.md --since HEAD~5           # Constructs on lines changed since git ref
```

`--at` is the reverse lookup — give it a line number from a grep hit, diff, or error, and it tells you the section breadcrumb and node type.

## Search

```bash
docmap file.md --search "python"        # Matches titles, content, code languages,
                                        # callout variants, table headers, math TeX,
                                        # footnote IDs, link labels, HTML content
```

`--search` is broader than a simple grep — it searches through the typed AST so "python" matches code block languages, "warning" matches callout variants, etc.

## Reading the output

The header box shows an inventory of what's in the file:

```
╭──── file.md ────╮
│ Sections: 30 │ ~2.3k tokens                     │
│ 5 callouts · 4 tables · 8 code blocks · 2 math  │
│ 6 tasks (3 done) · 4 wiki · 6 embeds            │
╰──────────────────────────────────────────────────╯
```

Each section line in the tree carries a dense annotation of its notable contents:

```
├── Code Blocks (141) · go :142-149, python :154-156, bash :161-162, ...
├── GFM Alerts (51) · note :219, tip :222, warning :228
├── Tables (57) · 4 tables :108 Name, :116 Left, :123 Field, :131 A
```

**`:154` is the jump target.** Same convention as `grep`, `ripgrep`, compiler errors, vim, and every editor — the number is a 1-indexed line in the source file. `:154-156` is a line range.

## JSON output for structured consumption

```bash
docmap file.md --json
```

Returns the full typed AST including per-section `notables`, a top-level `summary`, and every node with its kind + line range. Pipe to `jq` or hand to another agent.

## What docmap recognizes

Full CommonMark + GFM + Obsidian: frontmatter, ATX + Setext headings, callouts (all 5 GFM alert kinds), tables (with alignment), code blocks (fenced + indented, by language), lists (incl. task lists), blockquotes, math blocks (`$$…$$`, `\[…\]`), footnotes, definition lists, HTML blocks, link references, autolinks (`<url>`, GFM bare URLs, email), mentions, issue refs, commit SHAs, emoji shortcodes, Obsidian wiki links and embeds.

## Pair with codemap

```bash
codemap .   # code structure
docmap .    # doc structure
```

Together: complete project understanding.
