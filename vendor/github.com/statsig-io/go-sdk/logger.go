package statsig

import (
	"strconv"
	"time"
)

const (
	maxEvents           = 500
	gateExposureEvent   = "statsig::gate_exposure"
	configExposureEvent = "statsig::config_exposure"
)

type exposureEvent struct {
	EventName          string              `json:"eventName"`
	User               User                `json:"user"`
	Value              string              `json:"value"`
	Metadata           map[string]string   `json:"metadata"`
	SecondaryExposures []map[string]string `json:"secondaryExposures"`
}

type logEventInput struct {
	Events          []interface{}   `json:"events"`
	StatsigMetadata statsigMetadata `json:"statsigMetadata"`
}

type logEventResponse struct{}

type logger struct {
	events    []interface{}
	transport *transport
	tick      *time.Ticker
}

func newLogger(transport *transport) *logger {
	log := &logger{
		events:    make([]interface{}, 0),
		transport: transport,
		tick:      time.NewTicker(time.Minute),
	}

	go log.backgroundFlush()

	return log
}

func (l *logger) backgroundFlush() {
	for range l.tick.C {
		l.flush(false)
	}
}

func (l *logger) logCustom(evt Event) {
	evt.User.PrivateAttributes = nil
	l.logInternal(evt)
}

func (l *logger) logExposure(evt exposureEvent) {
	evt.User.PrivateAttributes = nil
	l.logInternal(evt)
}

func (l *logger) logInternal(evt interface{}) {
	l.events = append(l.events, evt)
	if len(l.events) >= maxEvents {
		l.flush(false)
	}
}

func (l *logger) logGateExposure(
	user User,
	gateName string,
	value bool,
	ruleID string,
	exposures []map[string]string,
) {
	evt := &exposureEvent{
		User:      user,
		EventName: gateExposureEvent,
		Metadata: map[string]string{
			"gate":      gateName,
			"gateValue": strconv.FormatBool(value),
			"ruleID":    ruleID,
		},
		SecondaryExposures: exposures,
	}
	l.logExposure(*evt)
}

func (l *logger) logConfigExposure(
	user User,
	configName string,
	ruleID string,
	exposures []map[string]string,
) {
	evt := &exposureEvent{
		User:      user,
		EventName: configExposureEvent,
		Metadata: map[string]string{
			"config": configName,
			"ruleID": ruleID,
		},
		SecondaryExposures: exposures,
	}
	l.logExposure(*evt)
}

func (l *logger) flush(closing bool) {
	if closing {
		l.tick.Stop()
	}
	if len(l.events) == 0 {
		return
	}

	if closing {
		l.sendEvents(l.events)
	} else {
		go l.sendEvents(l.events)
	}

	l.events = make([]interface{}, 0)
}

func (l *logger) sendEvents(events []interface{}) {
	input := &logEventInput{
		Events:          events,
		StatsigMetadata: l.transport.metadata,
	}
	var res logEventResponse
	l.transport.retryablePostRequest("/log_event", input, &res, maxRetries)
}
