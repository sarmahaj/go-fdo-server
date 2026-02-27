package db

import (
	"testing"
)


// FuzzParseHumanToTO2AddrsJSON ensures the TO2 addresses parser never panics on arbitrary inputs.
func FuzzParseHumanToTO2AddrsJSON(f *testing.F) {
	// Seed with a minimal valid example
	f.Add([]byte(`[{"dns":"owner.example.com","port":"1","protocol":"http"}]`))
	// Seed with IP-only
	f.Add([]byte(`[{"ip":"192.168.1.10","port":"65535","protocol":"https"}]`))
	// Seed with both DNS and IP
	f.Add([]byte(`[{"dns":"owner.example.com","ip":"10.0.0.5","port":"65535","protocol":"tls"}]`))
	// Seed with invalid IPs
	f.Add([]byte(`[{"ip":"300.300.300.300","port":"8043","protocol":"http"}]`))
	f.Add([]byte(`[{"ip":"not.an.ip","port":"8043","protocol":"http"}]`))
	// Seed malformed JSON
	f.Add([]byte(`[{bad}]`))
	// Seed with missing fields
	f.Add([]byte(`[]`))
	// Seed empty object
	f.Add([]byte(`{}`))
	// Seed bad types
	f.Add([]byte(`[{"dns":123,"ip":false,"port":{},"protocol":[]}]`))
	// Seed large list
	f.Add([]byte(`[{"dns":"a.com","port":"1"},{"dns":"b.com","ip":"10.0.0.2","port":"2"}]`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic
		_, _ = parseHumanToTO2AddrsJSON(data)
	})
}
