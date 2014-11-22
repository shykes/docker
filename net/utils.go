package net

import (
	"github.com/docker/docker/extensions"

	log "github.com/Sirupsen/logrus"
)

func MockExtension(logger *log.Logger) *mockExtension {
	logger.Level = log.DebugLevel
	return &mockExtension{
		logger: logger,
	}
}

type mockExtension struct {
	logger *log.Logger
}

func (m *mockExtension) Log(s string) {
	if m.logger != nil {
		m.logger.Debug([]byte(s))
	}
}

func (m *mockExtension) Logf(format string, args ...interface{}) {
	if m.logger != nil {
		m.logger.Debugf(format, args...)
	}
}

func (m *mockExtension) Opts() []extensions.Opt {
	m.Log("extensions.NetExtension.Opts()")
	return []extensions.Opt{}
}

func (m *mockExtension) Init(s extensions.State) error {
	m.Log("extensions.NetExtension.Init()")
	return nil
}

func (m *mockExtension) Shutdown(s extensions.State) error {
	m.Log("extensions.NetExtension.Shutdown()")
	return nil
}

func (m *mockExtension) AddNet(netid string, s extensions.State) error {
	m.Logf("extensions.NetExtension.AddNet(%v)", netid)
	return nil
}

func (m *mockExtension) RemoveNet(netid string, s extensions.State) error {
	m.Logf("extensions.NetExtension.RemoveNet(%v)", netid)
	return nil
}

func (m *mockExtension) AddEndpoint(netid, epid string, s extensions.State) ([]*extensions.Interface, error) {
	m.Logf("extensions.NetExtension.AddEndpoint(%v, %v)", netid, epid)
	return nil, nil
}

func (m *mockExtension) RemoveEndpoint(netid, epid string, s extensions.State) error {
	m.Logf("extensions.NetExtension.RemoveEndpoint(%v, %v)", netid, epid)
	return nil
}
