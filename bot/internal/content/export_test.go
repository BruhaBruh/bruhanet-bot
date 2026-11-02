package content

import "time"

func SetRefreshTimeout(p *Provider, d time.Duration) { p.refreshTimeout = d }
