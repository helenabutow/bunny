package common

import (
	"sync"
	"time"
)

// TODO-HIGH: we probably want to make a copy of this in each package that references it (we really don't want a global shared mutex if we can avoid it)
var ResponseTimes map[string]*time.Duration = make(map[string]*time.Duration)

// TODO-HIGH: we also want a map of mutexes instead of one for everything
var ResponseTimesMutex sync.Mutex
