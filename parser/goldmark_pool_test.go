package parser

import (
	"sync"
	"testing"
)

func TestGoldmarkPoolConcurrent(t *testing.T) {
	const workers = 32
	const iters = 20
	src := []byte("# Title\n\nParagraph with a [link](https://example.com).\n\n" +
		"| A | B |\n| --- | --- |\n| 1 | 2 |\n\n" +
		"[^1]: footnote\n\nTerm\n: definition\n")

	var wg sync.WaitGroup
	errs := make(chan string, workers*iters)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				nodes := parseWithGoldmark(src)
				if len(nodes) == 0 {
					errs <- "expected non-empty parse"
					return
				}
				foundHeading := false
				for _, n := range nodes {
					if n.Kind() == KindHeading {
						foundHeading = true
						break
					}
				}
				if !foundHeading {
					errs <- "expected a heading node"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
}
