package mcp

import (
	"context"
	"strings"
	"testing"
)

type fakeRunner struct {
	lastArgs []string
	out      string
	err      error
}

func (f *fakeRunner) Run(_ context.Context, args ...string) (string, error) {
	f.lastArgs = append([]string(nil), args...)
	return f.out, f.err
}

func TestBuildBriefArgs(t *testing.T) {
	r := &fakeRunner{out: "ok"}
	server := NewServer("test", r)
	if server == nil {
		t.Fatal("expected server")
	}

	h := makeBrief(r)
	_, _, err := h(context.Background(), nil, briefArgs{Path: ".", Stale: true, Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".", "--brief", "--stale", "--days", "30"}
	if strings.Join(r.lastArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", r.lastArgs, want)
	}
}

func TestTreeRequiresPath(t *testing.T) {
	r := &fakeRunner{out: "{}"}
	res, _, err := makeTree(r)(context.Background(), nil, pathArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected path error")
	}
	if len(r.lastArgs) != 0 {
		t.Fatalf("runner should not run, got %v", r.lastArgs)
	}
}

func TestSinceArgs(t *testing.T) {
	r := &fakeRunner{out: "{}"}
	_, _, err := makeSince(r)(context.Background(), nil, sinceArgs{Path: "README.md", Ref: "HEAD~1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"README.md", "--since", "HEAD~1", "--json"}
	if strings.Join(r.lastArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", r.lastArgs, want)
	}
}

func TestStaleRemoteArgs(t *testing.T) {
	r := &fakeRunner{out: "[]"}
	_, _, err := makeStale(r)(context.Background(), nil, staleArgs{Path: ".", Remote: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".", "--stale", "--json", "--remote"}
	if strings.Join(r.lastArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", r.lastArgs, want)
	}
}

func TestSearchAndAtLineValidation(t *testing.T) {
	r := &fakeRunner{}
	res, _, err := makeSearch(r)(context.Background(), nil, searchArgs{Path: "."})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected query error")
	}

	res, _, err = makeAtLine(r)(context.Background(), nil, atLineArgs{Path: "README.md", Line: 0})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.IsError {
		t.Fatal("expected line error")
	}
}
