package modgearman

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSettingsFile(t *testing.T) {
	var testConfig config

	// set default values so we can check if they get overwritten
	testConfig.setDefaultValues()

	file, err := os.Create("testConfigFile")
	require.NoError(t, err)

	file.WriteString(`#ich bin ein kommentar also werde ich ignoriert
debug=2
servicegroups=a,b,c
servicegroups=d
hostgroups=
idle-timeout=200
server=hostname:4730
server=:4730
server=hostname
server=hostname2
`)

	testConfig.readSettingsFile("testConfigFile")
	testConfig.cleanListAttributes()

	assert.Equalf(t, 2, testConfig.debug, "debug set to 2")
	assert.Equal(t, []string{"a", "b", "c", "d"}, testConfig.servicegroups)
	assert.Equal(t, []string{}, testConfig.hostgroups)
	assert.Equal(t, 200, testConfig.idleTimeout)
	assert.Equal(t, []string{"hostname:4730", "0.0.0.0:4730", "hostname2:4730"}, testConfig.server)

	os.Remove("testConfigFile")
}

func TestReadSettingsPath(t *testing.T) {
	var testConfig config

	// set default values so we can check if they get overwritten
	testConfig.setDefaultValues()

	err := os.Mkdir("testConfigFolder", 0o755)
	require.NoError(t, err)

	file, err := os.Create("testConfigFolder/file.cfg")
	require.NoError(t, err)

	file.WriteString(`#ich bin ein kommentar also werde ich ignoriert
debug=2
servicegroups=a,b,c
servicegroups=d
hostgroups=
idle-timeout=200
server=hostname:4730
server=:4730
server=hostname
server=hostname2
`)

	testConfig.readSettingsPath("testConfigFolder")
	testConfig.cleanListAttributes()

	assert.Equalf(t, 2, testConfig.debug, "debug set to 2")
	assert.Equal(t, []string{"a", "b", "c", "d"}, testConfig.servicegroups)
	assert.Equal(t, []string{}, testConfig.hostgroups)
	assert.Equal(t, 200, testConfig.idleTimeout)
	assert.Equal(t, []string{"hostname:4730", "0.0.0.0:4730", "hostname2:4730"}, testConfig.server)

	os.RemoveAll("testConfigFolder")
}

func TestGetFloat(t *testing.T) {
	disableLogging()

	// int value, float value, string value
	value := getFloat("1")
	if value != 1 {
		t.Errorf("wrong value expected 1 got %f", value)
	}

	value = getFloat("1.2345")
	if value != 1.2345 {
		t.Errorf("wrong value expected 1.2345 got %f", value)
	}

	value = getFloat("abc")
	if value != 0 {
		t.Errorf("wrong value expected 0 got %f", value)
	}

	// restore error loglevel
	setLogLevel(0)
}

func TestGetInt(t *testing.T) {
	disableLogging()

	// int value, float value, string value
	value := getInt("1")
	if value != 1 {
		t.Errorf("wrong value expected 1 got %d", value)
	}

	value = getInt("1.2345")
	if value != 1 {
		t.Errorf("wrong value expected 1 got %d", value)
	}

	value = getInt("abc")
	if value != 0 {
		t.Errorf("wrong value expected 0 got %d", value)
	}

	// restore error loglevel
	setLogLevel(0)
}

func TestGetBool(t *testing.T) {
	value := getBool("1")
	if !value {
		t.Errorf("wrong value expected true got false")
	}

	value = getBool("yes")
	if !value {
		t.Errorf("wrong value expected true got false")
	}

	value = getBool("on")
	if !value {
		t.Errorf("wrong value expected true got false")
	}

	value = getBool(";jklsfad")
	if value {
		t.Errorf("wrong value expected false got true")
	}
}
