package templatex

import "testing"

func TestRenderStrictSuccess(t *testing.T) {
	t.Parallel()

	res, err := Render([]byte("Hello {{ shared.project_name }}"), map[string]string{
		"project_name": "Doclane",
	}, true)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if got := string(res.Rendered); got != "Hello Doclane" {
		t.Fatalf("unexpected render output: %q", got)
	}
}

func TestRenderStrictMissingVar(t *testing.T) {
	t.Parallel()

	_, err := Render([]byte("Hello {{ shared.project_name }}"), map[string]string{}, true)
	if err == nil {
		t.Fatal("expected missing var error")
	}
}

func TestRenderStrictUnusedVar(t *testing.T) {
	t.Parallel()

	_, err := Render([]byte("Hello world"), map[string]string{"project_name": "Doclane"}, true)
	if err == nil {
		t.Fatal("expected unused var error")
	}
}
