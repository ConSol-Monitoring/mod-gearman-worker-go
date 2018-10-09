package main

import (
	"testing"
)

func TestDecodeBase64(t *testing.T) {
	base64Encoded := "VGVzdCBFbmNvZGVkIFN0cmluZw=="
	result := decodeBase64(base64Encoded)

	if string(result) != "Test Encoded String" {
		t.Errorf("expected: %s, got:%s", "Test Encoded String", string(result))
	}

}

func TestCreateMap(t *testing.T) {
	testValue := []byte("einkaufen=wasser\n essen=gut")
	result := createMap(testValue)
	if result["einkaufen"] != "wasser" || result["essen"] != "gut" {
		t.Errorf("expected: %s, got:%s", "einkaufen=wasser and essen=gut", result)
	}
}

func TestDecrypt(t *testing.T) {
	config.encryption = true

	encrypted := encrypt("type=test123", []byte("LaDPjcEqfZuKnUJStXHX27bxkHLAHSbD"))
	result := decrypt([]byte(encrypted), []byte("LaDPjcEqfZuKnUJStXHX27bxkHLAHSbD"))
	if result.typ != "test123" {
		t.Errorf("expected: %s, got:%s", "test123", result.typ)
	}
}
