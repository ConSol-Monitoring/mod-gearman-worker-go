package main

import (
	"os"
	"testing"
)

func TestGetKey(t *testing.T) {
	//set the key in the config struct
	config.key = "MeinTestKey"
	//get the key from the method
	result := getKey()

	if string(result) != "MeinTestKey" {
		t.Errorf("expected: %s, got:%s", "MeinTestKey", result)
	}

	config.key = ""
	config.keyfile = "testfile.key"
	//create the testfile
	f, err := os.Create(config.keyfile)
	if err != nil {
		t.Errorf("could not create testKeyFile %s", err.Error())
	}
	_, err = f.WriteString("MeinTestKey")
	if err != nil {
		t.Errorf("could not write to testFile %s", err.Error())
	}

	result = getKey()

	if string(result) != "MeinTestKey" {
		t.Errorf("getkey from file expected: %s, got:%s", "MeinTestKey", result)
	}

	//remove the testfile
	err = os.Remove(config.keyfile)
	if err != nil {
		t.Errorf("could not delete testKeyfile %s", err)
	}
}

func TestOpenFileOrCreate(t *testing.T) {
	_, err := openFileOrCreate("testdirectory/with/sub/and/file.txt")
	if err != nil {
		t.Errorf("error opening file got error: %s", err.Error())
	}

	os.RemoveAll("testdirectory")
}
