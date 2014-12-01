package libpack

import (
	"fmt"
	"testing"
)

func TestMkAnnotation(t *testing.T) {
	resultMap := map[string]string{
		"/":        "0",
		"/one":     "1/one",
		"/one/two": "2/one/two",
	}

	for original, annotation := range resultMap {
		if MkAnnotation(original) != annotation {
			t.Fatalf("Path %q does not correspond to annotation %q", original, annotation)
		}
	}
}

func TestParseAnnotation(t *testing.T) {
	resultMap := map[string]string{
		"0":         "",
		"1/one":     "one",
		"2/one/two": "one/two",
	}

	for annotation, original := range resultMap {
		if target, err := ParseAnnotation(annotation); err == nil {
			if target != original {
				t.Fatalf("Annotation %q does not equal original path %q: %q", annotation, original, target)
			}
		} else {
			t.Fatal(err)
		}
	}
}

func TestGetSetDeleteAnnotations(t *testing.T) {
	db := tmpDB(t, "")
	defer nukeDB(db)

	if err := db.SetAnnotation("/one", "tmp"); err != nil {
		t.Fatal(err)
	}

	if str, err := db.GetAnnotation("/one"); err != nil {
		t.Fatal(err)
	} else if str != "tmp" {
		t.Fatalf("annotation for /one did not equal 'tmp': %q", str)
	}

	if err := db.DeleteAnnotation("/one"); err != nil {
		t.Fatal(err)
	}

	if str, err := db.GetAnnotation("/one"); err == nil || str != "" {
		t.Fatalf("annotation /one still has content: %q", str)
	}
}

func TestWalkAnnotations(t *testing.T) {
	db := tmpDB(t, "")

	// ideally, this structure should enforce the deterministic nature of
	// WalkAnnotations.
	resultMap := [][][]string{
		[][]string{[]string{"/one"}, []string{"one"}},
		[][]string{[]string{"/one", "/one/two"}, []string{"one", "one/two"}},
	}

	for _, list := range resultMap {
		for _, path := range list[0] {
			if err := db.SetAnnotation(path, "tmp"); err != nil {
				t.Fatal(err)
			}
		}

		results := []string{}

		if err := db.WalkAnnotations(func(name, value string) { results = append(results, name) }); err != nil {
			t.Fatal(err)
		}

		fmt.Println(results)

		if len(results) != len(list[1]) {
			t.Fatalf("expected list (%d) has different size than produced list (%d)", len(list[1]), len(results))
		}

		for i, result := range results {
			if list[1][i] != result {
				t.Fatalf("expected annotation %q does not equal result %q", list[1][i], result)
			}
		}
	}
}
