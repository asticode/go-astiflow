package astiavflow

import (
	"fmt"

	"github.com/asticode/go-astiav"
)

type DictionaryOptions struct {
	Flags             astiav.DictionaryFlags
	KeyValueSeparator string
	PairsSeparator    string
	String            string
}

func NewCommaDictionaryOptions(format string, args ...interface{}) DictionaryOptions {
	return DictionaryOptions{
		KeyValueSeparator: "=",
		PairsSeparator:    ",",
		String:            fmt.Sprintf(format, args...),
	}
}

func NewSemiColonDictionaryOptions(format string, args ...interface{}) DictionaryOptions {
	return DictionaryOptions{
		KeyValueSeparator: "=",
		PairsSeparator:    ";",
		String:            fmt.Sprintf(format, args...),
	}
}

type dictionary struct {
	*astiav.Dictionary
}

func newDictionary(d *astiav.Dictionary) *dictionary {
	return &dictionary{Dictionary: d}
}

func (d *dictionary) close() {
	if d.Dictionary != nil {
		d.Free()
	}
}

func (o DictionaryOptions) dictionary() (*dictionary, error) {
	// Nothing to do
	if o.String == "" {
		return newDictionary(nil), nil
	}

	// Create dictionnary
	d := astiav.NewDictionary()

	// Parse string
	if err := d.ParseString(o.String, o.KeyValueSeparator, o.PairsSeparator, o.Flags); err != nil {
		return nil, fmt.Errorf("astiavflow: parsing string failed: %w", err)
	}
	return newDictionary(d), nil
}
