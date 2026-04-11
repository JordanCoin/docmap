---
name: docmap
description: Navigate markdown, PDF, and YAML documentation structure without reading full files. Use before opening any large .md file, to locate specific constructs (code blocks, callouts, tables), to jump to a line number's context, or to see what changed in docs since a git ref.
---

# docmap skill

`docmap` is a local CLI that turns a markdown file into a typed AST and surfaces every construct as a jump target. Use it as your **first read** of any documentation you haven't seen before.

## When to invoke

Reach for docmap when any of these is true:

- You're about to `Read` a markdown file larger than ~200 lines
- You need to find a specific kind of content (code block, callout, table, math block, footnote, task list, wiki link)
- You have a line number (from grep, diff, error, user) and need to know what construct lives there
- You want to see what changed in docs since a git ref
- You're exploring an unfamiliar repository and want a project-wide inventory
- You want structured JSON output of a document's AST

Do **not** invoke docmap for:

- Short READMEs that fit in context anyway
- Non-documentation markdown (chat logs, commit messages, issue bodies)
- Markdown files you've already read in this session

## Command reference

### Directory inventory

```bash
docmap .
docmap docs/
```

Returns a project-wide inventory header (total callouts, code blocks, tables, tasks, etc.) plus a one-line digest per file. Use this to decide which file to drill into.

### Single-file tree

```bash
docmap README.md
```

Returns a dense tree where each section carries an inline annotation of its notable contents. Every construct has a `:line` jump target.

### Drill into one construct type

```bash
docmap file.md --type <kind>
```

Valid kinds: `code`, `callout`, `table`, `math`, `footnote`, `deflist`, `linkref`, `html`, `task`, `wiki`, `embed`, `mention`, `issue`, `sha`, `emoji`.

Sub-filters:

```bash
docmap file.md --type code --lang python
docmap file.md --type callout --kind warning
```

Output lists every matching construct grouped by containing section, with exact line ranges.

### Reverse lookup (line → construct)

```bash
docmap file.md --at 154
```

Returns the section breadcrumb and node type at line 154. Use this when you have a line number from anywhere (grep, diff, error, LSP) and need to know what it actually is.

### Changed since git ref

```bash
docmap file.md --since HEAD~5
docmap file.md --since main
```

Shows only the constructs that sit on lines modified since the given git ref. Uses `git diff --unified=0` under the hood.

### Search across notables

```bash
docmap file.md --search "python"
docmap . --search "warning"
```

Searches section titles and content *plus* the typed AST: code block languages, callout variants, table headers, math TeX, footnote IDs, link labels, HTML content. Much broader than a plain grep.

### Focused section view

```bash
docmap file.md --section "Installation"    # Just that subtree
docmap file.md --expand "Installation"      # Raw content of that section
```

### Cross-file references

```bash
docmap . --refs
```

Shows which docs link to which other docs — useful for understanding doc graph topology.

### JSON output

```bash
docmap file.md --json
docmap . --json | jq '.documents[] | select(.summary.code_blocks > 10)'
```

Full typed AST including per-section `notables`, top-level `summary` counts, and every node with its kind + line range. Pipe into `jq` or another tool.

## Reading the output

The header box shows the file's full inventory:

```
╭──── file.md ────╮
│ Sections: 30 │ ~2.3k tokens                     │
│ 5 callouts · 4 tables · 8 code blocks · 2 math  │
│ 6 tasks (3 done) · 4 wiki · 6 embeds            │
╰──────────────────────────────────────────────────╯
```

Every section line in the tree has the shape:

```
├── <title> (<tokens>) · <annotation>
```

Where `<annotation>` packs every notable construct in that section:

- `go :142-149, python :154-156, bash :161-162` — code blocks by language and line range
- `note :219, tip :222, warning :228` — callouts by variant and line
- `4 tables :108 Name, :116 Left, :123 Field` — tables with first-header identification
- `^1 :276, ^bignote :277` — footnote definitions by ID
- `6 tasks (3 done)` — aggregate task list counts
- `4 wiki · 6 embeds` — Obsidian aggregate counts

**The `:NNN` format is a 1-indexed source line.** Same convention as `grep`, `ripgrep`, `vim`, and compiler errors. `:154-156` means "lines 154 through 156."

## Typical agent workflows

**Workflow: skim before read**
```bash
docmap unfamiliar.md                    # Get the structure
docmap unfamiliar.md --at 247           # What's at the section I care about?
# Now do a targeted Read on just lines 240-280
```

**Workflow: find every instance of a construct**
```bash
docmap huge-doc.md --type code --lang go        # Every Go snippet
docmap huge-doc.md --type callout --kind warning # Every warning
```

**Workflow: understand changes**
```bash
git checkout feature-branch
docmap docs/api.md --since main         # What did this PR change?
```

**Workflow: project-wide discovery**
```bash
docmap .                                # Header shows aggregate totals
# "there are 41 code blocks and 18 callouts in this repo"
# Decide which file is worth opening based on the per-file digest
```

**Workflow: structured consumption**
```bash
docmap file.md --json | jq '.documents[0].nodes[] | select(.kind == "code_block")'
```

## What docmap recognizes

Full CommonMark + GitHub Flavored Markdown + Obsidian extensions. Block constructs: frontmatter (YAML/TOML/JSON), ATX + Setext headings, paragraphs, blockquotes, GFM callouts (5 kinds), ordered/unordered/task lists, tables with alignment, fenced + indented code blocks, math blocks, footnote definitions, definition lists, HTML blocks, thematic breaks, link reference definitions. Inline constructs: links, autolinks (angle-bracket + GFM bare + email), images, emphasis/strong/strikethrough, inline code, inline math, footnote refs, wiki links (`[[…]]`), wiki embeds (`![[…]]`), mentions (`@user`), issue refs (`#123`, `user/repo#123`, `GH-123`), commit SHAs, emoji shortcodes, HTML entities.

## Pair with codemap

```bash
codemap .   # code structure (filesystem + symbols)
docmap .    # doc structure (sections + notables)
```

Together, they give an agent full spatial awareness of a repository without reading any source file first.
