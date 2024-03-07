package modgearman

import (
	"os"
	"testing"
)

func TestGetKey(t *testing.T) {
	// set the key in the cfg struct
	cfg := config{key: "MeinTestKey", encryption: true}

	// get the key from the method
	result := getKey(&cfg)

	if string(result[0:11]) != "MeinTestKey" {
		t.Errorf("expected: %s, got:%s", "MeinTestKey", result)
	}

	if len(result) != 32 {
		t.Errorf("length expected: %d, got: %d", 32, len(result))
	}

	if result[31] != 0 {
		t.Errorf("key must be rightpadded with zero bytes")
	}

	cfg.key = ""
	cfg.keyfile = "testfile.key"
	// create the testfile
	f, err := os.Create(cfg.keyfile)
	if err != nil {
		t.Errorf("could not create testKeyFile %s", err.Error())
	}
	_, err = f.WriteString("MeinTestKey")
	if err != nil {
		t.Errorf("could not write to testFile %s", err.Error())
	}

	result = getKey(&cfg)

	if string(result[0:11]) != "MeinTestKey" {
		t.Errorf("getkey from file expected: %s, got:%s", "MeinTestKey", result)
	}

	// remove the testfile
	err = os.Remove(cfg.keyfile)
	if err != nil {
		t.Errorf("could not delete testKeyfile %s", err.Error())
	}
}

func TestOpenFileOrCreate(t *testing.T) {
	_, err := openFileOrCreate("testdirectory/with/sub/and/file.txt")
	if err != nil {
		t.Errorf("error opening file got error: %s", err.Error())
	}

	os.RemoveAll("testdirectory")
}
