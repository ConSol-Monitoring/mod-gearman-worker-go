package main

import (
	"crypto/aes"
	b64 "encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

type receivedStruct struct {
	typ                 string
	result_queue        string
	host_name           string
	service_description string
	start_time          float64
	next_check          float64
	core_time           float64
	timeout             int
	command_line        string
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
		r.result_queue,
		r.host_name,
		r.service_description,
		r.start_time,
		r.next_check,
		r.core_time,
		r.timeout,
		r.command_line)
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
func decrypt(data []byte, key []byte) *receivedStruct {
	if !config.encryption {
		return createReceived(data)
	}
	cipher, err := aes.NewCipher([]byte(key))
	if err != nil {
		logger.Panic(err)
	}
	decrypted := make([]byte, len(data))
	size := 16

	for bs, be := 0, size; bs < len(data); bs, be = bs+size, be+size {
		cipher.Decrypt(decrypted[bs:be], data[bs:be])
	}

	return createReceived(decrypted)
}

/*
*@input: the bytes received from the gearman
*@return: a received struct conating the values received
 */
func createReceived(input []byte) *receivedStruct {
	var result receivedStruct

	//create a map with the values
	stringMap := createMap(input)

	//then extract them and store them
	result.typ = stringMap["type"]
	result.result_queue = stringMap["result_queue"]
	result.host_name = stringMap["host_name"]
	result.service_description = stringMap["service_description"]
	result.command_line = stringMap["command_line"]
	result.start_time = parseTimeStringToFloat64(stringMap["start_time"])
	result.core_time = parseTimeStringToFloat64(stringMap["core_time"])
	result.next_check = parseTimeStringToFloat64(stringMap["next_check"])
	result.timeout = getInt(stringMap["timeout"])

	return &result
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
	stringValue := string(input[:])
	splitted := strings.Split(stringValue, "\n")
	//every command is now in one array field

	resultMap := make(map[string]string)

	for i := 0; i < len(splitted); i++ {

		//split at = and store in map
		access := strings.Split(splitted[i], "=")

		if len(access) > 1 {
			access[0] = strings.Trim(access[0], " ")
			access[1] = strings.Trim(access[1], " ")
			resultMap[access[0]] = access[1]
		}
	}
	return resultMap
}
