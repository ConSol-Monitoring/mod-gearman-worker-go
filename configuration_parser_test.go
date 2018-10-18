package modgearman

import (
	"os"
	"testing"
)

func TestReadSettingsFile(t *testing.T) {
	var testConfig configurationStruct

	//set default values so we can check if they get overwritten
	setDefaultValues(&testConfig)

	f, err := os.Create("testConfigFile")
	if err != nil {
		t.Errorf("could not create config testfile")
	}

	f.Write([]byte(`#ich bin ein kommentar also werde ich ignoriert
debug=2
servicegroups=a,b,c
servicegroups=d
idle-timeout=200
server=hostname:4730
server=:4730
server=hostname
`))

	readSettingsFile("testConfigFile", &testConfig)

	if testConfig.debug != 2 {
		t.Errorf("wrong value expected 2 got %d", testConfig.debug)
	}

	if len(testConfig.servicegroups) != 4 {
		t.Errorf("servicegroups len false expected 4 got %d", len(testConfig.servicegroups))
	}

	if testConfig.idleTimeout != 200 {
		t.Errorf("idle_timeout should have been overwritten to 200 but is %d", testConfig.idleTimeout)
	}

	if testConfig.server[0] != "hostname:4730" {
		t.Errorf("server 1 parsed incorrect: '%s' vs. 'hostname:4730'", testConfig.server[0])
	}

	if testConfig.server[1] != "0.0.0.0:4730" {
		t.Errorf("server 2 parsed incorrect: '%s' vs. '0.0.0.0:4730'", testConfig.server[1])
	}

	if testConfig.server[2] != "hostname:4730" {
		t.Errorf("server 3 parsed incorrect: '%s' vs. 'hostname:4730'", testConfig.server[2])
	}

	os.Remove("testConfigFile")

}

func TestGetFloat(t *testing.T) {
	disableLogging()

	//int value, float value, string value
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

	//int value, float value, string value
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
