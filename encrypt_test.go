package main

import "testing"

func TestEncodeBase64(t *testing.T) {
	input := "Encode me"
	result := encodeBase64(input)
	if string(result) != "RW5jb2RlIG1l" {
		t.Errorf("expected: %s, got:%s", "RW5jb2RlIG1l", string(result))
	}
}

func TestEncrypt(t *testing.T) {
	config.encryption = true
	input := "encrypt me please"
	result := encrypt(input, []byte("LaDPjcEqfZuKnUJStXHX27bxkHLAHSbD"))
	//base64 encoded so the error message gets readable
	expectedResult := "xzJEkKA+e/nBusozXOuPBz9hzIgUsEpHrZT0yo+HaPw="

	if encodeBase64(result) != expectedResult {
		t.Errorf("expected: %s, got:%s", []byte(expectedResult), string(result))
	}
}
