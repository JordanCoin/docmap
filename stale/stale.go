package stale

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JordanCoin/docmap/parser"
)

// Finding is one suspect claim in a documentation section.
type Finding struct {
	File    string `json:"file"`
	Section string `json:"section"`
	Reason  string `json:"reason"`
	Kind    string `json:"kind"`
}

// Options controls local and optional remote stale checks.
type Options struct {
	Root         string
	Days         int // status-heading dates older than this are stale; default 90
	CheckFlags   bool
	Remote       bool
	AllowDomains []string
	Timeout      time.Duration
	Now          time.Time

	LookPath func(file string) (string, error)
	HTTPHead func(url string) (status int, err error)
}

// Check walks every section in docs and returns sorted findings.
func Check(docs []*parser.Document, opt Options) []Finding {
	opt = normalize(opt)
	cfg := loadConfigIndex(opt.Root)
	helpCache := &sync.Map{}
	var out []Finding
	for _, doc := range docs {
		docRoot := filepath.Join(opt.Root, filepath.Dir(doc.Filename))
		for _, sec := range doc.GetAllSections() {
			text := sectionText(doc, sec)
			if text == "" {
				continue
			}
			crumb := breadcrumb(sec)
			out = append(out, checkPaths(doc.Filename, crumb, text, opt, docRoot)...)
			out = append(out, checkCommands(doc.Filename, crumb, text, opt, helpCache)...)
			out = append(out, checkDates(doc.Filename, crumb, sec, text, opt)...)
			out = append(out, checkEnvKeys(doc.Filename, crumb, text, cfg)...)
			if opt.Remote {
				out = append(out, checkURLs(doc.Filename, crumb, text, doc, opt)...)
				out = append(out, checkVersions(doc.Filename, crumb, text, opt)...)
				out = append(out, checkConfigValues(doc.Filename, crumb, text, cfg)...)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Section != out[j].Section {
			return out[i].Section < out[j].Section
		}
		return out[i].Reason < out[j].Reason
	})
	return dedupe(out)
}

func normalize(opt Options) Options {
	if opt.Days <= 0 {
		opt.Days = 90
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 3 * time.Second
	}
	if opt.LookPath == nil {
		opt.LookPath = exec.LookPath
	}
	if opt.HTTPHead == nil {
		client := &http.Client{Timeout: opt.Timeout}
		opt.HTTPHead = func(url string) (int, error) {
			req, err := http.NewRequest(http.MethodHead, url, nil)
			if err != nil {
				return 0, err
			}
			resp, err := client.Do(req)
			if err != nil {
				// Some hosts reject HEAD; fall back to GET.
				req, err = http.NewRequest(http.MethodGet, url, nil)
				if err != nil {
					return 0, err
				}
				resp, err = client.Do(req)
				if err != nil {
					return 0, err
				}
			}
			defer resp.Body.Close()
			return resp.StatusCode, nil
		}
	}
	if abs, err := filepath.Abs(opt.Root); err == nil {
		opt.Root = abs
	}
	return opt
}

func sectionText(doc *parser.Document, sec *parser.Section) string {
	if span := doc.SourceSpan(sec.LineStart, sec.LineEnd); span != "" {
		return span
	}
	return sec.Title + "\n" + sec.Content
}

func breadcrumb(sec *parser.Section) string {
	var parts []string
	for s := sec; s != nil; s = s.Parent {
		parts = append([]string{s.Title}, parts...)
	}
	return strings.Join(parts, " > ")
}

func dedupe(in []Finding) []Finding {
	seen := map[string]bool{}
	var out []Finding
	for _, f := range in {
		key := f.File + "\x00" + f.Section + "\x00" + f.Reason
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

var backtickRe = regexp.MustCompile("`([^`\n]{1,200})`")

func backticks(text string) []string {
	ms := backtickRe.FindAllStringSubmatch(text, -1)
	var out []string
	for _, m := range ms {
		s := strings.TrimSpace(m[1])
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func looksLikePath(s string) bool {
	if strings.ContainsAny(s, " \t|;&<>#@![]{}*?^") {
		return false
	}
	if strings.HasPrefix(s, "--") || strings.HasPrefix(s, "-") {
		return false
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return false
	}
	if strings.Contains(s, "/") || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		// owner/repo without a file extension is usually a GitHub slug, not a path
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(s, "./"), "../"), "/")
		if len(parts) == 2 && !strings.Contains(parts[1], ".") && !strings.HasPrefix(s, "./") && !strings.HasPrefix(s, "../") && !strings.HasPrefix(s, "~/") {
			return false
		}
		return true
	}
	lower := strings.ToLower(s)
	for _, ext := range []string{".md", ".go", ".py", ".js", ".ts", ".tsx", ".json", ".yaml", ".yml", ".toml", ".sh", ".bash", ".zsh", ".env", ".txt", ".pdf", ".html", ".css", ".rs", ".rb", ".java", ".kt", ".swift", ".c", ".h", ".cpp", ".mod", ".sum", ".lock", ".conf", ".cfg", ".ini", ".xml", ".sql", ".proto", ".graphql", ".dockerfile"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func checkPaths(file, section, text string, opt Options, docDir string) []Finding {
	home, _ := os.UserHomeDir()
	repo := parser.GitRoot(opt.Root)
	var out []Finding
	for _, tick := range backticks(text) {
		if !looksLikePath(tick) {
			continue
		}
		if pathExists(tick, opt.Root, repo, docDir, home) {
			continue
		}
		out = append(out, Finding{
			File:    file,
			Section: section,
			Kind:    "missing_path",
			Reason:  fmt.Sprintf("missing path `%s`", tick),
		})
	}
	return out
}

func pathExists(raw, root, repo, docDir, home string) bool {
	cand := expandPath(raw, home)
	bases := []string{docDir, root}
	if repo != "" {
		bases = append(bases, repo)
	}
	if filepath.IsAbs(cand) {
		if st, err := os.Stat(cand); err == nil && (st.Mode().IsRegular() || st.IsDir()) {
			return true
		}
		return false
	}
	for _, base := range bases {
		if base == "" {
			continue
		}
		p := filepath.Join(base, filepath.FromSlash(cand))
		if st, err := os.Stat(p); err == nil && (st.Mode().IsRegular() || st.IsDir()) {
			return true
		}
	}
	return false
}

func expandPath(raw, home string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "~/") && home != "" {
		return filepath.Join(home, raw[2:])
	}
	return raw
}

var cmdSkip = map[string]bool{
	"cd": true, "echo": true, "export": true, "set": true, "unset": true,
	"source": true, ".": true, "true": true, "false": true, "exit": true,
	"then": true, "fi": true, "do": true, "done": true, "if": true, "elif": true,
	"else": true, "for": true, "while": true, "case": true, "esac": true,
}

func checkCommands(file, section, text string, opt Options, helpCache *sync.Map) []Finding {
	var out []Finding
	for _, tick := range backticks(text) {
		bin, flags := parseCommand(tick)
		if bin == "" || cmdSkip[bin] {
			continue
		}
		if strings.Contains(bin, "/") || looksLikePath(bin) {
			continue
		}
		if _, err := opt.LookPath(bin); err != nil {
			out = append(out, Finding{
				File:    file,
				Section: section,
				Kind:    "missing_binary",
				Reason:  fmt.Sprintf("binary `%s` not on PATH", bin),
			})
			continue
		}
		if !opt.CheckFlags || len(flags) == 0 {
			continue
		}
		help := commandHelp(bin, opt, helpCache)
		for _, flag := range flags {
			if !helpHasFlag(help, flag) {
				out = append(out, Finding{
					File:    file,
					Section: section,
					Kind:    "missing_flag",
					Reason:  fmt.Sprintf("flag `%s` not in `%s --help`", flag, bin),
				})
			}
		}
	}
	return out
}

var cmdNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

func parseCommand(tick string) (bin string, flags []string) {
	// Only treat backtick text as a command when it looks like an invocation
	// (binary, or binary + args). Lone words used as prose/examples are skipped
	// unless they contain flags or multiple fields.
	fields := strings.Fields(tick)
	if len(fields) == 0 {
		return "", nil
	}
	bin = fields[0]
	if strings.HasPrefix(bin, "$") || strings.HasPrefix(bin, "${") {
		return "", nil
	}
	if i := strings.LastIndexAny(bin, "/\\"); i >= 0 {
		bin = bin[i+1:]
	}
	if !cmdNameRe.MatchString(bin) || cmdSkip[bin] || !likelyCLI(bin) {
		return "", nil
	}
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "--") && len(f) > 2 && !strings.Contains(f, "=") {
			flags = append(flags, f)
		} else if strings.HasPrefix(f, "--") && strings.Contains(f, "=") {
			flags = append(flags, strings.SplitN(f, "=", 2)[0])
		}
	}
	return bin, flags
}

func likelyCLI(bin string) bool {
	switch strings.ToLower(bin) {
	case "git", "go", "npm", "npx", "yarn", "pnpm", "pip", "pip3", "python", "python3",
		"node", "ruby", "cargo", "rustc", "docker", "docker-compose", "kubectl", "helm",
		"make", "cmake", "curl", "wget", "jq", "rg", "ripgrep", "fd", "sed", "awk", "grep",
		"gh", "aws", "gcloud", "az", "terraform", "ansible", "brew", "apt", "yum", "pacman",
		"systemctl", "ssh", "scp", "rsync", "tar", "zip", "unzip", "openssl", "sqlite3",
		"psql", "mysql", "redis-cli", "mongosh", "firebase", "vercel", "netlify", "wrangler",
		"docmap", "codemap", "eslint", "prettier", "tsc", "webpack", "vite", "next",
		"poetry", "uv", "bun", "deno", "java", "javac", "mvn", "gradle", "dotnet", "swift",
		"xcodebuild", "pod", "flutter", "dart", "php", "composer", "lua", "perl", "R",
		"bazel", "ninja", "meson", "pkg-config", "clang", "gcc", "g++", "lldb", "gdb":
		return true
	default:
		return false
	}
}

func commandHelp(bin string, opt Options, cache *sync.Map) string {
	if v, ok := cache.Load(bin); ok {
		return v.(string)
	}
	cmd := exec.Command(bin, "--help")
	cmd.Env = append(os.Environ(), "TERM=dumb")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && len(text) == 0 {
		cmd = exec.Command(bin, "-h")
		cmd.Env = append(os.Environ(), "TERM=dumb")
		out, _ = cmd.CombinedOutput()
		text = string(out)
	}
	cache.Store(bin, text)
	return text
}

func helpHasFlag(help, flag string) bool {
	if help == "" {
		return true // unknown help → don't flag
	}
	return strings.Contains(help, flag)
}

var (
	isoDateRe   = regexp.MustCompile(`\b(20\d{2})-(\d{2})-(\d{2})\b`)
	slashDateRe = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})/(20\d{2})\b`)
	monthDateRe = regexp.MustCompile(`(?i)\b(Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:t(?:ember)?)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?)\s+(\d{1,2})(?:st|nd|rd|th)?,?\s+(20\d{2})\b`)
	statusHead  = regexp.MustCompile(`(?i)\b(status|last checked|verified|baseline|as of|updated|last update|checked)\b`)
)

func isStatusHeading(title string) bool {
	return statusHead.MatchString(title)
}

func checkDates(file, section string, sec *parser.Section, text string, opt Options) []Finding {
	if !isStatusHeading(sec.Title) && !ancestorStatus(sec) {
		return nil
	}
	cutoff := opt.Now.AddDate(0, 0, -opt.Days)
	var out []Finding
	for _, d := range extractDates(text, opt.Now) {
		if d.Before(cutoff) {
			out = append(out, Finding{
				File:    file,
				Section: section,
				Kind:    "stale_date",
				Reason:  fmt.Sprintf("date %s is older than %d days", d.Format("2006-01-02"), opt.Days),
			})
		}
	}
	return out
}

func ancestorStatus(sec *parser.Section) bool {
	for s := sec; s != nil; s = s.Parent {
		if isStatusHeading(s.Title) {
			return true
		}
	}
	return false
}

func extractDates(text string, now time.Time) []time.Time {
	var out []time.Time
	for _, m := range isoDateRe.FindAllStringSubmatch(text, -1) {
		t, err := time.ParseInLocation("2006-01-02", m[0], now.Location())
		if err == nil {
			out = append(out, t)
		}
	}
	for _, m := range monthDateRe.FindAllStringSubmatch(text, -1) {
		raw := fmt.Sprintf("%s %s %s", m[1], m[2], m[3])
		for _, layout := range []string{"January 2 2006", "Jan 2 2006", "January 2, 2006", "Jan 2, 2006"} {
			t, err := time.ParseInLocation(layout, raw, now.Location())
			if err == nil {
				out = append(out, t)
				break
			}
		}
	}
	_ = slashDateRe
	return out
}

var envTickRe = regexp.MustCompile(`\$\{?([A-Z][A-Z0-9_]{1,63})\}?`)
var envWordRe = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{2,63})\b`)

type configIndex struct {
	keys   map[string]bool
	values map[string]map[string]bool // key -> set of values seen
}

func loadConfigIndex(root string) configIndex {
	idx := configIndex{keys: map[string]bool{}, values: map[string]map[string]bool{}}
	patterns := []string{
		".env", ".env.example", ".env.sample", ".env.template",
		"*.env.example", ".env.*",
		"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml",
		"config.yaml", "config.yml", "config.toml", "config.json",
		"app.yaml", "app.yml", "settings.json", "settings.yaml",
	}
	var files []string
	for _, p := range patterns {
		if strings.ContainsAny(p, "*?[") {
			matches, _ := filepath.Glob(filepath.Join(root, p))
			files = append(files, matches...)
			continue
		}
		files = append(files, filepath.Join(root, p))
	}
	// Also scan one level of config/ and .config/
	for _, dir := range []string{"config", ".config", "configs", "deploy", "infra"} {
		_ = filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			name := strings.ToLower(info.Name())
			if strings.Contains(name, "env") || strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") ||
				strings.HasSuffix(name, ".toml") || strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".conf") ||
				strings.HasSuffix(name, ".ini") || strings.HasSuffix(name, ".env") {
				files = append(files, path)
			}
			return nil
		})
	}
	seenFile := map[string]bool{}
	for _, f := range files {
		if seenFile[f] {
			continue
		}
		seenFile[f] = true
		ingestConfigFile(f, &idx)
	}
	return idx
}

var (
	envLineRe    = regexp.MustCompile(`(?m)^\s*(?:export\s+)?([A-Z][A-Z0-9_]{1,63})\s*=\s*(.*)$`)
	yamlKeyRe    = regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9_.-]{1,63})\s*:\s*(.+?)\s*$`)
	tomlKeyRe    = regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9_]{1,63})\s*=\s*(.+?)\s*$`)
	composeEnvRe = regexp.MustCompile(`(?m)^\s*-\s*([A-Z][A-Z0-9_]{1,63})=(.*)$`)
)

func ingestConfigFile(path string, idx *configIndex) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	text := string(content)
	addKV := func(k, v string) {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if k == "" {
			return
		}
		idx.keys[k] = true
		idx.keys[strings.ToUpper(k)] = true
		if v != "" && v != "|" && v != ">" && !strings.HasPrefix(v, "#") {
			if idx.values[k] == nil {
				idx.values[k] = map[string]bool{}
			}
			idx.values[k][v] = true
			uk := strings.ToUpper(k)
			if idx.values[uk] == nil {
				idx.values[uk] = map[string]bool{}
			}
			idx.values[uk][v] = true
		}
	}
	for _, m := range envLineRe.FindAllStringSubmatch(text, -1) {
		addKV(m[1], m[2])
	}
	for _, m := range composeEnvRe.FindAllStringSubmatch(text, -1) {
		addKV(m[1], m[2])
	}
	for _, m := range yamlKeyRe.FindAllStringSubmatch(text, -1) {
		addKV(m[1], m[2])
	}
	for _, m := range tomlKeyRe.FindAllStringSubmatch(text, -1) {
		addKV(m[1], m[2])
	}
}

func checkEnvKeys(file, section, text string, cfg configIndex) []Finding {
	if len(cfg.keys) == 0 {
		return nil
	}
	candidates := map[string]bool{}
	addKey := func(k string) {
		k = strings.TrimSpace(k)
		k = strings.TrimPrefix(k, "$")
		k = strings.TrimPrefix(k, "{")
		k = strings.TrimSuffix(k, "}")
		if k == "" || !envWordRe.MatchString(k) {
			return
		}
		candidates[k] = true
	}
	for _, tick := range backticks(text) {
		for _, m := range envTickRe.FindAllStringSubmatch(tick, -1) {
			addKey(m[1])
		}
		if strings.HasPrefix(tick, "$") {
			addKey(tick)
			continue
		}
		if envWordRe.MatchString(tick) && strings.ToUpper(tick) == tick && strings.Contains(tick, "_") {
			addKey(tick)
		}
	}
	for _, m := range envTickRe.FindAllStringSubmatch(text, -1) {
		addKey(m[1])
	}
	var out []Finding
	for k := range candidates {
		if cfg.keys[k] || cfg.keys[strings.ToUpper(k)] {
			continue
		}
		// Skip very common false positives
		if k == "HTTP" || k == "HTTPS" || k == "JSON" || k == "YAML" || k == "URL" || k == "API" || k == "CLI" || k == "GPU" || k == "CPU" || k == "README" || k == "LICENSE" {
			continue
		}
		out = append(out, Finding{
			File:    file,
			Section: section,
			Kind:    "missing_env",
			Reason:  fmt.Sprintf("env/config key `%s` not found in root config/.env.example", k),
		})
	}
	return out
}

var urlRe = regexp.MustCompile(`https?://[^\s)\]>` + "`\"'" + `]+`)

func checkURLs(file, section, text string, doc *parser.Document, opt Options) []Finding {
	urls := map[string]bool{}
	for _, m := range urlRe.FindAllString(text, -1) {
		urls[strings.TrimRight(m, ".,;:")] = true
	}
	for _, ref := range doc.References {
		if strings.HasPrefix(ref.Target, "http://") || strings.HasPrefix(ref.Target, "https://") {
			urls[ref.Target] = true
		}
	}
	var out []Finding
	for u := range urls {
		if !domainAllowed(u, opt.AllowDomains) {
			continue
		}
		status, err := opt.HTTPHead(u)
		if err != nil {
			out = append(out, Finding{
				File: file, Section: section, Kind: "remote_url",
				Reason: fmt.Sprintf("remote url error %s: %v", u, err),
			})
			continue
		}
		if status == 404 || status == 410 || status >= 500 {
			out = append(out, Finding{
				File: file, Section: section, Kind: "remote_url",
				Reason: fmt.Sprintf("remote url %s returned %d", u, status),
			})
		}
	}
	return out
}

func domainAllowed(raw string, allow []string) bool {
	if len(allow) == 0 {
		return true
	}
	host := raw
	if i := strings.Index(raw, "://"); i >= 0 {
		host = raw[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	host = strings.ToLower(host)
	for _, d := range allow {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

var versionNearRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9_-]{1,32})\s+(?:v)?(\d+\.\d+(?:\.\d+)?)\b`)

func checkVersions(file, section, text string, opt Options) []Finding {
	var out []Finding
	seen := map[string]bool{}
	for _, m := range versionNearRe.FindAllStringSubmatch(text, -1) {
		tool, want := m[1], m[2]
		if cmdSkip[tool] || tool == "http" || tool == "https" || tool == "version" {
			continue
		}
		key := tool + "@" + want
		if seen[key] {
			continue
		}
		seen[key] = true
		path, err := opt.LookPath(tool)
		if err != nil {
			continue // missing binary already covered by local check if backticked
		}
		got := toolVersion(path, tool)
		if got == "" {
			continue
		}
		if !strings.Contains(got, want) {
			out = append(out, Finding{
				File: file, Section: section, Kind: "remote_version",
				Reason: fmt.Sprintf("remote version `%s` claims %s but `%s --version` is %q", tool, want, tool, strings.TrimSpace(got)),
			})
		}
	}
	return out
}

func toolVersion(path, tool string) string {
	for _, args := range [][]string{{"--version"}, {"version"}, {"-v"}} {
		cmd := exec.Command(path, args...)
		cmd.Env = append(os.Environ(), "TERM=dumb")
		out, err := cmd.CombinedOutput()
		if len(out) > 0 {
			line := strings.SplitN(string(out), "\n", 2)[0]
			return strings.TrimSpace(line)
		}
		_ = err
	}
	return ""
}

var claimKVRe = regexp.MustCompile("(?m)(?:^|\\s)([A-Za-z][A-Za-z0-9_.-]{1,63})\\s*=\\s*([^\\s`\"']+)|(?:^|\\s)`([A-Za-z][A-Za-z0-9_.-]{1,63})=([^`]+)`")

func checkConfigValues(file, section, text string, cfg configIndex) []Finding {
	if len(cfg.values) == 0 {
		return nil
	}
	var out []Finding
	add := func(k, v string) {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		if k == "" || v == "" {
			return
		}
		vals := cfg.values[k]
		if vals == nil {
			vals = cfg.values[strings.ToUpper(k)]
		}
		if vals == nil {
			return // unknown key — handled by missing_env when applicable
		}
		if vals[v] {
			return
		}
		out = append(out, Finding{
			File: file, Section: section, Kind: "remote_config",
			Reason: fmt.Sprintf("remote config `%s=%s` does not match root config (have %s)", k, v, joinKeys(vals)),
		})
	}
	for _, m := range claimKVRe.FindAllStringSubmatch(text, -1) {
		if m[1] != "" {
			add(m[1], m[2])
		}
		if m[3] != "" {
			add(m[3], m[4])
		}
	}
	for _, tick := range backticks(text) {
		if i := strings.IndexByte(tick, '='); i > 0 {
			add(tick[:i], tick[i+1:])
		}
	}
	return out
}

func joinKeys(m map[string]bool) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	if len(ks) > 5 {
		ks = append(ks[:5], "…")
	}
	return strings.Join(ks, ", ")
}

// FormatLines prints human findings plus a summary line.
func FormatLines(findings []Finding) []string {
	var lines []string
	for _, f := range findings {
		lines = append(lines, fmt.Sprintf("%s > %s: %s", f.File, f.Section, f.Reason))
	}
	return lines
}
