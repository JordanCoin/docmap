# docmap

Run `docmap .` at the start of any session to understand documentation structure.

## When to Use

- Before reading large markdown files
- When exploring a new repository's docs
- To find where specific topics are documented
- When you need to understand doc structure without burning tokens

## Commands

```bash
docmap .                          # Map all markdown files
docmap docs/                      # Map specific folder
docmap README.md                  # Deep dive single file
docmap README.md --section "API"  # Filter to section
docmap README.md --expand "API"   # Show section content
```

## Pair with codemap

```bash
codemap .   # code structure
docmap .    # doc structure
```

Together: complete project understanding.
