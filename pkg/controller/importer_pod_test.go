package controller

import (
	"testing"
)

func Test_Humanize(t *testing.T) {
	p, _ := extractProgress(`
registry_progress{ownerUID="11"} 12.34
registry_speed{ownerUID="11"} 1.234532345432e+6
`, `11`)

	if p.AvgSpeed() != `1.2 MB/s` {
		t.Fatalf("%s is not expected human readable value for raw value %v", p.AvgSpeed(), p.AvgSpeedRaw())
	}
}
