package sessions_mongo

import (
	"time"
)

//Options is a collection of settings and options relevant to the implementation
//of the Store
type Options struct {
	TTLOptions    TTLOptions
	Logging       LoggingOptions
}

//TTLOptions is a collection of settings and options regarding the TimeToLive
//functionality of the Store
type TTLOptions struct {
	EnsureTTLIndex bool
	TTL time.Duration
}

//LoggingOptions is a collection of settings and options regarding the logging
//capabilities of the implementation of the Store
type LoggingOptions struct {
	Enabled bool
}

//Validate does a sanity check on relevant options that can be modified by
//an implementing developer.
func (o Options) Validate() error {
	if o.TTLOptions.TTL.Seconds() <= 0 {
		return NewInvalidTTLErr(o.TTLOptions.TTL)
	}

	return nil
}
