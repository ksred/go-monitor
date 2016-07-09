package main

import (
	"bytes"
	"io/ioutil"
	"testing"
)

func TestParseConf(t *testing.T) {
	conf := ymlConf{}

	// Construct yaml data
	data := `
processes:
config:
`
	err := parseConf([]byte(data), &conf)
	if err == nil {
		t.Errorf("Looking for %v, got %v", "We need to monitor at least one process", nil)
	}
	data = `
processes: [ "test" ]
config:
`
	err = parseConf([]byte(data), &conf)
	if err == nil {
		t.Errorf("Looking for %v, got %v", "Not all config variables present", nil)
	}
	data = `
processes: [ "test" ]
config:
  messagebirdtoken: "test"
  messagebirdsender: "test"
  recipients: "test"
  defaultttl: 1
  servernicename: "test"
`
	err = parseConf([]byte(data), &conf)
	if err != nil {
		t.Errorf("Looking for %v, got %v", nil, err)
	}
}

func TestParseConfFromFile(t *testing.T) {
	conf := ymlConf{}
	data, err := ioutil.ReadFile("go-monitor.yml")
	if err != nil {
		t.Errorf("Looking for %v, got %v", nil, err)
	}
	err = parseConf(data, &conf)
	if err != nil {
		t.Errorf("Looking for %v, got %v", nil, err)
	}
}

func TestGetServerInfo(t *testing.T) {
	conf := ymlConf{}
	data, err := ioutil.ReadFile("go-monitor.yml")
	if err != nil {
		t.Errorf("Looking for %v, got %v", nil, err)
	}
	err = parseConf(data, &conf)
	if err != nil {
		t.Errorf("Looking for %v, got %v", nil, err)
	}

	_, err = getServerInfo(&conf)
	if err != nil {
		t.Errorf("Looking for %v, got %v", nil, err)
	}
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
