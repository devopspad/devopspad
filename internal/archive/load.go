package archive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	RootFile        = "devopspad.yaml"
	maxResourceSize = 256 << 10
	maxTextSize     = 1 << 20
	maxRecordSize   = 64 << 10
)

// Object is implemented by every resource kind through the embedded Header.
type Object interface{ head() *Header }

type kindInfo struct {
	Kind string // Kind field value
	Ref  string // lowercase reference prefix
	Dir  string // inventory directory; empty for product-scoped kinds
	New  func() Object
}

var kinds = []kindInfo{
	{"Provider", "provider", "inventory/providers", func() Object { return &Provider{} }},
	{"Host", "host", "inventory/hosts", func() Object { return &Host{} }},
	{"Cluster", "cluster", "inventory/clusters", func() Object { return &Cluster{} }},
	{"Datastore", "datastore", "inventory/datastores", func() Object { return &Datastore{} }},
	{"Registry", "registry", "inventory/registries", func() Object { return &Registry{} }},
	{"Domain", "domain", "inventory/domains", func() Object { return &Domain{} }},
	{"Product", "product", "", func() Object { return &Product{} }},
	{"Service", "service", "", func() Object { return &Service{} }},
}

func kindByRef(ref string) (kindInfo, bool) {
	ref = strings.ToLower(strings.TrimSuffix(ref, "s"))
	if ref == "registrie" {
		ref = "registry"
	}
	for _, k := range kinds {
		if k.Ref == ref {
			return k, true
		}
	}
	return kindInfo{}, false
}

// Resource is one loaded resource file.
type Resource struct {
	Ref    string `json:"ref"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	File   string `json:"file"`
	Object Object `json:"object"`
	raw    []byte
	node   *yaml.Node
}

func (r *Resource) Header() *Header { return r.Object.head() }

// Raw returns the file bytes as stored.
func (r *Resource) Raw() []byte { return r.raw }

// Note is a Markdown file under notes/ or products/<p>/notes/.
type Note struct {
	File string   `json:"file"`
	Meta NoteMeta `json:"meta"`
}

// Issue is one validation finding. Paths use YAML field names.
type Issue struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Path    string `json:"path,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (i Issue) String() string {
	loc := i.File
	if i.Line > 0 {
		loc += ":" + strconv.Itoa(i.Line)
	}
	if i.Path != "" {
		loc += " " + i.Path
	}
	return fmt.Sprintf("%s: [%s] %s", loc, i.Code, i.Message)
}

// RefUse is one reference from a resource field to another resource.
type RefUse struct {
	From  string `json:"from"`
	Field string `json:"field"`
	To    string `json:"to"` // canonical target, without #part
	Part  string `json:"part,omitempty"`
	File  string `json:"file"`
}

// SecretUse is one secretRef found in the archive.
type SecretUse struct {
	From  string `json:"from"`
	Field string `json:"field"`
	Ref   string `json:"ref"`
	File  string `json:"file"`
}

// Archive is a fully loaded and validated archive directory.
type Archive struct {
	Dir       string          `json:"dir"`
	Root      *Root           `json:"root"`
	Resources []*Resource     `json:"resources"`
	Notes     []*Note         `json:"notes"`
	Records   []*ActionRecord `json:"records"`
	Uses      []RefUse        `json:"-"`
	Secrets   []SecretUse     `json:"-"`
	Errors    []Issue         `json:"errors"`
	Warnings  []Issue         `json:"warnings"`
	byRef     map[string]*Resource
	pending   []pendingRef
	// closureCache holds each product's dependency closure; see closures.
	closureCache map[string]map[string]bool
	textFiles    map[string][]byte // configs and notes, for secret scanning and existence checks
}

// Get returns a resource by canonical reference.
func (a *Archive) Get(ref string) *Resource { return a.byRef[ref] }

func (a *Archive) errorf(file string, line int, p, code, format string, args ...any) {
	a.Errors = append(a.Errors, Issue{File: file, Line: line, Path: p, Code: code, Message: fmt.Sprintf(format, args...)})
}
func (a *Archive) warnf(file string, line int, p, code, format string, args ...any) {
	a.Warnings = append(a.Warnings, Issue{File: file, Line: line, Path: p, Code: code, Message: fmt.Sprintf(format, args...)})
}

// Find locates an archive root: the explicit directory, DEVOPSPAD_ARCHIVE,
// the nearest parent of the working directory containing devopspad.yaml, or the
// path saved in <config dir>/archive-path.
func Find(explicit, configDir string) (string, error) {
	check := func(dir string) (string, error) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, RootFile)); err != nil {
			return "", fmt.Errorf("%s has no %s; run devopspadcli archive init or pass --archive", abs, RootFile)
		}
		return abs, nil
	}
	if explicit != "" {
		return check(explicit)
	}
	if env := os.Getenv("DEVOPSPAD_ARCHIVE"); env != "" {
		return check(env)
	}
	if wd, err := os.Getwd(); err == nil {
		for dir := wd; ; dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, RootFile)); err == nil {
				return dir, nil
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	if configDir != "" {
		if b, err := os.ReadFile(filepath.Join(configDir, "archive-path")); err == nil {
			return check(strings.TrimSpace(string(b)))
		}
	}
	return "", errors.New(`no archive found: run inside an archive, pass --archive <dir>, or set DEVOPSPAD_ARCHIVE.
Archive, edge (Cloudflare) and secrets check commands need one. To start:
  devopspadcli archive init ~/opc-archive                                     # creates it and makes it the default
  devopspadcli secrets set keychain:archive/providers/cloudflare/api-token   # Cloudflare API token (stdin, kept in the OS keychain)
  devopspadcli archive import --cloudflare                                    # adds provider:cloudflare and your zones
Then for example: devopspadcli edge routes list --zone example.com`)
}

// Load reads and validates every file of the archive. The returned error is
// only for an unusable root; findings are in Errors and Warnings.
func Load(dir string) (*Archive, error) {
	a := &Archive{Dir: dir, byRef: map[string]*Resource{}, textFiles: map[string][]byte{}, Resources: []*Resource{}, Notes: []*Note{}, Records: []*ActionRecord{}, Errors: []Issue{}, Warnings: []Issue{}}
	b, err := readRegular(filepath.Join(dir, RootFile), maxResourceSize)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", RootFile, err)
	}
	root := &Root{}
	if node, ok := a.decode(RootFile, b, root); ok {
		if root.Kind != "Archive" {
			a.errorf(RootFile, 0, "kind", "kind.mismatch", "kind must be Archive")
		}
		a.checkName(RootFile, root.Metadata.Name, false)
		a.walkStruct(RootFile, node, "", "spec", root.Spec)
	}
	a.Root = root
	a.loadInventory()
	a.loadProducts()
	a.loadNotes("notes")
	a.loadRecords()
	a.checkKinds()
	a.resolveRefs()
	a.checkServiceCycles()
	a.scanSecrets()
	a.checkIndex()
	a.checkIgnore()
	a.checkExpired()
	sortIssues(a.Errors)
	sortIssues(a.Warnings)
	return a, nil
}

func sortIssues(list []Issue) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].File != list[j].File {
			return list[i].File < list[j].File
		}
		return list[i].Line < list[j].Line
	})
}

func (a *Archive) loadInventory() {
	entries, err := os.ReadDir(filepath.Join(a.Dir, "inventory"))
	if err != nil {
		return
	}
	known := map[string]kindInfo{}
	for _, k := range kinds {
		if k.Dir != "" {
			known[path.Base(k.Dir)] = k
		}
	}
	for _, e := range entries {
		rel := "inventory/" + e.Name()
		k, ok := known[e.Name()]
		if !ok || !e.IsDir() {
			if !hiddenOrKeep(e.Name()) {
				a.errorf(rel, 0, "", "layout.unknown", "unexpected entry; inventory contains providers, hosts, clusters, datastores, registries and domains")
			}
			continue
		}
		for _, f := range a.listFiles(rel) {
			if !strings.HasSuffix(f, ".yaml") {
				a.errorf(f, 0, "", "layout.extension", "resource files must use the .yaml extension")
				continue
			}
			a.loadResource(f, k, strings.TrimSuffix(path.Base(f), ".yaml"), "")
		}
	}
}

func (a *Archive) loadProducts() {
	entries, err := os.ReadDir(filepath.Join(a.Dir, "products"))
	if err != nil {
		return
	}
	product, _ := kindByRef("product")
	service, _ := kindByRef("service")
	for _, e := range entries {
		if hiddenOrKeep(e.Name()) {
			continue
		}
		base := "products/" + e.Name()
		if !e.IsDir() {
			a.errorf(base, 0, "", "layout.unknown", "products contains one directory per product")
			continue
		}
		if _, err := os.Lstat(filepath.Join(a.Dir, base, "product.yaml")); err != nil {
			a.errorf(base, 0, "", "layout.missing_product", "missing product.yaml; create it with devopspadcli new product %s", e.Name())
		} else {
			a.loadResource(base+"/product.yaml", product, e.Name(), "")
		}
		children, _ := os.ReadDir(filepath.Join(a.Dir, base))
		for _, c := range children {
			rel := base + "/" + c.Name()
			switch {
			case c.Name() == "product.yaml" || hiddenOrKeep(c.Name()):
			case c.Name() == "services" && c.IsDir():
				for _, f := range a.listFiles(rel) {
					if path.Dir(f) != rel || !strings.HasSuffix(f, ".yaml") {
						a.errorf(f, 0, "", "layout.extension", "services contains one <service>.yaml per service")
						continue
					}
					a.loadResource(f, service, strings.TrimSuffix(path.Base(f), ".yaml"), e.Name())
				}
			case c.Name() == "configs" && c.IsDir():
				for _, f := range a.listFiles(rel) {
					if b, err := readRegular(filepath.Join(a.Dir, f), maxTextSize); err != nil {
						a.errorf(f, 0, "", "file.unreadable", "%v", err)
					} else {
						a.textFiles[f] = b
					}
				}
			case c.Name() == "notes" && c.IsDir():
				a.loadNotes(rel)
			default:
				a.errorf(rel, 0, "", "layout.unknown", "a product directory contains product.yaml, services/, configs/ and notes/")
			}
		}
	}
}

// listFiles returns regular files below rel (slash separated, relative to the
// archive). Symbolic links are rejected so nothing outside the archive is read.
func (a *Archive) listFiles(rel string) []string {
	var out []string
	_ = filepath.WalkDir(filepath.Join(a.Dir, rel), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		r, _ := filepath.Rel(a.Dir, p)
		r = filepath.ToSlash(r)
		if d.Type()&fs.ModeSymlink != 0 {
			a.errorf(r, 0, "", "layout.symlink", "symbolic links are not allowed in the archive")
			return nil
		}
		if d.IsDir() {
			if p != filepath.Join(a.Dir, rel) && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !hiddenOrKeep(d.Name()) {
			out = append(out, r)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func hiddenOrKeep(name string) bool { return strings.HasPrefix(name, ".") }

func (a *Archive) loadResource(file string, k kindInfo, name, product string) {
	b, err := readRegular(filepath.Join(a.Dir, file), maxResourceSize)
	if err != nil {
		a.errorf(file, 0, "", "file.unreadable", "%v", err)
		return
	}
	obj := k.New()
	node, ok := a.decode(file, b, obj)
	if !ok {
		return
	}
	h := obj.head()
	if h.Kind != k.Kind {
		a.errorf(file, lineOf(node, "kind"), "kind", "kind.mismatch", "kind must be %s in this directory, found %q", k.Kind, h.Kind)
		return
	}
	if h.Metadata.Name != name {
		a.errorf(file, lineOf(node, "metadata.name"), "metadata.name", "name.file_mismatch", "metadata.name %q must equal the file or directory name %q", h.Metadata.Name, name)
		return
	}
	ref := k.Ref + ":" + name
	if k.Kind == "Service" {
		if h.Metadata.Product != product {
			a.errorf(file, lineOf(node, "metadata.product"), "metadata.product", "name.product_mismatch", "metadata.product must be %q (the product directory)", product)
			return
		}
		ref = "service:" + product + "/" + name
	} else if h.Metadata.Product != "" {
		a.errorf(file, lineOf(node, "metadata.product"), "metadata.product", "field.not_allowed", "metadata.product is only used by Service")
	}
	a.checkName(file, name, k.Kind == "Domain")
	a.checkLabels(file, node, h.Metadata.Labels)
	r := &Resource{Ref: ref, Kind: k.Kind, Name: name, File: file, Object: obj, raw: b, node: node}
	a.Resources = append(a.Resources, r)
	a.byRef[ref] = r
	a.walkStruct(file, node, ref, "spec", specOf(obj))
}

// decode parses one YAML document strictly into out and also returns its node
// tree for line lookups and secret scanning.
func (a *Archive) decode(file string, b []byte, out Object) (*yaml.Node, bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		a.errorf(file, yamlLine(err.Error()), "", "yaml.syntax", "%s", cleanYAMLError(err.Error()))
		a.scanText(file, b, false) // invalid files are dropped, but secrets in them must still be reported
		return nil, false
	}
	if len(doc.Content) == 0 {
		a.errorf(file, 0, "", "yaml.empty", "file is empty")
		return nil, false
	}
	node := doc.Content[0]
	ok := false
	defer func() {
		if !ok || file == RootFile {
			a.scanResourceKeys(file, node)
			a.scanText(file, b, false)
		}
	}()
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	err := d.Decode(out)
	var extra any
	if err == nil && d.Decode(&extra) != io.EOF {
		a.errorf(file, 0, "", "yaml.multiple_documents", "only one YAML document is allowed per file")
		return nil, false
	}
	if err != nil {
		var te *yaml.TypeError
		if errors.As(err, &te) {
			for _, msg := range te.Errors {
				a.errorf(file, yamlLine(msg), "", schemaCode(msg), "%s", cleanYAMLError(msg))
			}
		} else {
			a.errorf(file, yamlLine(err.Error()), "", "yaml.syntax", "%s", cleanYAMLError(err.Error()))
		}
		return nil, false
	}
	h := out.head()
	if h.APIVersion != APIVersion {
		a.errorf(file, lineOf(node, "apiVersion"), "apiVersion", "api.version", "apiVersion must be %s", APIVersion)
		return nil, false
	}
	a.walkStruct(file, node, "", "metadata", h.Metadata)
	ok = true
	return node, true
}

var (
	lineRE    = regexp.MustCompile(`line (\d+)`)
	unknownRE = regexp.MustCompile(`field (\S+) not found in type \S+`)
	typeRE    = regexp.MustCompile(`cannot unmarshal !!(\w+) .*into (\S+)`)
)

func yamlLine(msg string) int {
	if m := lineRE.FindStringSubmatch(msg); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}
func schemaCode(msg string) string {
	if unknownRE.MatchString(msg) {
		return "schema.unknown_field"
	}
	return "schema.type"
}
func cleanYAMLError(msg string) string {
	msg = strings.TrimPrefix(msg, "yaml: ")
	msg = regexp.MustCompile(`^line \d+: `).ReplaceAllString(msg, "")
	msg = unknownRE.ReplaceAllString(msg, "unknown field $1; see devopspadcli schema <kind>, or put free text in metadata.annotations")
	msg = typeRE.ReplaceAllString(msg, "wrong type: found a $1, expected $2")
	return msg
}

// lineOf finds the line of a dotted path such as spec.deployments[0].environment.
func lineOf(node *yaml.Node, p string) int {
	if node == nil {
		return 0
	}
	cur := node
	for _, seg := range splitPath(p) {
		next := (*yaml.Node)(nil)
		if idx, err := strconv.Atoi(seg); err == nil {
			if cur.Kind == yaml.SequenceNode && idx < len(cur.Content) {
				next = cur.Content[idx]
			}
		} else if cur.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(cur.Content); i += 2 {
				if cur.Content[i].Value == seg {
					next = cur.Content[i+1]
					if next.Kind != yaml.MappingNode && next.Kind != yaml.SequenceNode {
						return cur.Content[i].Line
					}
					break
				}
			}
		}
		if next == nil {
			return cur.Line
		}
		cur = next
	}
	return cur.Line
}

func splitPath(p string) []string {
	p = strings.NewReplacer("[", ".", "]", "").Replace(p)
	var out []string
	for _, s := range strings.Split(p, ".") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func readRegular(p string, limit int64) ([]byte, error) {
	info, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("file exceeds %d KiB", limit>>10)
	}
	return os.ReadFile(p)
}

func (a *Archive) loadNotes(rel string) {
	for _, f := range a.listFiles(rel) {
		if !strings.HasSuffix(f, ".md") {
			a.errorf(f, 0, "", "layout.extension", "notes are Markdown files (.md)")
			continue
		}
		b, err := readRegular(filepath.Join(a.Dir, f), maxTextSize)
		if err != nil {
			a.errorf(f, 0, "", "file.unreadable", "%v", err)
			continue
		}
		a.textFiles[f] = b
		note := &Note{File: f}
		if fm, ok := frontMatter(b); ok {
			d := yaml.NewDecoder(bytes.NewReader(fm))
			d.KnownFields(true)
			if err := d.Decode(&note.Meta); err != nil && err != io.EOF {
				a.errorf(f, yamlLine(err.Error())+1, "", "note.front_matter", "%s (allowed keys: title, refs, date, tags)", cleanYAMLError(err.Error()))
			}
			if note.Meta.Date != "" && !validDate(note.Meta.Date) {
				a.errorf(f, 0, "date", "format.date", "date must be YYYY-MM-DD")
			}
		}
		a.Notes = append(a.Notes, note)
	}
}

func frontMatter(b []byte) ([]byte, bool) {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return nil, false
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		return nil, false
	}
	return []byte(s[4 : 4+end+1]), true
}
