package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// genLarge writes an archive shaped like an OPC matrix: many small products on two hosts.
func genLarge(t testing.TB, dir string, products int) {
	w := func(rel, body string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w(RootFile, "apiVersion: devopspad.io/v1alpha1\nkind: Archive\nmetadata:\n  name: big\n")
	for _, p := range []string{"aliyun", "cloudflare", "namecheap", "github", "stripe", "resend", "openai", "plausible"} {
		w("inventory/providers/"+p+".yaml", fmt.Sprintf("apiVersion: devopspad.io/v1alpha1\nkind: Provider\nmetadata:\n  name: %s\nspec:\n  cost: { amount: \"10\", currency: USD, period: month }\n", p))
	}
	for _, h := range []string{"hk-1", "hk-2"} {
		w("inventory/hosts/"+h+".yaml", "apiVersion: devopspad.io/v1alpha1\nkind: Host\nmetadata:\n  name: "+h+"\nspec:\n  provider: provider:aliyun\n  cost: { amount: \"34\", currency: CNY, period: month }\n")
	}
	w("inventory/clusters/k3s.yaml", "apiVersion: devopspad.io/v1alpha1\nkind: Cluster\nmetadata:\n  name: k3s\nspec:\n  distribution: k3s\n  hosts: [host:hk-1, host:hk-2]\n  kubeconfig: { source: remote, host: host:hk-1, path: /etc/rancher/k3s/k3s.yaml }\n")
	w("inventory/datastores/mysql.yaml", "apiVersion: devopspad.io/v1alpha1\nkind: Datastore\nmetadata:\n  name: mysql\nspec:\n  engine: mysql\n  runtime: { host: host:hk-2 }\n")
	for i := 0; i < products; i++ {
		n := fmt.Sprintf("p%03d", i)
		w("inventory/domains/"+n+".example.yaml", fmt.Sprintf("apiVersion: devopspad.io/v1alpha1\nkind: Domain\nmetadata:\n  name: %s.example\nspec:\n  registrar: provider:namecheap\n  dns: provider:cloudflare\n  cost: { amount: \"12\", currency: USD, period: year }\n  renewal: { expiresAt: 2027-0%d-01, autoRenew: true }\n  records:\n    - { name: \"@\", type: A, target: host:hk-1 }\n", n, i%9+1))
		w("products/"+n+"/product.yaml", fmt.Sprintf("apiVersion: devopspad.io/v1alpha1\nkind: Product\nmetadata:\n  name: %s\nspec:\n  stage: live\n  domains: [domain:%s.example]\n  providers: [{ ref: provider:stripe }, { ref: provider:resend }]\n  environments:\n    - { name: pro, cluster: cluster:k3s, namespace: %s, hosts: [host:hk-1] }\n", n, n, n))
		for _, s := range []string{"web", "api"} {
			w("products/"+n+"/services/"+s+".yaml", fmt.Sprintf("apiVersion: devopspad.io/v1alpha1\nkind: Service\nmetadata:\n  name: %s\n  product: %s\nspec:\n  type: web\n  build: { method: ci, ci: { provider: provider:github } }\n  deployments:\n    - { environment: pro, runtime: k8s, selector: app=%s-%s }\n  dependsOn: [datastore:mysql, provider:openai]\n", s, n, n, s))
		}
	}
}

// TestLargeArchive guards the target scale (hundreds of products on a few
// hosts) against quadratic slowdowns in validation and reports.
func TestLargeArchive(t *testing.T) {
	dir := t.TempDir()
	genLarge(t, dir, 300)
	start := time.Now()
	a := load(t, dir)
	a.Cost()
	a.Renewals(fixedNow, -1)
	a.Orphans()
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("300 products took %v", d)
	}
	if len(a.Errors) > 0 || len(must(a.Matrix("", ""))) != 300 || len(a.UsedBy("host:hk-1")) != 300 {
		t.Fatalf("errors=%v matrix=%d", a.Errors, len(must(a.Matrix("", ""))))
	}
}

func BenchmarkLoad300Products(b *testing.B) {
	dir := b.TempDir()
	genLarge(b, dir, 300)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Load(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
