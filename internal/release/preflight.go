package release

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var serviceAccountGVR = schema.GroupVersionResource{Version: "v1", Resource: "serviceaccounts"}

// PullTarget is a pod spec that will run Image: a Deployment or a migration Job.
type PullTarget struct {
	Label          string
	Namespace      string
	Image          string
	BeforeImage    string // empty for Jobs: no registry change check
	PullSecrets    []string
	ServiceAccount string
}

type PreflightOptions struct {
	AckRegistryChange bool // allow a registry/owner change without a matching pull secret
	Verify            bool // request the manifest from the registry with the pull secret
}

// PullTargetOf describes a prepared Deployment plan for the preflight.
func PullTargetOf(p DesktopPlan) PullTarget {
	t := PullTarget{Label: "deployment " + p.Workload, Namespace: p.Namespace, Image: p.Image, BeforeImage: p.BeforeImage}
	if p.candidate != nil {
		t.PullSecrets, t.ServiceAccount = podPullSecrets(p.candidate.Object, "spec", "template", "spec")
	}
	return t
}

// PullTargetOfJob describes a rendered migration Job for the preflight.
func PullTargetOfJob(job *unstructured.Unstructured, image string) PullTarget {
	t := PullTarget{Label: "migration Job " + job.GetName(), Namespace: job.GetNamespace(), Image: image}
	t.PullSecrets, t.ServiceAccount = podPullSecrets(job.Object, "spec", "template", "spec")
	return t
}

func podPullSecrets(obj map[string]any, path ...string) ([]string, string) {
	spec, _, _ := unstructured.NestedMap(obj, path...)
	var names []string
	if list, ok := spec["imagePullSecrets"].([]any); ok {
		for _, raw := range list {
			if m, ok := raw.(map[string]any); ok {
				if n, _ := m["name"].(string); n != "" {
					names = append(names, n)
				}
			}
		}
	}
	sa, _ := spec["serviceAccountName"].(string)
	if sa == "" {
		sa = "default"
	}
	return names, sa
}

// imageParts splits a reference into registry host, repository and the tag or digest.
func imageParts(ref string) (host, repo, reference string) {
	name := ref
	if i := strings.Index(name, "@"); i >= 0 {
		name, reference = name[:i], name[i+1:]
	} else if i := strings.LastIndex(name, ":"); i > strings.LastIndex(name, "/") {
		name, reference = name[:i], name[i+1:]
	} else {
		reference = "latest"
	}
	first, rest, ok := strings.Cut(name, "/")
	if ok && (strings.ContainsAny(first, ".:") || first == "localhost") {
		host, repo = strings.ToLower(first), rest
	} else {
		host, repo = "docker.io", name
	}
	if host == "docker.io" && !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	return host, repo, reference
}

func owner(repo string) string {
	o, _, _ := strings.Cut(repo, "/")
	return o
}

// PreflightPull checks what a server dry-run cannot: every referenced pull
// secret exists and is a docker config, a move to another registry or owner
// is covered by credentials (or acknowledged), and optionally that the
// registry serves the image with those credentials. It returns notes for the
// plan; credentials are read into memory only and never printed or recorded.
func PreflightPull(ctx context.Context, cl dynamic.Interface, targets []PullTarget, opts PreflightOptions) ([]string, error) {
	var notes []string
	verified := map[string]bool{}
	for _, t := range targets {
		names := append([]string{}, t.PullSecrets...)
		// Pods also use the pull secrets of their ServiceAccount.
		if sa, err := cl.Resource(serviceAccountGVR).Namespace(t.Namespace).Get(ctx, t.ServiceAccount, metav1.GetOptions{}); err == nil {
			saSecrets, _ := podPullSecrets(sa.Object)
			names = append(names, saSecrets...)
		}
		host, repo, ref := imageParts(t.Image)
		var covering []string
		creds := map[string]dockerAuth{}
		for _, n := range names {
			hosts, err := checkPullSecret(ctx, cl, t.Namespace, n)
			if err != nil {
				return notes, fmt.Errorf("%s: %w", t.Label, err)
			}
			for _, h := range hosts {
				if h == host {
					covering = append(covering, n)
				}
			}
		}
		sort.Strings(covering)
		if t.BeforeImage != "" {
			oldHost, oldRepo, _ := imageParts(t.BeforeImage)
			if oldHost != host || owner(oldRepo) != owner(repo) {
				move := fmt.Sprintf("%s: the image moves from %s/%s to %s/%s", t.Label, oldHost, owner(oldRepo), host, owner(repo))
				switch {
				case len(covering) > 0:
					notes = append(notes, fmt.Sprintf("%s; pull secret %s has credentials for %s (a token for the old owner may not read the new package; --verify-pull checks it)", move, strings.Join(covering, ", "), host))
				case opts.AckRegistryChange:
					notes = append(notes, move+"; no pull secret has credentials for "+host+" (acknowledged as public)")
				default:
					return notes, fmt.Errorf("%s, and no pull secret has credentials for %s. Create one (devopspadcli secret set --type docker-registry) and add it (devopspadcli workload patch --add-image-pull-secret), or pass --ack-registry-change if the image is public", move, host)
				}
			}
		}
		if !opts.Verify {
			continue
		}
		key := t.Image + "|" + strings.Join(covering, ",")
		if verified[key] {
			continue
		}
		verified[key] = true
		for _, n := range covering {
			s, err := getSecret(ctx, cl, t.Namespace, n)
			if err != nil {
				return notes, err
			}
			auths, _ := dockerAuths(s)
			creds[n] = auths[host]
		}
		note, err := verifyPull(ctx, t.Label, host, repo, ref, covering, creds)
		if err != nil {
			return notes, err
		}
		notes = append(notes, note)
	}
	return notes, nil
}

func verifyPull(ctx context.Context, label, host, repo, ref string, secrets []string, creds map[string]dockerAuth) (string, error) {
	image := host + "/" + repo + "@" + ref
	if !strings.HasPrefix(ref, "sha256:") {
		image = host + "/" + repo + ":" + ref
	}
	if len(secrets) == 0 {
		code, err := registryHead(ctx, host, repo, ref, "", "")
		if err != nil {
			return "", fmt.Errorf("pull check: %s: %s: %w", label, image, err)
		}
		if code == http.StatusOK {
			return fmt.Sprintf("pull check: %s: %s is public", label, image), nil
		}
		return "", fmt.Errorf("pull check: %s: %s returned HTTP %d without credentials; add a pull secret for %s", label, image, code, host)
	}
	var results []string
	for _, n := range secrets {
		c := creds[n]
		code, err := registryHead(ctx, host, repo, ref, c.Username, c.Password)
		if err != nil {
			return "", fmt.Errorf("pull check: %s: %s: %w", label, image, err)
		}
		if code == http.StatusOK {
			return fmt.Sprintf("pull check: %s: %s readable with pull secret %s", label, image, n), nil
		}
		results = append(results, fmt.Sprintf("%s → HTTP %d", n, code))
	}
	return "", fmt.Errorf("pull check: %s: cannot read %s (%s): the credentials cannot pull this image (403: no access, 404: missing or no access, 401: invalid)", label, image, strings.Join(results, ", "))
}

// registryScheme and registryClient are replaced in tests.
var (
	registryScheme = "https"
	registryClient = &http.Client{Timeout: 15 * time.Second}
)

const manifestAccept = "application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json"

// registryHead requests a manifest with the registry v2 API, following a
// Bearer token or Basic challenge. It returns the final HTTP status.
func registryHead(ctx context.Context, host, repo, ref, user, pass string) (int, error) {
	api := host
	if host == "docker.io" {
		api = "registry-1.docker.io"
	}
	target := registryScheme + "://" + api + "/v2/" + repo + "/manifests/" + ref
	head := func(auth string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", manifestAccept)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		return registryClient.Do(req)
	}
	res, err := head("")
	if err != nil {
		return 0, err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		return res.StatusCode, nil
	}
	challenge := res.Header.Get("Www-Authenticate")
	scheme, params, _ := strings.Cut(challenge, " ")
	var auth string
	switch strings.ToLower(scheme) {
	case "basic":
		if user == "" {
			return http.StatusUnauthorized, nil
		}
		auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	case "bearer":
		token, err := registryToken(ctx, parseChallenge(params), repo, user, pass)
		if err != nil {
			return 0, err
		}
		if token == "" {
			return http.StatusUnauthorized, nil
		}
		auth = "Bearer " + token
	default:
		return http.StatusUnauthorized, nil
	}
	res, err = head(auth)
	if err != nil {
		return 0, err
	}
	res.Body.Close()
	return res.StatusCode, nil
}

func parseChallenge(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			out[strings.ToLower(k)] = strings.Trim(v, `"`)
		}
	}
	return out
}

func registryToken(ctx context.Context, c map[string]string, repo, user, pass string) (string, error) {
	realm := c["realm"]
	if realm == "" {
		return "", errors.New("registry sent a Bearer challenge without a realm")
	}
	q := url.Values{}
	if c["service"] != "" {
		q.Set("service", c["service"])
	}
	q.Set("scope", "repository:"+repo+":pull")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	res, err := registryClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return "", nil
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry token endpoint returned HTTP %d", res.StatusCode)
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return "", err
	}
	if body.Token != "" {
		return body.Token, nil
	}
	return body.AccessToken, nil
}

var stuckReasons = map[string]bool{"ImagePullBackOff": true, "ErrImagePull": true, "InvalidImageName": true, "CreateContainerConfigError": true, "CreateContainerError": true, "CrashLoopBackOff": true}

// PodWaitingSummary explains why matching pods are not running, e.g.
// "pods waiting: ImagePullBackOff for ghcr.io/x/app@sha256:…". It uses its own
// short timeout so it works after the caller's deadline passed.
func PodWaitingSummary(cl dynamic.Interface, namespace string, match func(*unstructured.Unstructured) bool) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pods, err := cl.Resource(podGVR).Namespace(namespace).List(ctx, metav1.ListOptions{Limit: 500})
	if err != nil {
		return ""
	}
	seen := map[string]bool{}
	var out []string
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !match(pod) {
			continue
		}
		for _, field := range []string{"initContainerStatuses", "containerStatuses"} {
			statuses, _, _ := unstructured.NestedSlice(pod.Object, "status", field)
			for _, raw := range statuses {
				st, _ := raw.(map[string]any)
				reason, _, _ := unstructured.NestedString(st, "state", "waiting", "reason")
				if !stuckReasons[reason] {
					continue
				}
				image, _ := st["image"].(string)
				item := reason + " for " + image
				if !seen[item] {
					seen[item] = true
					out = append(out, item)
				}
			}
		}
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	return "pods waiting: " + strings.Join(out, "; ")
}

// ReleasePods matches the pods of one release plan.
func ReleasePods(id string) func(*unstructured.Unstructured) bool {
	return func(p *unstructured.Unstructured) bool { return p.GetAnnotations()["devopspad.io/release"] == id }
}
