package modgearman

import (
	b64 "encoding/base64"
)

const EncryptionBlockSize = 16

func createAnswer(value *answer, withEncrypt bool) []byte {
	encrypted := encrypt([]byte(value.String()), withEncrypt)
	return encodeBase64(encrypted)
}

/**
* encrypts a given string with a given key, returns the encrypted string
*
 */
func encrypt(data []byte, encrypt bool) []byte {
	if !encrypt {
		return data
	}
	// len(data) needs to be multiple of 16 (EncryptionBlockSize)
	if (len(data) % EncryptionBlockSize) != 0 {
		times := (EncryptionBlockSize - 1) - (len(data) % EncryptionBlockSize)

		for i := 0; i <= times; i++ {
			data = append(data, 10)
		}
	}

	encrypted := make([]byte, len(data))

	for bs, be := 0, EncryptionBlockSize; bs < len(data); bs, be = bs+EncryptionBlockSize, be+EncryptionBlockSize {
		myCipher.Encrypt(encrypted[bs:be], data[bs:be])
	}

	return encrypted
}

/**
* encodes the given string with base64
* returns the base64 encoded string
 */
func encodeBase64(data []byte) []byte {
	encodedBase := b64.StdEncoding.EncodeToString(data)
	return []byte(encodedBase)
}
