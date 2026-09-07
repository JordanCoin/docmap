package parser

import (
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// goldmarkPool reuses configured goldmark.Markdown instances. goldmark is not
// goroutine-safe, so each Parse borrows one instance exclusively and returns
// it afterward.
var goldmarkPool = sync.Pool{
	New: func() any {
		return newGoldmark()
	},
}

func newGoldmark() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			extension.DefinitionList,
		),
	)
}

func getGoldmark() goldmark.Markdown {
	return goldmarkPool.Get().(goldmark.Markdown)
}

func putGoldmark(md goldmark.Markdown) {
	goldmarkPool.Put(md)
}
