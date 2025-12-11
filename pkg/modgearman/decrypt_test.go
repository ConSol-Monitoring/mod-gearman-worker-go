package modgearman

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeBase64(t *testing.T) {
	base64Encoded := "VGVzdCBFbmNvZGVkIFN0cmluZw=="
	result, err := decodeBase64(base64Encoded)
	require.NoError(t, err)

	assert.Equalf(t, "Test Encoded String", string(result), "expected: %s, got:%s", "Test Encoded String", string(result))
}

func TestCreateMap(t *testing.T) {
	testValue := []byte("einkaufen=wasser\n essen=gut")
	result := createMap(testValue)
	if result["einkaufen"] != "wasser" || result["essen"] != "gut" {
		t.Errorf("expected: %s, got:%s", "einkaufen=wasser and essen=gut", result)
	}
}

func TestDecrypt(t *testing.T) {
	cfg := config{}
	cfg.encryption = true
	cfg.key = "LaDPjcEqfZuKnUJStXHX27bxkHLAHSbD"
	key := getKey(&cfg)
	myCipher = createCipher(key, true)
	encrypted := encrypt([]byte("type=test123"), true)
	result, err := decrypt(encrypted, true)
	require.NoError(t, err)
	assert.Equalf(t, "test123", result.typ, "expected: %s, got:%s", "test123", result.typ)
}

func TestDecryptErrors1(t *testing.T) {
	brokenB64 := "VGVzdCBFbmNvZGVkIFN.-12456"
	result, err := decryptJobData([]byte(brokenB64), true)
	assert.Nilf(t, result, "no result expected, got:%s", result)
	assert.Errorf(t, err, "expected an error")
}

func TestDecryptErrors2(t *testing.T) {
	// valid base64 but invalid cipher text
	brokenB64 := "dGVzdGFzamhka2phaHNramRoYWtqc2hka2phaGtqc2RoamthaHNkamtoYWtqc2hkawo="
	result, err := decryptJobData([]byte(brokenB64), true)
	assert.Nilf(t, result, "no result expected, got:%s", result)
	assert.Errorf(t, err, "expected an error")
}
