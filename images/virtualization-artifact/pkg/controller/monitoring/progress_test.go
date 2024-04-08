package monitoring

import (
	"testing"
)

func Test_Humanize(t *testing.T) {
	p, _ := extractProgress(`
registry_progress{ownerUID="11"} 12.34
registry_average_speed{ownerUID="11"} 1.234532345432e+6
registry_current_speed{ownerUID="11"} 2.345632345432e+6
`, `11`)
	if p.AvgSpeed() != `1.2Mi/s` {
		t.Fatalf("%s is not expected human readable value for raw value %v", p.AvgSpeed(), p.AvgSpeedRaw())
	}

	if p.CurSpeed() != `2.2Mi/s` {
		t.Fatalf("%s is not expected human readable value for raw value %v", p.CurSpeed(), p.CurSpeedRaw())
	}
}
