package workspace

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestConcurrentAIProviderChangesKeepCredentialsPaired(t *testing.T) {
	s, _ := testStore(t)
	provider := func(key string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer "+key {
				t.Errorf("provider received another provider's credential")
			}
			fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
		}))
	}
	a, b := provider("key-a"), provider("key-b")
	defer a.Close()
	defer b.Close()
	configs := []AISettings{{Endpoint: a.URL, Model: "a", APIKey: "key-a"}, {Endpoint: b.URL, Model: "b", APIKey: "key-b"}}
	if err := s.SaveAI(configs[0]); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if err := s.SaveAI(configs[i%2]); err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if _, err := s.Ask(context.Background(), AIRequest{Prompt: "test", Kind: "mysql"}); err != nil {
				t.Error(err)
			}
		}
	}()
	wg.Wait()
}
