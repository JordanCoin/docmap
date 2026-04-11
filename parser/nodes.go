package parser

// NodeKind enumerates every markdown construct docmap recognizes.
// The taxonomy covers CommonMark + GFM + Obsidian wiki links/embeds.
type NodeKind string

const (
	KindFrontmatter    NodeKind = "frontmatter"
	KindHeading        NodeKind = "heading"
	KindParagraph      NodeKind = "paragraph"
	KindBlockquote     NodeKind = "blockquote"
	KindCallout        NodeKind = "callout"
	KindList           NodeKind = "list"
	KindListItem       NodeKind = "list_item"
	KindTaskItem       NodeKind = "task_item"
	KindTable          NodeKind = "table"
	KindTableRow       NodeKind = "table_row"
	KindTableCell      NodeKind = "table_cell"
	KindCodeBlock      NodeKind = "code_block"
	KindMathBlock      NodeKind = "math_block"
	KindThematicBreak  NodeKind = "thematic_break"
	KindHTMLBlock      NodeKind = "html_block"
	KindDefinitionList NodeKind = "definition_list"
	KindDefTerm        NodeKind = "definition_term"
	KindDefinition     NodeKind = "definition"
	KindFootnoteDef    NodeKind = "footnote_def"
	KindLinkRefDef     NodeKind = "link_ref_def"

	KindText        NodeKind = "text"
	KindEmphasis    NodeKind = "emphasis"
	KindStrong      NodeKind = "strong"
	KindDelete      NodeKind = "delete"
	KindInlineCode  NodeKind = "inline_code"
	KindLink        NodeKind = "link"
	KindAutoLink    NodeKind = "autolink"
	KindImage       NodeKind = "image"
	KindWikiLink    NodeKind = "wiki_link"
	KindWikiEmbed   NodeKind = "wiki_embed"
	KindFootnoteRef NodeKind = "footnote_ref"
	KindMention     NodeKind = "mention"
	KindIssueRef    NodeKind = "issue_ref"
	KindCommitRef   NodeKind = "commit_ref"
	KindEmoji       NodeKind = "emoji"
	KindLineBreak   NodeKind = "line_break"
	KindEntity      NodeKind = "entity"
	KindInlineMath  NodeKind = "inline_math"
	KindInlineHTML  NodeKind = "inline_html"
)

// CalloutKind identifies the variant of a GFM alert (> [!NOTE] etc).
type CalloutKind string

const (
	CalloutNote      CalloutKind = "note"
	CalloutTip       CalloutKind = "tip"
	CalloutImportant CalloutKind = "important"
	CalloutWarning   CalloutKind = "warning"
	CalloutCaution   CalloutKind = "caution"
)

// TableAlign specifies a column's alignment in a GFM table.
type TableAlign string

const (
	AlignNone   TableAlign = ""
	AlignLeft   TableAlign = "left"
	AlignCenter TableAlign = "center"
	AlignRight  TableAlign = "right"
)

// FrontmatterFormat is the serialization format of a frontmatter block.
type FrontmatterFormat string

const (
	FrontmatterYAML FrontmatterFormat = "yaml"
	FrontmatterTOML FrontmatterFormat = "toml"
	FrontmatterJSON FrontmatterFormat = "json"
)

// Node is the common interface every markdown construct implements.
// Concrete types embed BaseNode to satisfy it.
type Node interface {
	Kind() NodeKind
	LineStart() int
	LineEnd() int
	Tokens() int
	Children() []Node
}

// BaseNode holds fields common to every node and is embedded in each
// concrete type. Its exported fields let constructors populate state
// without per-type setters.
type BaseNode struct {
	NKind    NodeKind
	Start    int
	End      int
	TokCount int
	Kids     []Node
}

func (b *BaseNode) Kind() NodeKind   { return b.NKind }
func (b *BaseNode) LineStart() int   { return b.Start }
func (b *BaseNode) LineEnd() int     { return b.End }
func (b *BaseNode) Tokens() int      { return b.TokCount }
func (b *BaseNode) Children() []Node { return b.Kids }

// ---------- Block-level nodes ----------

// Frontmatter is a YAML/TOML/JSON header block at the top of a file.
type Frontmatter struct {
	BaseNode
	Format FrontmatterFormat
	Raw    string
}

// Heading is an ATX (# Title) or Setext (underline) heading.
type Heading struct {
	BaseNode
	Level    int
	Title    string
	RawTitle string
	IsSetext bool
	ID       string
}

// Paragraph is a block of flowing text.
// Text is the inline-rendered form (newlines collapsed); Raw preserves
// the original multi-line source so post-passes (math fences, etc.)
// can operate on real line boundaries.
type Paragraph struct {
	BaseNode
	Text string
	Raw  string
}

// Blockquote is a > quoted block, possibly nested.
type Blockquote struct {
	BaseNode
}

// Callout is a GFM alert (> [!NOTE] / [!TIP] / [!IMPORTANT] / [!WARNING] / [!CAUTION]).
type Callout struct {
	BaseNode
	Variant CalloutKind
}

// List is an ordered or unordered list.
type List struct {
	BaseNode
	Ordered bool
	Tight   bool
	Start   int
	Marker  rune
}

// ListItem is a regular item inside a list.
type ListItem struct {
	BaseNode
}

// TaskItem is a GFM task list item ([ ] or [x]).
type TaskItem struct {
	BaseNode
	Checked bool
}

// Table is a GFM table.
type Table struct {
	BaseNode
	Headers []string
	Aligns  []TableAlign
}

// TableRow is a single row of a table.
type TableRow struct {
	BaseNode
	IsHeader bool
}

// TableCell is one cell within a row.
type TableCell struct {
	BaseNode
	Align TableAlign
}

// CodeBlock is a fenced or indented code block.
type CodeBlock struct {
	BaseNode
	Language string
	Info     string
	Fenced   bool
	Code     string
}

// MathBlock is a block-level math expression ($$...$$ or \[...\]).
type MathBlock struct {
	BaseNode
	TeX   string
	Fence string
}

// ThematicBreak is --- / *** / ___.
type ThematicBreak struct {
	BaseNode
}

// HTMLBlock is a raw HTML block (includes <details>, <div>, comments).
type HTMLBlock struct {
	BaseNode
	Raw string
}

// DefinitionList is a Pandoc-style definition list.
type DefinitionList struct {
	BaseNode
}

// DefinitionTerm is the term being defined.
type DefinitionTerm struct {
	BaseNode
	Term string
}

// Definition is a single definition for a term.
type Definition struct {
	BaseNode
}

// FootnoteDef is a footnote definition ([^id]: content).
type FootnoteDef struct {
	BaseNode
	ID string
}

// LinkRefDef is a reference link definition ([label]: url "title").
type LinkRefDef struct {
	BaseNode
	Label string
	URL   string
	Title string
}

// ---------- Inline nodes ----------

// Text is a plain run of characters.
type Text struct {
	BaseNode
	Value string
}

// Emphasis is *text* or _text_.
type Emphasis struct {
	BaseNode
}

// Strong is **text** or __text__.
type Strong struct {
	BaseNode
}

// Delete is ~~text~~ (GFM strikethrough).
type Delete struct {
	BaseNode
}

// InlineCode is `text`.
type InlineCode struct {
	BaseNode
	Code string
}

// Link is an inline or reference link.
type Link struct {
	BaseNode
	URL   string
	Title string
	Text  string
	RefID string
}

// AutoLink is <url> or a GFM bare-URL autolink.
type AutoLink struct {
	BaseNode
	URL    string
	IsBare bool
	IsMail bool
}

// Image is an inline or reference image.
type Image struct {
	BaseNode
	URL   string
	Alt   string
	Title string
	RefID string
}

// WikiLink is [[Page]] / [[Page|alias]] / [[Page#header]] / [[Page#^block]] (Obsidian).
type WikiLink struct {
	BaseNode
	Target string
	Alias  string
	Anchor string
	Block  string
}

// WikiEmbed is ![[file]] / ![[file|200]] / ![[file|200x100]] (Obsidian).
type WikiEmbed struct {
	BaseNode
	Target string
	Width  int
	Height int
}

// FootnoteRef is [^id] in flowing text.
type FootnoteRef struct {
	BaseNode
	ID string
}

// Mention is @user (GFM).
type Mention struct {
	BaseNode
	User string
}

// IssueRef is #123 / GH-123 / user/repo#123 (GFM).
type IssueRef struct {
	BaseNode
	Repo   string
	Number int
}

// CommitRef is a SHA autolink (GFM).
type CommitRef struct {
	BaseNode
	SHA string
}

// Emoji is :name: (GFM).
type Emoji struct {
	BaseNode
	Name string
}

// LineBreak is a hard (two-space / backslash) or soft line break.
type LineBreak struct {
	BaseNode
	Hard bool
}

// Entity is an HTML entity (&copy;, &#169;, &#x2665;).
type Entity struct {
	BaseNode
	Raw     string
	Decoded string
}

// InlineMath is $...$.
type InlineMath struct {
	BaseNode
	TeX string
}

// InlineHTML is raw inline HTML like <kbd>.
type InlineHTML struct {
	BaseNode
	Raw string
}

// ---------- Document summary ----------

// ContentSummary is an at-a-glance inventory of notable constructs across
// an entire Document. Used by the renderer to draw the file header.
type ContentSummary struct {
	Callouts     int
	Tables       int
	CodeBlocks   int
	MathBlocks   int
	HTMLBlocks   int
	Footnotes    int
	DefLists     int
	LinkRefDefs  int
	Tasks        int
	TasksChecked int
	WikiLinks    int
	WikiEmbeds   int
	Mentions     int
	IssueRefs    int
	CommitRefs   int
	Emojis       int
}

// ---------- Traversal ----------

// Walk invokes fn for every node reachable from root, depth-first.
// Returning false from fn skips descending into that node's children.
func Walk(root Node, fn func(Node) bool) {
	if root == nil {
		return
	}
	if !fn(root) {
		return
	}
	for _, child := range root.Children() {
		Walk(child, fn)
	}
}

// FindByKind returns every descendant of root whose Kind matches kind.
func FindByKind(root Node, kind NodeKind) []Node {
	var out []Node
	Walk(root, func(n Node) bool {
		if n.Kind() == kind {
			out = append(out, n)
		}
		return true
	})
	return out
}
