package sessions_mongo

import (
	"fmt"
	"time"
)

type InvalidTTLErr struct {
	invalidTTL time.Duration
}

func NewInvalidTTLErr(ttl time.Duration) *InvalidTTLErr {
	return &InvalidTTLErr{invalidTTL: ttl}
}

func (e *InvalidTTLErr) Error() string {
	return fmt.Sprintf("ttl cannot be 0 or fewer seconds; supplies ttl: %d", int(e.invalidTTL.Seconds()))
}

