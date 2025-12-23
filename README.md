# 🗺️ docmap

> **docmap — instant documentation structure for LLMs and humans.**
> Navigate massive docs without burning tokens.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go](https://img.shields.io/badge/go-1.21+-00ADD8.svg)

## The Problem

Documentation files are everywhere — READMEs, design docs, changelogs, API references. But:

- LLMs can't open large markdown files (token limits)
- Humans have to open each file to see what's inside
- There's no "file tree" for documentation *content*

## The Solution

```bash
docmap .
```

```
╭─────────────────────── docs/ ───────────────────────╮
│      22 files | 645 sections | ~109.2k tokens       │
╰─────────────────────────────────────────────────────╯

├── README.md (3.8k)
│   ├─ Project Overview
│   ├─ Installation
│   └─ Quick Start
├── docs/ARCHITECTURE.md (7.7k)
│   ├─ System Design
│   ├─ Component Details
│   └─ Data Flow
├── docs/API.md (12.1k)
│   ├─ Authentication
│   ├─ Endpoints
│   └─ Error Handling
└── CHANGELOG.md (15.2k)
    ├─ v2.0.0
    ├─ v1.5.0
    └─ v1.0.0
```

One command. Instant structure. No LLM needed.

## Install

```bash
# macOS/Linux
brew tap JordanCoin/tap && brew install docmap

# Windows
scoop bucket add docmap https://github.com/JordanCoin/scoop-docmap
scoop install docmap
```

> Other options: [Releases](https://github.com/JordanCoin/docmap/releases) | `go install github.com/JordanCoin/docmap@latest`

## Usage

```bash
docmap .                          # All markdown files in directory
docmap README.md                  # Single file deep dive
docmap docs/                      # Specific folder
docmap README.md --section "API"  # Filter to section
docmap README.md --expand "API"   # Show section content
docmap . --refs                   # Show cross-references between docs
```

## Output

### Directory Mode

Map all markdown files in a project:

```bash
docmap /path/to/project
```

Shows each file with token count and top-level sections.

### Single File Mode

Deep dive into one document:

```bash
docmap docs/ARCHITECTURE.md
```

```
╭────────────────── ARCHITECTURE.md ──────────────────╮
│           33 sections | ~7.7k tokens                │
╰─────────────────────────────────────────────────────╯

├── System Design (2.1k)
│   ├── Vision
│   ├── Core Principles
│   │   └─ "Headless-first", "Plan before execute"
│   └── Architecture Overview
├── Components (3.2k)
│   ├── Scheduler
│   ├── Orchestrator
│   └── Memory (RAG)
└── Security (1.4k)
    └─ "SSH keys only", "Human gates"
```

### Section Filter

Zoom into a specific section:

```bash
docmap docs/API.md --section "Authentication"
```

### Expand Section

See the actual content:

```bash
docmap docs/API.md --expand "Authentication"
```

### References Mode

See how docs link to each other (like `codemap --deps`):

```bash
docmap . --refs
```

```
╭─────────────────────── project/ ───────────────────────╮
│              References: 53 links between docs         │
╰────────────────────────────────────────────────────────╯

HUBS: docs/architecture.md (5←), docs/api.md (3←)

Reference Flow:

  README.md
  ├──▶ docs/architecture.md
  ├──▶ docs/api.md
  └──▶ CHANGELOG.md

  docs/architecture.md
  ├──▶ docs/components.md
  └──▶ docs/data-flow.md
```

## Why docmap?

| Before | After |
|--------|-------|
| "Read this 100k token doc" | "Here's the structure, ask for what you need" |
| Open 20 files to find something | See all sections at a glance |
| Scroll through giant CHANGELOGs | Jump to the version you need |
| Guess what's in each doc | Token counts show what's meaty |

## Sister Tool

docmap is the documentation companion to [codemap](https://github.com/JordanCoin/codemap):

```bash
codemap .   # code structure
docmap .    # doc structure
```

Together: complete spatial awareness of any repository.

## How It Works

1. **Parse** markdown headings into a tree structure
2. **Estimate** tokens per section (~4 chars/token)
3. **Extract** key terms (bold text, inline code)
4. **Render** as a navigable tree

No external dependencies. No API calls. Just fast, local parsing.

## Contributing

1. Fork → 2. Branch → 3. Commit → 4. PR

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

MIT
