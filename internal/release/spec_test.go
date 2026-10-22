package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseSpecAcceptsPinnedImage(t *testing.T) {
	spec, err := ParseSpec("testdata/ok.release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Service != "heartie-admin" || spec.Images[0].Image == "" {
		t.Fatalf("%+v", spec)
	}
	sum := spec.SHA256()
	if len(sum) != 64 {
		t.Fatalf("sha256 = %q", sum)
	}
}

func TestReleaseSpecRejectsUnsafeInput(t *testing.T) {
	header := "apiVersion: infradex.release/v1\nservice: app\nversion: 1.0.0\nfleet: hk\n"
	cases := []string{
		header + "images:\n  - {workload: app, kind: Deployment, namespace: default, container: app, image: app:latest}\n",
		header + "images:\n  - {workload: app, kind: Deployment, namespace: default, container: app, image: app:1.2}\n",
		header + "nginx:\n  - {file: ../x.conf, name: x.conf}\n",
		header + "nginx:\n  - {file: a.conf, name: ../a.conf}\n",
		header + "unknown: true\n",
	}
	for _, body := range cases {
		_, err := ParseSpecBytes([]byte(body))
		if err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}

func TestReleaseSpecRejectsMissingStepsAndBounds(t *testing.T) {
	header := "apiVersion: infradex.release/v1\nservice: app\nversion: 1.0.0\nfleet: hk\n"
	if _, err := ParseSpecBytes([]byte(header)); err == nil {
		t.Fatal("accepted a spec with no release steps")
	}
	body := header + "migrate:\n  jobFile: job.yaml\n  waitTimeoutSeconds: 0\n"
	if _, err := ParseSpecBytes([]byte(body)); err == nil {
		t.Fatal("accepted a zero wait timeout")
	}
}

func TestValidateImagesAllowsExplicitTagButNotLatest(t *testing.T) {
	spec := Spec{Images: []Image{{Image: "registry.example/app:1.2.3"}}}
	if err := ValidateImages(spec, true); err != nil {
		t.Fatal(err)
	}
	if err := ValidateImages(spec, false); err == nil {
		t.Fatal("tag accepted without allowTag")
	}
	latest := Spec{Images: []Image{{Image: "registry.example/app:latest"}}}
	if err := ValidateImages(latest, true); err == nil || !strings.Contains(err.Error(), "latest") {
		t.Fatalf("latest error = %v", err)
	}
}

func TestReleaseSpecRejectsSecretsAndWrongManifestKind(t *testing.T) {
	header := "apiVersion: infradex.release/v1\nservice: app\nversion: 1.0.0\nfleet: hk\n"
	dsn := header + "images:\n  - {workload: app, kind: Deployment, namespace: default, container: app, image: registry.example/app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}\ncheck:\n  url: user:secret@tcp(db:3306)/app\n  expectStatus: 200\n  timeoutSeconds: 10\n"
	if _, err := ParseSpecBytes([]byte(dsn)); err == nil || !strings.Contains(err.Error(), "DSN") {
		t.Fatalf("dsn error = %v", err)
	}

	dir := t.TempDir()
	job := filepath.Join(dir, "job.yaml")
	if err := os.WriteFile(job, []byte("kind: Deployment\nmetadata:\n  name: app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "service.release.yaml")
	raw := "apiVersion: infradex.release/v1\nservice: app\nversion: 1.0.0\nfleet: hk\nmigrate:\n  jobFile: job.yaml\n  waitTimeoutSeconds: 30\n"
	if err := os.WriteFile(specPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSpec(specPath); err == nil || !strings.Contains(err.Error(), "Job") {
		t.Fatalf("kind error = %v", err)
	}
}

func TestReleaseSpecRejectsTooManyResources(t *testing.T) {
	dir := t.TempDir()
	var docs strings.Builder
	for i := 0; i < 17; i++ {
		if i > 0 {
			docs.WriteString("---\n")
		}
		docs.WriteString("kind: ConfigMap\nmetadata:\n  name: cm\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "cms.yaml"), []byte(docs.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(dir, "service.release.yaml")
	raw := "apiVersion: infradex.release/v1\nservice: app\nversion: 1.0.0\nfleet: hk\nconfigMaps:\n  - file: cms.yaml\n"
	if err := os.WriteFile(specPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ParseSpec(specPath)
	if err == nil || !strings.Contains(err.Error(), "16") {
		t.Fatalf("limit error = %v", err)
	}
}
