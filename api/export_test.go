package api

import "context"

// SetLookupTXT replaces the DNS resolver used to check DNS-01 records.
func SetLookupTXT(lookup func(ctx context.Context, server, name string) ([]string, error)) (restore func()) {
	previous := lookupTXT
	lookupTXT = lookup
	return func() { lookupTXT = previous }
}
