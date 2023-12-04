package astiflow_test

import (
	"testing"

	"github.com/asticode/go-astiflow/pkg/astiflow"
	"github.com/stretchr/testify/require"
)

func TestMetadata(t *testing.T) {
	m1 := astiflow.Metadata{
		Description: "d1",
		Name:        "n1",
		Tags:        []string{"t1"},
	}
	m1.Merge(astiflow.Metadata{Description: "d2"})
	require.Equal(t, "d2", m1.Description)
	m1.Merge(astiflow.Metadata{Name: "n2"})
	require.Equal(t, "n2", m1.Name)
	m1.Merge(astiflow.Metadata{Tags: []string{"t2"}})
	require.Equal(t, []string{"t1", "t2"}, m1.Tags)
}
