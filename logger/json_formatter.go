package logger

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
)

// This is to not silently overwrite `time`, `msg` and `level` fields when
// dumping it. If this code wasn't there doing:
//
//  logrus.WithField("level", 1).Info("hello")
//
// Would just silently drop the user provided level. Instead with this code
// it'll logged as:
//
//  {"level": "info", "fields.level": 1, "msg": "hello", "time": "..."}
//
// It's not exported because it's still using Data in an opinionated way. It's to
// avoid code duplication between the two default formatters.
func prefixFieldClashes(data Fields) {
	if t, ok := data["time"]; ok {
		data["fields.time"] = t
	}

	if m, ok := data["msg"]; ok {
		data["fields.msg"] = m
	}

	if l, ok := data["level"]; ok {
		data["fields.level"] = l
	}
}

// ------------------------------------------------------------------------

type fieldKey string

// FieldMap allows customization of the key names for default fields.
type FieldMap map[fieldKey]string

// Default key names for the default fields
const (
	FieldKeyMsg   = "msg"
	FieldKeyLevel = "level"
	FieldKeyTime  = "time"
)

func (f FieldMap) resolve(key fieldKey) string {
	if k, ok := f[key]; ok {
		return k
	}

	return string(key)
}

// JSONFormatter formats logs into parsable json
type JSONFormatter struct {
	// TimestampFormat sets the format used for marshaling timestamps.
	TimestampFormat string

	// DisableTimestamp allows disabling automatic timestamps in output
	DisableTimestamp bool

	// FieldMap allows users to customize the names of keys for default fields.
	// As an example:
	// formatter := &JSONFormatter{
	//   	FieldMap: FieldMap{
	// 		 FieldKeyTime: "@timestamp",
	// 		 FieldKeyLevel: "@level",
	// 		 FieldKeyMsg: "@message",
	//    },
	// }
	FieldMap FieldMap

	// CallerSkip runtime caller skip
	CallerSkip int
}

// Format renders a single log entry
func (f *JSONFormatter) Format(entry *Entry) ([]byte, error) {
	data := make(Fields, len(entry.Data)+3)
	for k, v := range entry.Data {
		switch v := v.(type) {
		case error:
			// Otherwise errors are ignored by `encoding/json`
			// https://github.com/sirupsen/logrus/issues/137
			data[k] = v.Error()
		default:
			data[k] = v
		}
	}
	prefixFieldClashes(data)

	timestampFormat := f.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = defaultTimestampFormat
	}

	currentTime := entry.Time.Format(timestampFormat)

	if !f.DisableTimestamp {
		data[f.FieldMap.resolve(FieldKeyTime)] = currentTime
	}
	data[f.FieldMap.resolve(FieldKeyMsg)] = entry.Message

	reqID := fmt.Sprintf("[%v] ", data["x-reqid"])
	currentTime = fmt.Sprintf("\033[33m[%s]\033[0m ", currentTime)
	level := "[" + strings.ToUpper(entry.Level.String()) + "] "

	if f.CallerSkip == 0 {
		f.CallerSkip = 4
	}
	var info string
	if _, file, line, ok := runtime.Caller(f.CallerSkip); ok {
		info = fmt.Sprintf("\033[35m(%s:%d)\033[0m", file, line)
	}

	delete(data, "x-reqid")
	serialized, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("Failed to marshal fields to JSON, %v", err)
	}

	msg := []byte(reqID + currentTime + level + info + "\t")
	msg = append(msg, serialized...)
	return append(msg, '\n'), nil
}
