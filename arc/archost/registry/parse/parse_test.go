package parse

import (
	"fmt"
	"testing"
)

func TestExpr(t *testing.T) {
	var tsts = []string{
		"elem-type.org",
		"[UTC16]elem",
		"elem:name",
		"elem-type.org:name",
		"[Surface.Name]elem:name",
		"[Locale.Name]elem-type:name.ext",
	}

	for _, tst := range tsts {
		spec, err := AttrSpecParser.ParseString("", tst)
		if err != nil {
			fmt.Printf("%-30s %v\n", tst, err)
		} else {
			fmt.Printf("%-30s %-15v %-15v %-15v\n", tst, spec.SeriesSpec, spec.ElemType, spec.AttrName)
		}
	}

}
