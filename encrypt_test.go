package modgearman

import "testing"

func TestEncodeBase64(t *testing.T) {
	input := []byte("Encode me")
	result := encodeBase64(input)
	if string(result) != "RW5jb2RlIG1l" {
		t.Errorf("expected: %s, got:%s", "RW5jb2RlIG1l", string(result))
	}
}

func TestEncrypt(t *testing.T) {
	config := configurationStruct{}
	config.encryption = true
	input := "encrypt me please"
	result := encrypt([]byte(input), []byte("LaDPjcEqfZuKnUJStXHX27bxkHLAHSbD"), true)
	//base64 encoded so the error message gets readable
	expectedResult := "xzJEkKA+e/nBusozXOuPBz9hzIgUsEpHrZT0yo+HaPw="

	if string(encodeBase64(result)) != expectedResult {
		t.Errorf("expected: %s, got:%s", expectedResult, string(encodeBase64(result)))
	}
}
