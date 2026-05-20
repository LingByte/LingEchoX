package logger

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

var testLogger = New()

func TestErrorNotLost(t *testing.T) {
	formatter := &JSONFormatter{}

	b, err := formatter.Format(testLogger.WithField("error", errors.New("wild walrus")).entry)
	if err != nil {
		t.Fatal("Unable to format entry: ", err)
	}

	msgs := strings.Split(string(b), "\t")
	b = []byte(msgs[1:len(msgs)][0])

	entry := make(map[string]interface{})
	err = json.Unmarshal(b, &entry)
	if err != nil {
		t.Fatal("Unable to unmarshal formatted entry: ", err)
	}

	if entry["error"] != "wild walrus" {
		t.Fatal("Error field not set")
	}
}

func TestErrorNotLostOnFieldNotNamedError(t *testing.T) {
	formatter := &JSONFormatter{}

	b, err := formatter.Format(testLogger.WithField("omg", errors.New("wild walrus")).entry)
	if err != nil {
		t.Fatal("Unable to format entry: ", err)
	}

	msgs := strings.Split(string(b), "\t")
	b = []byte(msgs[1:len(msgs)][0])

	entry := make(map[string]interface{})
	err = json.Unmarshal(b, &entry)
	if err != nil {
		t.Fatal("Unable to unmarshal formatted entry: ", err)
	}

	if entry["omg"] != "wild walrus" {
		t.Fatal("Error field not set")
	}
}

func TestFieldClashWithTime(t *testing.T) {
	formatter := &JSONFormatter{}

	b, err := formatter.Format(testLogger.WithField("time", "right now!").entry)
	if err != nil {
		t.Fatal("Unable to format entry: ", err)
	}

	msgs := strings.Split(string(b), "\t")
	b = []byte(msgs[1:len(msgs)][0])

	entry := make(map[string]interface{})
	err = json.Unmarshal(b, &entry)
	if err != nil {
		t.Fatal("Unable to unmarshal formatted entry: ", err)
	}

	if entry["fields.time"] != "right now!" {
		t.Fatal("fields.time not set to original time field")
	}

	if entry["time"] != "0001-01-01 00:00:00" {
		t.Fatal("time field not set to current time, was: ", entry["time"])
	}
}

func TestFieldClashWithMsg(t *testing.T) {
	formatter := &JSONFormatter{}

	b, err := formatter.Format(testLogger.WithField("msg", "something").entry)
	if err != nil {
		t.Fatal("Unable to format entry: ", err)
	}

	msgs := strings.Split(string(b), "\t")
	b = []byte(msgs[1:len(msgs)][0])

	entry := make(map[string]interface{})
	err = json.Unmarshal(b, &entry)
	if err != nil {
		t.Fatal("Unable to unmarshal formatted entry: ", err)
	}

	if entry["fields.msg"] != "something" {
		t.Fatal("fields.msg not set to original msg field")
	}
}

func TestFieldClashWithLevel(t *testing.T) {
	formatter := &JSONFormatter{}

	b, err := formatter.Format(testLogger.WithField("level", "something").entry)
	if err != nil {
		t.Fatal("Unable to format entry: ", err)
	}

	msgs := strings.Split(string(b), "\t")
	b = []byte(msgs[1:len(msgs)][0])

	entry := make(map[string]interface{})
	err = json.Unmarshal(b, &entry)
	if err != nil {
		t.Fatal("Unable to unmarshal formatted entry: ", err)
	}

	if entry["fields.level"] != "something" {
		t.Fatal("fields.level not set to original level field")
	}
}

func TestJSONEntryEndsWithNewline(t *testing.T) {
	formatter := &JSONFormatter{}

	b, err := formatter.Format(testLogger.WithField("level", "something").entry)
	if err != nil {
		t.Fatal("Unable to format entry: ", err)
	}

	if b[len(b)-1] != '\n' {
		t.Fatal("Expected JSON log entry to end with a newline")
	}
}

func TestJSONMessageKey(t *testing.T) {
	formatter := &JSONFormatter{
		FieldMap: FieldMap{
			FieldKeyMsg: "message",
		},
	}

	b, err := formatter.Format(&Entry{Message: "oh hai"})
	if err != nil {
		t.Fatal("Unable to format entry: ", err)
	}
	s := string(b)
	if !(strings.Contains(s, "message") && strings.Contains(s, "oh hai")) {
		t.Fatal("Expected JSON to format message key")
	}
}

func TestJSONTimeKey(t *testing.T) {
	formatter := &JSONFormatter{
		FieldMap: FieldMap{
			FieldKeyTime: "timeywimey",
		},
	}

	b, err := formatter.Format(testLogger.WithField("level", "something").entry)
	if err != nil {
		t.Fatal("Unable to format entry: ", err)
	}
	s := string(b)
	if !strings.Contains(s, "timeywimey") {
		t.Fatal("Expected JSON to format time key")
	}
}

func TestJSONDisableTimestamp(t *testing.T) {
	formatter := &JSONFormatter{
		DisableTimestamp: true,
	}

	b, err := formatter.Format(testLogger.WithField("level", "something").entry)
	if err != nil {
		t.Fatal("Unable to format entry: ", err)
	}
	s := string(b)
	if strings.Contains(s, FieldKeyTime) {
		t.Error("Did not prevent timestamp", s)
	}
}

func TestJSONEnableTimestamp(t *testing.T) {
	formatter := &JSONFormatter{}

	b, err := formatter.Format(testLogger.WithField("level", "something").entry)
	if err != nil {
		t.Fatal("Unable to format entry: ", err)
	}
	s := string(b)
	if !strings.Contains(s, FieldKeyTime) {
		t.Error("Timestamp not present", s)
	}
}
