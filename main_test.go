package main

import (
	"bytes"
	"testing"
)

func TestValidate(t *testing.T) {
	monitor := Monitor{}

	err := monitor.validate()
	if err == nil {
		t.Errorf("Looking for %v, got %v", "We need to monitor at least one process", nil)
	}

	monitor.Processes = []string{"test"}

	err = monitor.validate()
	if err == nil {
		t.Errorf("Looking for %v, got %v", "Not all config variables present", nil)
	}

	monitor.Config.MessageBirdToken = "test"
	monitor.Config.MessageBirdSender = "test"
	monitor.Config.Recipients = "test"
	monitor.Config.DefaultTTLSeconds = 1
	monitor.Config.ServerNiceName = "test"

	err = monitor.validate()
	if err != nil {
		t.Errorf("Looking for %v, got %v", nil, err)
	}
}

func TestGetServerInfo(t *testing.T) {

}

func TestCheckProc(t *testing.T) {
	// @TODO Not sure how to test this without involving setting up a channel
}

func TestLineCount(t *testing.T) {
	line := bytes.NewBufferString("test one line\n")
	lines, err := lineCounter(line)
	if err != nil {
		t.Errorf("Looking for %v, got %v", nil, err)
	}
	if lines != 1 {
		t.Errorf("Looking for %v, got %v", 1, lines)
	}

	line = bytes.NewBufferString("test one line\ntest two lines\n")
	lines, err = lineCounter(line)
	if err != nil {
		t.Errorf("Looking for %v, got %v", nil, err)
	}
	if lines != 2 {
		t.Errorf("Looking for %v, got %v", 2, lines)
	}

	line = bytes.NewBufferString("test one line\ntest two lines\nthree\nfour\nfive\n")
	lines, err = lineCounter(line)
	if err != nil {
		t.Errorf("Looking for %v, got %v", nil, err)
	}
	if lines != 5 {
		t.Errorf("Looking for %v, got %v", 5, lines)
	}
}

func TestNotifyProcError(t *testing.T) {
	// @TODO
}
