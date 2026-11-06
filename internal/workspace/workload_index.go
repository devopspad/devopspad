package workspace

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// A Table without embedded objects avoids transferring every workload's large
// annotations and managed fields over SSH. It is only a candidate index: the
// returned rows always come from full objects, with the usual final label check.
func scopedWorkloadLists(ctx context.Context, c Connection, namespace, selector string) ([]namespaceListResult, error) {
	cfg, err := kubeConfig(c)
	if err != nil {
		return nil, err
	}
	// Index and full-object reads must share keep-alive/TLS connections too,
	// otherwise the extra handshake cancels much of the smaller payload's gain.
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, err
	}
	cl, err := dynamic.NewForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	cfg = dynamic.ConfigFor(cfg)
	cfg.GroupVersion = nil
	index, err := rest.UnversionedRESTClientForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	requests := workloadListRequests()
	results := make([]namespaceListResult, len(requests))
	var wg sync.WaitGroup
	for i, request := range requests {
		wg.Add(1)
		go func(i int, request namespaceListRequest) {
			defer wg.Done()
			results[i].list, results[i].err = scopedWorkloadList(ctx, cl, index, namespace, selector, request)
		}(i, request)
	}
	wg.Wait()
	return results, nil
}

func scopedWorkloadList(ctx context.Context, cl dynamic.Interface, index rest.Interface, namespace, selector string, request namespaceListRequest) (*unstructured.UnstructuredList, error) {
	resource := cl.Resource(request.resource).Namespace(namespace)
	fallback := func() (*unstructured.UnstructuredList, error) { return resource.List(ctx, request.options) }
	readIndex := func(metadataSelector string) (*metav1.Table, error) {
		raw, err := index.Get().AbsPath("apis", request.resource.Group, request.resource.Version, "namespaces", namespace, request.resource.Resource).
			Param("limit", strconv.FormatInt(request.options.Limit, 10)).Param("includeObject", "None").Param("labelSelector", metadataSelector).
			SetHeader("Accept", "application/json;as=Table;g=meta.k8s.io;v=v1,application/json").DoRaw(ctx)
		if err != nil {
			return nil, err
		}
		var table metav1.Table
		if err := json.Unmarshal(raw, &table); err != nil {
			return nil, err
		}
		if table.Kind != "Table" || table.APIVersion != "meta.k8s.io/v1" {
			return nil, nil
		}
		return &table, nil
	}
	table, err := readIndex("")
	if err != nil || table == nil {
		return fallback()
	}
	out := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{}}
	out.SetContinue(table.Continue)
	out.SetResourceVersion(table.ResourceVersion)
	if len(table.Rows) == 0 {
		return out, nil
	}
	names, ok := tableNames(table)
	if !ok {
		return fallback()
	}
	selectorColumn := tableColumn(table, "Selector")
	if selectorColumn < 0 {
		return fallback()
	}
	selected, err := labels.Parse(selector)
	if err != nil {
		return nil, err
	}
	candidates := map[string]bool{}
	for i, row := range table.Rows {
		if selectorColumn >= len(row.Cells) {
			return fallback()
		}
		raw, ok := row.Cells[selectorColumn].(string)
		if !ok {
			return fallback()
		}
		workloadSelector, err := labels.Parse(raw)
		// Unknown or non-exact selectors remain candidates; never infer a
		// missing template label from an absent index column or requirement.
		if err != nil || couldMatchTemplate(selected, workloadSelector) {
			candidates[names[i]] = true
		}
	}
	tooMany := func() bool { return len(candidates) > 6 || len(candidates)*2 > len(names) }
	if tooMany() {
		return fallback()
	}
	// The original scope matches metadata OR Pod-template labels. A second
	// compact index preserves metadata-only matches even when templates differ.
	metadata, err := readIndex(selector)
	if err != nil || metadata == nil || metadata.Continue != "" {
		return fallback()
	}
	metadataNames, ok := tableNames(metadata)
	if !ok {
		return fallback()
	}
	metadataMatches := map[string]bool{}
	for _, name := range metadataNames {
		metadataMatches[name] = true
	}
	for _, name := range names {
		if metadataMatches[name] {
			candidates[name] = true
		}
	}
	if tooMany() {
		return fallback()
	}
	// At most six candidates per kind; fetch sequentially to keep the request
	// budget bounded while the three resource kinds are already concurrent.
	for _, name := range names {
		if !candidates[name] {
			continue
		}
		obj, err := resource.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		// List-only RBAC and servers without Table support retain the old path.
		if err != nil {
			return fallback()
		}
		out.Items = append(out.Items, *obj)
	}
	return out, nil
}

func tableColumn(table *metav1.Table, name string) int {
	for i, column := range table.ColumnDefinitions {
		if column.Name == name {
			return i
		}
	}
	return -1
}

func tableNames(table *metav1.Table) ([]string, bool) {
	column := tableColumn(table, "Name")
	if column < 0 {
		return nil, len(table.Rows) == 0
	}
	names := make([]string, 0, len(table.Rows))
	seen := map[string]bool{}
	for _, row := range table.Rows {
		if column >= len(row.Cells) {
			return nil, false
		}
		name, ok := row.Cells[column].(string)
		if !ok || len(validation.IsDNS1123Subdomain(name)) != 0 || seen[name] {
			return nil, false
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, true
}

func couldMatchTemplate(selected, workload labels.Selector) bool {
	requirements, selectable := selected.Requirements()
	if !selectable {
		return false
	}
	for _, requirement := range requirements {
		// A workload's exact selector is guaranteed by Kubernetes validation to
		// appear on its template. Only a proven contradiction can exclude it.
		if value, exact := workload.RequiresExactMatch(requirement.Key()); exact && !requirement.Matches(labels.Set{requirement.Key(): value}) {
			return false
		}
	}
	return true
}
