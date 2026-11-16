package skills

import (
	"testing"
	"time"
)

// lchtimes is not done on Windows, whose tests do not use it.
func lchtimes(t *testing.T, p string, at time.Time) { t.Skip("no link times here") }
