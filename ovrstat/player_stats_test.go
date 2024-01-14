package ovrstat

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPlayerStats(t *testing.T) {
	if os.Getenv("TEST_PLATFORM") == "" {
		t.Skip("Skipping test due to missing platform")
		return
	}

	if os.Getenv("TEST_USER") == "" {
		t.Skip("Skipping test due to missing user")
		return
	}

	stats, err := Stats(os.Getenv("TEST_PLATFORM"), os.Getenv("TEST_USER"))

	if err != nil {
		t.Fatal(err)
	}

	t.Log("Name:", stats.Name)

	b, _ := json.MarshalIndent(stats, "", "\t")

	t.Log(string(b))
}
