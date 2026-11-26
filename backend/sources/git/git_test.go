package git

import (
	"fmt"
	"testing"
)

func pkt(line string) string {
	return fmt.Sprintf("%04x%s", len(line)+4, line)
}

func TestParsePktLines(t *testing.T) {
	raw := pkt("# service=git-upload-pack\n") +
		"0000" +
		pkt("1111111111111111111111111111111111111111 refs/heads/main\x00multi_ack\n") +
		pkt("2222222222222222222222222222222222222222 refs/tags/v1.0.0\n") +
		pkt("3333333333333333333333333333333333333333 refs/tags/v1.0.0^{}\n") +
		"0000"

	refs := parsePktLines([]byte(raw))
	if refs["refs/heads/main"] != "1111111111111111111111111111111111111111" {
		t.Fatalf("bad main ref: %+v", refs)
	}
	if refs["refs/tags/v1.0.0^{}"] != "3333333333333333333333333333333333333333" {
		t.Fatalf("bad peeled tag: %+v", refs)
	}
}
