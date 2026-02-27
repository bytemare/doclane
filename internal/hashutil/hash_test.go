package hashutil

import "testing"

func TestSHA256Hex(t *testing.T) {
	t.Parallel()

	if got := SHA256Hex([]byte("abc")); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("unexpected hash for abc: %s", got)
	}
	if got := SHA256Hex(nil); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("unexpected hash for nil: %s", got)
	}
}
