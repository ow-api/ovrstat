package ovrstat

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPlayerStats(t *testing.T) {
	stats, err := Stats(os.Getenv("TEST_USER"))

	if err != nil {
		t.Fatal(err)
	}

	t.Log("Name:", stats.Name)

	b, _ := json.MarshalIndent(stats, "", "\t")

	t.Log(string(b))
}
