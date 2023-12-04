package astiavflow

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/stretchr/testify/require"
)

func TestDictionary(t *testing.T) {
	o := NewCommaDictionaryOptions("1=%s", "2")
	require.Equal(t, DictionaryOptions{
		KeyValueSeparator: "=",
		PairsSeparator:    ",",
		String:            "1=2",
	}, o)
	o = NewSemiColonDictionaryOptions("1=%s;3=%d", "2", 4)
	require.Equal(t, DictionaryOptions{
		KeyValueSeparator: "=",
		PairsSeparator:    ";",
		String:            "1=2;3=4",
	}, o)
	d, err := o.dictionary()
	require.NoError(t, err)
	require.Equal(t, "4", d.Get("3", nil, astiav.NewDictionaryFlags()).Value())
	d.close()
	require.Nil(t, d.Get("1", nil, astiav.NewDictionaryFlags()))
	d, err = DictionaryOptions{}.dictionary()
	require.NoError(t, err)
	require.Nil(t, d.Dictionary)
}
