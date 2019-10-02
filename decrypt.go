package modgearman

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	b64 "encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

var myCipher cipher.Block

type receivedStruct struct {
	typ                string
	resultQueue        string
	hostName           string
	serviceDescription string
	startTime          float64
	nextCheck          float64
	coreTime           float64
	timeout            int
	commandLine        string
}

func (r *receivedStruct) String() string {
	return fmt.Sprintf(
		"\n\t type: %s\n"+
			"\t result_queue: %s\n"+
			"\t host_name: %s\n"+
			"\t service_description: %s\n"+
			"\t start_time: %f\n"+
			"\t next_check: %f\n"+
			"\t core_time: %f\n"+
			"\t timeout: %d\n"+
			"\t command_line: %s\n\n",
		r.typ,
		r.resultQueue,
		r.hostName,
		r.serviceDescription,
		r.startTime,
		r.nextCheck,
		r.coreTime,
		r.timeout,
		r.commandLine)
}

func createCipher(key []byte, encrypt bool) cipher.Block {
	if encrypt {
		newCipher, err := aes.NewCipher(key)
		if err != nil {
			logger.Panic(err)
		}
		return newCipher
	}
	return nil
}

/**
*
*@input: string to be converted to base64
*@return: byte array representing the string in b64
 */
func decodeBase64(data string) []byte {
	decodedBase, _ := b64.StdEncoding.DecodeString(data)
	return decodedBase
}

/* Decrypt
*  Decodes the bytes from data with the given key
*  returns a received struct
 */
func decrypt(data []byte, encryption bool) (*receivedStruct, error) {
	if !encryption {
		return createReceived(data)
	}

	decrypted := make([]byte, len(data))
	size := 16

	for bs, be := 0, size; bs < len(data); bs, be = bs+size, be+size {
		myCipher.Decrypt(decrypted[bs:be], data[bs:be])
	}

	return createReceived(decrypted)
}

/*
*@input: the bytes received from the gearman
*@return: a received struct conating the values received
 */
func createReceived(input []byte) (*receivedStruct, error) {
	var result receivedStruct

	if !bytes.HasPrefix(input, []byte("type=")) {
		return nil, fmt.Errorf("decrypt error, invalid data package received, check encryption key")
	}

	//create a map with the values
	stringMap := createMap(input)

	//then extract them and store them
	result.typ = stringMap["type"]
	result.resultQueue = stringMap["result_queue"]
	result.hostName = stringMap["host_name"]
	result.serviceDescription = stringMap["service_description"]
	result.commandLine = stringMap["command_line"]
	result.startTime = parseTimeStringToFloat64(stringMap["start_time"])
	result.coreTime = parseTimeStringToFloat64(stringMap["core_time"])
	result.nextCheck = parseTimeStringToFloat64(stringMap["next_check"])
	result.timeout = getInt(stringMap["timeout"])

	return &result, nil
}

func parseTimeStringToFloat64(input string) float64 {
	floatValue, err := strconv.ParseFloat(input, 64)
	if err != nil {
		return 0
	}
	return floatValue
}

//takes a byte input, splits it first at every new line
//then stores every line splitted by an = in a map
//returns map[(value before = )] = (value after =)
func createMap(input []byte) map[string]string {
	stringValue := string(input)
	splitted := strings.Split(stringValue, "\n")
	//every command is now in one array field

	resultMap := make(map[string]string)

	for i := 0; i < len(splitted); i++ {
		//split at = and store in map
		access := strings.SplitN(splitted[i], "=", 2)

		if len(access) > 1 {
			access[0] = strings.Trim(access[0], " ")
			access[1] = strings.Trim(access[1], " ")
			resultMap[access[0]] = access[1]
		}
	}
	return resultMap
}
