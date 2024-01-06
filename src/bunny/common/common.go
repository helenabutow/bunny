package common

import (
	"sync"
	"time"
)

var ResponseTimes map[string]*time.Duration = make(map[string]*time.Duration)
var ResponseTimesMutex sync.Mutex
