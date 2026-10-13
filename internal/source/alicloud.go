package source

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rakaprad/alert-copilot/internal/alert"
)

// ParseAliCloud parses an Alibaba Cloud CloudMonitor alert callback, which is
// sent as application/x-www-form-urlencoded. Relevant fields:
//
//	alertName, alertState (ALERT | OK | INSUFFICIENT_DATA), triggerLevel
//	(CRITICAL | WARN | INFO), instanceName, regionName, curValue,
//	metricName, namespace, expression, dimensions, timestamp (ms).
func ParseAliCloud(form url.Values) ([]alert.Alert, error) {
	name := form.Get("alertName")
	if name == "" {
		name = form.Get("metricName")
	}
	if name == "" {
		return nil, errors.New("alicloud callback missing alertName")
	}

	status := alert.StatusFiring
	if strings.EqualFold(form.Get("alertState"), "OK") {
		status = alert.StatusResolved
	}

	labels := map[string]string{}
	for _, k := range []string{"regionName", "regionId", "namespace", "metricName", "metricProject", "dimensions", "ruleId", "alertState", "lastTime"} {
		if v := form.Get(k); v != "" {
			labels[k] = v
		}
	}

	a := alert.Alert{
		Source:   "alicloud",
		Name:     name,
		Status:   status,
		Severity: strings.ToLower(form.Get("triggerLevel")),
		Target:   firstNonEmpty(form.Get("instanceName"), form.Get("dimensions")),
		Summary:  form.Get("expression"),
		Value:    form.Get("curValue"),
		Labels:   labels,
		// Stable identity across repeats: lastTime/curValue change on every
		// callback, so we key on the rule and the resource instead.
		ID: firstNonEmpty(form.Get("ruleId"), name) + "/" + firstNonEmpty(form.Get("dimensions"), form.Get("instanceName")),
	}
	if ms, err := strconv.ParseInt(form.Get("timestamp"), 10, 64); err == nil {
		a.StartsAt = time.UnixMilli(ms).UTC()
	}
	return []alert.Alert{a}, nil
}
