package source

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/rakaprad/alert-copilot/internal/alert"
)

// ParseGeneric accepts either a single JSON object or an array of objects
// shaped like alert.Alert (name, status, severity, target, summary, ...).
// It is the escape hatch for cron jobs, scripts and tools without a
// dedicated parser.
func ParseGeneric(r io.Reader) ([]alert.Alert, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var list []alert.Alert
	if err := json.Unmarshal(raw, &list); err != nil {
		var one alert.Alert
		if err2 := json.Unmarshal(raw, &one); err2 != nil {
			return nil, fmt.Errorf("decode generic payload: %w", err2)
		}
		list = []alert.Alert{one}
	}
	if len(list) == 0 {
		return nil, errors.New("generic payload has no alerts")
	}
	for i := range list {
		a := &list[i]
		if a.Name == "" {
			return nil, fmt.Errorf("alert %d: name is required", i)
		}
		a.Source = "generic"
		if a.Status != alert.StatusResolved {
			a.Status = alert.StatusFiring
		}
	}
	return list, nil
}
