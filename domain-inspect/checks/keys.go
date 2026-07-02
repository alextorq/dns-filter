package checks

import "sync/atomic"

// Credentials is the process-local, runtime-updatable key store shared by
// manual domain inspection, the suggest-inspect worker and settings Apply
// hooks. It is instantiated in main; separate instances are fully isolated.
type Credentials struct {
	vtKey atomic.Pointer[string]
	sbKey atomic.Pointer[string]
}

func NewCredentials() *Credentials { return &Credentials{} }

func (c *Credentials) SetVirusTotal(key string) {
	v := key
	c.vtKey.Store(&v)
}

func (c *Credentials) VirusTotalKey() string {
	if p := c.vtKey.Load(); p != nil {
		return *p
	}
	return ""
}

func (c *Credentials) SetSafeBrowsing(key string) {
	v := key
	c.sbKey.Store(&v)
}

func (c *Credentials) SafeBrowsingKey() string {
	if p := c.sbKey.Load(); p != nil {
		return *p
	}
	return ""
}

func (c *Credentials) HasAnyKey() bool {
	return c.VirusTotalKey() != "" || c.SafeBrowsingKey() != ""
}
