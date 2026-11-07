package ops

import (
	"errors"
	"gopkg.in/yaml.v3"
)

// Attach adds a locally mapped environment and application. It preserves YAML
// comments and uses the same revision check as the configuration editor.
func (s *Store) Attach(env Environment, app Application, expected string) (Snapshot, error) {
	snapshot, err := s.Read()
	if err != nil {
		return snapshot, err
	}
	if snapshot.Revision != expected {
		return snapshot, errors.New("The project was modified; refresh and connect again")
	}
	var doc yaml.Node
	if err = yaml.Unmarshal([]byte(snapshot.YAML), &doc); err != nil {
		return snapshot, err
	}
	root := doc.Content[0]
	appendItem := func(key string, value any) error {
		var item yaml.Node
		if err := item.Encode(value); err != nil {
			return err
		}
		for i := 0; i < len(root.Content); i += 2 {
			if root.Content[i].Value == key {
				sequence := root.Content[i+1]
				if sequence.Kind != yaml.SequenceNode {
					return errors.New("Project field must be a list: " + key)
				}
				sequence.Content = append(sequence.Content, &item)
				return nil
			}
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{&item}})
		return nil
	}
	exists := false
	for _, existing := range snapshot.Project.Environments {
		if existing.Name == env.Name {
			if existing != env {
				return snapshot, errors.New("An environment with this name points to another cluster or namespace; use a new name")
			}
			exists = true
		}
	}
	if !exists {
		if err = appendItem("environments", env); err != nil {
			return snapshot, err
		}
	}
	exists = false
	for _, existing := range snapshot.Project.Applications {
		if existing.Name == app.Name {
			if existing.Name != app.Name || existing.Selector != app.Selector {
				return snapshot, errors.New("An application with this name uses another selector; use a new name")
			}
			exists = true
		}
	}
	if !exists {
		if err = appendItem("applications", app); err != nil {
			return snapshot, err
		}
	}
	b, err := yaml.Marshal(&doc)
	if err != nil {
		return snapshot, err
	}
	return s.Save(string(b), expected)
}
