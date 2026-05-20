package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	assert := assert.New(t)

	log := New()
	assert.NotNil(log)

	log.WithField("f1", "f1 value").Info("test")
	log.WithFields(Fields{"a": 1, "b": 2}).Info("test info")

	log = New("custom-req-id")
	assert.NotNil(log)

	log.WithField("f2", "f2 value").Info("test")
	log.WithFields(Fields{"a": 1, "b": 2}).Info("test info2")

	// StandardLogger
	log = StandardLogger()
	assert.NotNil(log)

	SetFormatter(&JSONFormatter{
		CallerSkip: 5,
	})

	Debug("Debug info")
	Print("Print info")
	Info("Info info")
	Warn("Warn info")
	Warning("Warning info")
	Error("Error info")
	Debugf("Debugf info")
	Printf("Printf info")
	Infof("Infof info")
	Warnf("Warnf info")
	Warningf("Warningf info")
	Errorf("Errorf info")
	Debugln("Debugln info")
	Println("Println info")
	Infoln("Infoln info")
	Warnln("Warnln info")
	Warningln("Warningln info")
	Errorln("Errorln info")
}
