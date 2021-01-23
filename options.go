package sessions_mongo

import (
	"time"
)


type Options struct {
	TTLOptions    TTLOptions
	Logging       LoggingOptions
	EnableLogging bool
}

type TTLOptions struct {
	EnsureTTLIndex bool
	TTL time.Duration
}

type LoggingOptions struct {
	Enabled bool
}

func (o Options) Validate() error {
	if o.TTLOptions.TTL.Seconds() <= 0 {
		return NewInvalidTTLErr(o.TTLOptions.TTL)
	}

	return nil
}
