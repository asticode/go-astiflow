package astiflow

import "slices"

type Metadata struct {
	Description string   `json:"description,omitempty"`
	Name        string   `json:"name,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

func (m *Metadata) Merge(i Metadata) Metadata {
	if i.Description != "" {
		m.Description = i.Description
	}
	if i.Name != "" {
		m.Name = i.Name
	}
	for _, t := range i.Tags {
		if !slices.Contains(m.Tags, t) {
			m.Tags = append(m.Tags, t)
		}
	}
	return *m
}
