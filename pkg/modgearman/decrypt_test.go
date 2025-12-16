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

func TestDecrypt1(t *testing.T) {
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

func TestDecrypt2(t *testing.T) {
	testdata := `5dp7w0Fk5oJBsrnWb31CgT5LHnbgtMi7KTcXz8OWVHDVsxDK2cfcWetQfYFin97e7i3J0ORAMTqSxQ7y/rENH1nVaWb6AzewELmj+TdYX85SDUWixWStx4PjYQ9kOSMLR4vpTx568fLtFUPd/9g5iQaAtdHZkNwp4/eeASqDAKOW2CqvrJyP1rvMLX/zsXGqSWO6FQMRf19yVFBiVexAJLDDFOEiU+APBAwc8Ds1OVcXpUCIefsPgFbLbAp/i72X+9/bmCjFfueV31ikSJX/w91XW08719z413UIBFF9I7toN9neHYZhUXjIUlm7RysK`
	cfg := config{}
	cfg.encryption = true
	cfg.key = `OtiuaSDFgpgEUrR6T0998egCAhCksIKh`
	key := getKey(&cfg)
	myCipher = createCipher(key, true)

	decoded, err := decodeBase64(testdata)
	require.NoError(t, err)
	result, err := decrypt(decoded, true)
	require.NoError(t, err)
	assert.Equalf(t, "service", result.typ, "expected type")
	assert.Equalf(t, "perl test", result.serviceDescription, "expected type")
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
