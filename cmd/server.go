package main

import (
	"time"

	"wallbox/pkg/debug"
	"wallbox/pkg/wallbox"
)

func (r *heatPumpRuntime) backupMeasurements(f string, p time.Duration) {
	for range time.Tick(p) {
		_ = saveMeasurements(f, r.data)
	}
}

func (r *heatPumpRuntime) calcRuntime(p time.Duration) {
	runtime := func(state wallbox.State, lastStateDate *time.Time, lastState *wallbox.State) (runTime float64) {
		if state == wallbox.On {
			if *lastState != wallbox.On {
				*lastStateDate = time.Now()
			}
			runTime = time.Since(*lastStateDate).Hours()
			*lastStateDate = time.Now()
		}
		*lastState = state
		return
	}

	ticker := time.NewTicker(p)
	defer ticker.Stop()

	for ; true; <-ticker.C {
		debug.DebugLog.Println("get data")

		temp := wallbox.New()
		*temp = *r.data

		if err := temp.Read(); err != nil {
			debug.ErrorLog.Printf("get heatpump data: %v", err)
			continue
		}

		func() {
			debug.DebugLog.Println("calc runtime")
			r.Lock()
			defer r.Unlock()

			*r.data = *temp
			r.data.Runtime += runtime(r.data.State, &r.lastStateDate, &r.lastState)
		}()
	}
}

func in(s interface{}, pattern ...interface{}) bool {
	for _, p := range pattern {
		if s == p {
			return true
		}
	}
	return false
}
