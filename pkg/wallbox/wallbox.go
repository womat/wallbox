package wallbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"sync"
	"time"

	"wallbox/pkg/debug"
)

const httpRequestTimeout = 10 * time.Second

const (
	On  State = "on"
	Off State = "off"

	ThresholdWallbox = 11000

	meterDoesntExists = "meter %q doesn't exists"
)

type State string

type Measurements struct {
	sync.RWMutex
	Timestamp time.Time
	Power     float64
	Energy    float64
	State     State
	Runtime   float64
	config    struct {
		meterURL string
	}
}

type readMeasurements struct {
	Timestamp time.Time
	Power     float64
	Energy    float64
	State     State
	Runtime   float64
}

/*
type meterURLBody struct {
	Timestamp time.Time `json:"Time"`
	Runtime   float64   `json:"Runtime"`
	Measurand struct {
		E float64 `json:"e"`
		P float64 `json:"p"`
	} `json:"Measurand"`
}
*/

type measurementURLBody struct {
	Value float64 `json:"Value"`
}

type meterURLBody struct {
	Timestamp time.Time                     `json:"Time"`
	Measurand map[string]measurementURLBody `json:"Measurand"`
}

type currentDataURLBody struct {
	Meter map[string]meterURLBody `json:"Meter"`
}

func New() *Measurements {
	return &Measurements{}
}

func (m *Measurements) SetMeterURL(url string) {
	m.config.meterURL = url
}

func (m *Measurements) Read() (err error) {
	var wg sync.WaitGroup

	data := New()
	data.SetMeterURL(m.config.meterURL)

	start := time.Now()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if e := data.readMeter(); e != nil {
			err = e
		}

		debug.TraceLog.Printf("runtime to request meter data: %vs", time.Since(start).Seconds())
	}()

	wg.Wait()
	debug.DebugLog.Printf("runtime to request data: %vs", time.Since(start).Seconds())

	m.Lock()
	defer m.Unlock()

	if data.Power > ThresholdWallbox {
		m.State = On
		m.Power = data.Power
	} else {
		m.State = Off
		m.Power = 0
	}

	m.Runtime = data.Runtime
	m.Timestamp = time.Now()
	return
}

func (m *Measurements) readMeter() (err error) {
	var r currentDataURLBody
	var boilerP, heatpumpP, inverterP, primaryP float64

	if err = read(m.config.meterURL, &r); err != nil {
		return
	}

	if boilerP, err = r.boilerP(); err != nil {
		return
	}
	if heatpumpP, err = r.heatpumpP(); err != nil {
		return
	}
	if inverterP, err = r.inverterP(); err != nil {
		return
	}
	if primaryP, err = r.primaryP(); err != nil {
		return
	}

	m.Lock()
	defer m.Unlock()
	m.Power = primaryP - heatpumpP - boilerP + inverterP
	return
}

func read(url string, data interface{}) (err error) {
	done := make(chan bool, 1)
	go func() {
		// ensures that data is sent to the channel when the function is terminated
		defer func() {
			select {
			case done <- true:
			default:
			}
			close(done)
		}()

		debug.TraceLog.Printf("performing http get: %v\n", url)

		var resp *http.Response
		if resp, err = http.Get(url); err != nil {
			return
		}

		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err = json.Unmarshal(bodyBytes, data); err != nil {
			return
		}
	}()

	// wait for API Data
	select {
	case <-done:
	case <-time.After(httpRequestTimeout):
		err = errors.New("timeout during receive data")
	}

	if err != nil {
		debug.ErrorLog.Println(err)
		return
	}
	return
}

func (c *currentDataURLBody) boilerP() (v float64, err error) {
	return c.power("boiler")
}

func (c *currentDataURLBody) dryerP() (v float64, err error) {
	return c.power("dryer")
}

func (c *currentDataURLBody) heatpumpP() (v float64, err error) {
	return c.power("heatpump")
}

func (c *currentDataURLBody) inverterP() (v float64, err error) {
	return c.power("inverter")
}

func (c *currentDataURLBody) primaryP() (v float64, err error) {
	return c.power("primarymeter")
}

func (c *currentDataURLBody) power(meter string) (v float64, err error) {
	if b, ok := c.Meter[meter]; ok {
		if v, ok := b.Measurand["p"]; ok {
			return v.Value, nil
		}
	}
	return math.NaN(), fmt.Errorf(meterDoesntExists, meter)
}
