package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/womat/debug"

	"wallbox/global"
	"wallbox/pkg/wallbox"
)

type heatPumpRuntime struct {
	sync.RWMutex
	data          *wallbox.Measurements
	lastState     wallbox.State
	lastRuntime   float64
	lastStateDate time.Time
}

func main() {
	debug.SetDebug(global.Config.Debug.File, global.Config.Debug.Flag)

	global.Measurements = wallbox.New()
	global.Measurements.SetMeterURL(global.Config.MeterURL)

	if err := loadMeasurements(global.Config.DataFile, global.Measurements); err != nil {
		debug.ErrorLog.Printf("can't open data file: %v\n", err)
		os.Exit(1)
		return
	}

	runtime := &heatPumpRuntime{
		data:          global.Measurements,
		lastState:     wallbox.Off,
		lastRuntime:   global.Measurements.Runtime,
		lastStateDate: time.Now(),
	}

	go runtime.calcRuntime(global.Config.DataCollectionInterval)
	go runtime.backupMeasurements(global.Config.DataFile, global.Config.BackupInterval)

	// capture exit signals to ensure resources are released on exit.
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	// wait for am os.Interrupt signal (CTRL C)
	sig := <-quit
	debug.InfoLog.Printf("Got %s signal. Aborting...\n", sig)
	_ = saveMeasurements(global.Config.DataFile, global.Measurements)
	os.Exit(1)
}
