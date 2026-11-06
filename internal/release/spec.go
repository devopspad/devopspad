package release

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	manifestByteLimit = 1 << 20
	nginxByteLimit    = 256 << 10
	maxResources      = 16
)

var (
	serviceName   = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
	versionName   = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.]+)?$`)
	digestImage   = regexp.MustCompile(`^[^[:space:]]+@sha256:[a-f0-9]{64}$`)
	tagImage      = regexp.MustCompile(`^[^[:space:]]+:[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	nginxName     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.conf$`)
	workloadKinds = map[string]struct{}{
		"Deployment": {}, "StatefulSet": {}, "DaemonSet": {},
	}
	topFields     = set("apiVersion", "service", "version", "fleet", "machines", "images", "configMaps", "nginx", "migrate", "restart", "check")
	imageFields   = set("workload", "kind", "namespace", "container", "image")
	fileFields    = set("file")
	nginxFields   = set("file", "name")
	migrateFields = set("jobFile", "waitTimeoutSeconds")
	restartFields = set("name", "kind", "namespace")
	checkFields   = set("url", "expectStatus", "timeoutSeconds")
)

// Spec is a local release description. Its hash covers the spec bytes only,
// never a resolved absolute path.
type Spec struct {
	APIVersion string         `yaml:"apiVersion"`
	Service    string         `yaml:"service"`
	Version    string         `yaml:"version"`
	Fleet      string         `yaml:"fleet"`
	Machines   []string       `yaml:"machines"`
	Images     []Image        `yaml:"images"`
	ConfigMaps []ConfigMapRef `yaml:"configMaps"`
	Nginx      []NginxFile    `yaml:"nginx"`
	Migrate    *Migrate       `yaml:"migrate"`
	Restart    []Restart      `yaml:"restart"`
	Check      *Check         `yaml:"check"`
	raw        []byte
}

type Image struct {
	Workload  string `yaml:"workload"`
	Kind      string `yaml:"kind"`
	Namespace string `yaml:"namespace"`
	Container string `yaml:"container"`
	Image     string `yaml:"image"`
}

type ConfigMapRef struct {
	File string `yaml:"file"`
}

type NginxFile struct {
	File string `yaml:"file"`
	Name string `yaml:"name"`
}

type Migrate struct {
	JobFile            string `yaml:"jobFile"`
	WaitTimeoutSeconds int    `yaml:"waitTimeoutSeconds"`
}

type Restart struct {
	Name      string `yaml:"name"`
	Kind      string `yaml:"kind"`
	Namespace string `yaml:"namespace"`
}

type Check struct {
	URL            string `yaml:"url"`
	ExpectStatus   int    `yaml:"expectStatus"`
	TimeoutSeconds int    `yaml:"timeoutSeconds"`
}

func (s Spec) SHA256() string {
	sum := sha256.Sum256(s.raw)
	return hex.EncodeToString(sum[:])
}

func ParseSpec(path string) (Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, fmt.Errorf("read release spec: %w", err)
	}
	return parse(b, filepath.Dir(path))
}

func ParseSpecBytes(b []byte) (Spec, error) {
	return parse(append([]byte(nil), b...), "")
}

// ValidateImages enforces the release image rule. allowTag permits an explicit
// non-latest tag; :latest is never accepted.
func ValidateImages(spec Spec, allowTag bool) error {
	for _, im := range spec.Images {
		ref := im.Image
		if ref == "" || strings.ContainsAny(ref, " \t\r\n") {
			return fmt.Errorf("image %q must not contain whitespace", ref)
		}
		if strings.Contains(strings.ToLower(ref), ":latest") {
			return fmt.Errorf("image %q uses :latest", ref)
		}
		if digestImage.MatchString(ref) {
			continue
		}
		if allowTag && tagImage.MatchString(ref) {
			continue
		}
		return fmt.Errorf("image %q must be name@sha256:<64 hex>", ref)
	}
	return nil
}

func parse(raw []byte, dir string) (Spec, error) {
	if err := rejectEmbeddedSecrets(raw); err != nil {
		return Spec{}, err
	}
	doc, err := oneDocument(raw)
	if err != nil {
		return Spec{}, err
	}
	if err := knownKeys(doc, topFields); err != nil {
		return Spec{}, err
	}
	if err := checkMachines(doc); err != nil {
		return Spec{}, err
	}
	if err := knownSequence(doc, "images", imageFields); err != nil {
		return Spec{}, err
	}
	if err := knownSequence(doc, "configMaps", fileFields); err != nil {
		return Spec{}, err
	}
	if err := knownSequence(doc, "nginx", nginxFields); err != nil {
		return Spec{}, err
	}
	if err := knownSequence(doc, "restart", restartFields); err != nil {
		return Spec{}, err
	}
	if err := knownMappingField(doc, "migrate", migrateFields); err != nil {
		return Spec{}, err
	}
	if err := knownMappingField(doc, "check", checkFields); err != nil {
		return Spec{}, err
	}

	var spec Spec
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		return Spec{}, fmt.Errorf("parse release spec: %w", err)
	}
	spec.raw = append([]byte(nil), raw...)
	if err := validateFields(spec); err != nil {
		return Spec{}, err
	}
	if err := inspectFiles(spec, dir); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func validateFields(spec Spec) error {
	if spec.APIVersion != "devopspad.release/v1" {
		return errors.New("apiVersion must be devopspad.release/v1")
	}
	if !serviceName.MatchString(spec.Service) {
		return fmt.Errorf("service %q is invalid", spec.Service)
	}
	if !versionName.MatchString(spec.Version) {
		return fmt.Errorf("version %q is invalid", spec.Version)
	}
	if strings.TrimSpace(spec.Fleet) == "" {
		return errors.New("fleet is required")
	}
	if len(spec.Images) == 0 && len(spec.ConfigMaps) == 0 && len(spec.Nginx) == 0 && spec.Migrate == nil && len(spec.Restart) == 0 {
		return errors.New("release spec needs an image, configMap, nginx file, migrate job, or restart")
	}
	for _, name := range spec.Machines {
		if name == "" || strings.ContainsAny(name, "/\\\t\r\n ") {
			return fmt.Errorf("machine name %q is invalid", name)
		}
	}
	for _, im := range spec.Images {
		if im.Workload == "" || im.Container == "" || im.Namespace == "" {
			return errors.New("image entries need workload, namespace, and container")
		}
		if _, ok := workloadKinds[im.Kind]; !ok {
			return fmt.Errorf("kind %q is not allowed", im.Kind)
		}
	}
	if err := ValidateImages(spec, false); err != nil {
		return err
	}
	for _, cm := range spec.ConfigMaps {
		if err := validateRelPath(cm.File); err != nil {
			return err
		}
	}
	for _, nf := range spec.Nginx {
		if err := validateRelPath(nf.File); err != nil {
			return err
		}
		if !nginxName.MatchString(nf.Name) {
			return fmt.Errorf("nginx name %q must be a conf.d basename", nf.Name)
		}
	}
	if spec.Migrate != nil {
		if err := validateRelPath(spec.Migrate.JobFile); err != nil {
			return err
		}
		if err := timeoutInRange(spec.Migrate.WaitTimeoutSeconds, "waitTimeoutSeconds"); err != nil {
			return err
		}
	}
	for _, rs := range spec.Restart {
		if rs.Name == "" || rs.Namespace == "" {
			return errors.New("restart entries need name and namespace")
		}
		if _, ok := workloadKinds[rs.Kind]; !ok {
			return fmt.Errorf("kind %q is not allowed", rs.Kind)
		}
	}
	return validateCheck(spec.Check)
}

func inspectFiles(spec Spec, dir string) error {
	if dir == "" {
		return nil
	}
	for _, cm := range spec.ConfigMaps {
		path, err := resolveUnder(dir, cm.File)
		if err != nil {
			return err
		}
		if err := validateManifest(path, "ConfigMap"); err != nil {
			return fmt.Errorf("configmap %s: %w", cm.File, err)
		}
	}
	for _, nf := range spec.Nginx {
		path, err := resolveUnder(dir, nf.File)
		if err != nil {
			return err
		}
		if _, err := readLimited(path, nginxByteLimit); err != nil {
			return fmt.Errorf("nginx %s: %w", nf.File, err)
		}
	}
	if spec.Migrate != nil {
		path, err := resolveUnder(dir, spec.Migrate.JobFile)
		if err != nil {
			return err
		}
		if err := validateManifest(path, "Job"); err != nil {
			return fmt.Errorf("migrate job: %w", err)
		}
	}
	return nil
}

func validateManifest(path, wantKind string) error {
	b, err := readLimited(path, manifestByteLimit)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	n := 0
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
		}
		err := dec.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("parse manifest: %w", err)
		}
		if doc.Kind == "" && doc.Metadata.Name == "" {
			continue
		}
		n++
		if n > maxResources {
			return fmt.Errorf("manifest exceeds %d resources", maxResources)
		}
		if doc.Kind != wantKind {
			return fmt.Errorf("expected kind=%s, got %q", wantKind, doc.Kind)
		}
		if strings.TrimSpace(doc.Metadata.Name) == "" {
			return fmt.Errorf("%s missing metadata.name", wantKind)
		}
	}
	if n == 0 {
		return errors.New("manifest contains no resources")
	}
	return nil
}

func validateCheck(c *Check) error {
	if c == nil {
		return nil
	}
	if err := timeoutInRange(c.TimeoutSeconds, "timeoutSeconds"); err != nil {
		return err
	}
	if c.ExpectStatus < 100 || c.ExpectStatus > 599 {
		return errors.New("expectStatus must be an HTTP status")
	}
	u, err := url.Parse(c.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("check url must be an http or https URL")
	}
	return nil
}

func timeoutInRange(v int, name string) error {
	if v < 1 || v > 180 {
		return fmt.Errorf("%s must be between 1 and 180", name)
	}
	return nil
}

func rejectEmbeddedSecrets(b []byte) error {
	s := string(b)
	if strings.Contains(s, "@tcp(") {
		return errors.New("release spec must not contain a DSN")
	}
	if strings.Contains(s, "PRIVATE KEY") || strings.Contains(s, "client-certificate-data") {
		return errors.New("release spec must not contain kubeconfig or private key material")
	}
	if strings.Contains(s, "`") || strings.Contains(s, "$(") {
		return errors.New("release spec must not contain a shell command")
	}
	return nil
}

func validateRelPath(rel string) error {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, `\`) || strings.ContainsRune(rel, 0) {
		return errors.New("release file path must be relative to the spec directory")
	}
	if rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") || strings.HasSuffix(rel, "/..") {
		return errors.New("release file path must stay inside the spec directory")
	}
	cleaned := filepath.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return errors.New("release file path must stay inside the spec directory")
	}
	return nil
}

func resolveUnder(base, rel string) (string, error) {
	if err := validateRelPath(rel); err != nil {
		return "", err
	}
	full := filepath.Join(base, filepath.Clean(rel))
	relOut, err := filepath.Rel(base, full)
	if err != nil || relOut == ".." || strings.HasPrefix(relOut, ".."+string(filepath.Separator)) {
		return "", errors.New("release file path must stay inside the spec directory")
	}
	return full, nil
}

func readLimited(path string, max int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > max {
		return nil, fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), max)
	}
	return b, nil
}

func oneDocument(raw []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse release spec: %w", err)
	}
	var extra yaml.Node
	err := dec.Decode(&extra)
	if err == nil {
		return nil, errors.New("release spec must contain one document")
	}
	if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse release spec: %w", err)
	}
	return &doc, nil
}

func knownKeys(n *yaml.Node, allowed map[string]struct{}) error {
	m, err := mappingNode(n)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for i := 0; i < len(m.Content); i += 2 {
		key := m.Content[i].Value
		if _, dup := seen[key]; dup {
			return fmt.Errorf("duplicate field %s", key)
		}
		seen[key] = struct{}{}
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown field %s", key)
		}
	}
	return nil
}

func knownSequence(n *yaml.Node, key string, allowed map[string]struct{}) error {
	seq := child(n, key)
	if seq == nil || seq.Tag == "!!null" {
		return nil
	}
	if seq.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s must be a list", key)
	}
	for _, item := range seq.Content {
		if err := knownKeys(item, allowed); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}

func knownMappingField(n *yaml.Node, key string, allowed map[string]struct{}) error {
	item := child(n, key)
	if item == nil || item.Tag == "!!null" {
		return nil
	}
	if err := knownKeys(item, allowed); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}

func checkMachines(n *yaml.Node) error {
	seq := child(n, "machines")
	if seq == nil || seq.Tag == "!!null" {
		return nil
	}
	if seq.Kind != yaml.SequenceNode {
		return errors.New("machines must be a list")
	}
	for _, item := range seq.Content {
		if item.Kind != yaml.ScalarNode {
			return errors.New("machine name must be a string")
		}
	}
	return nil
}

func child(n *yaml.Node, key string) *yaml.Node {
	m, err := mappingNode(n)
	if err != nil {
		return nil
	}
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mappingNode(n *yaml.Node) (*yaml.Node, error) {
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) != 1 {
			return nil, errors.New("release spec must contain one document")
		}
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil, errors.New("release spec must be a mapping")
	}
	return n, nil
}

func set(keys ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		out[key] = struct{}{}
	}
	return out
}
