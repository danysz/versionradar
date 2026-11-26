package firebase

import "testing"

func TestConfigExampleNames(t *testing.T) {
	if (Plugin{}).Name() != "firebase" {
		t.Fatal("name")
	}
}
