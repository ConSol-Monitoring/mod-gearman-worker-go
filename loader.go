package main

import (
	"io/ioutil"
	"log"
	"os"
	"path"
)

//returns the secret_key as byte array from the location in the worker.cfg
func getKey() []byte {
	if config.encryption {
		if config.key != "" {
			return []byte(config.key)
		}
		if config.keyfile != "" {
			return readKeyFile(config.keyfile)
		}
		logger.Debug("no key set!")
		return nil
	}
	return nil

}

//loads the keyfile and extracts the key, if a newline is at the end it gets cut off
func readKeyFile(path string) []byte {
	dat, err := ioutil.ReadFile(path)
	if err != nil {
		log.Panic("could not open keyfile")
	}
	if len(dat) > 1 && dat[len(dat)-1] == 10 {
		return dat[:len(dat)-1]
	}

	return dat[:]

}

func openFileOrCreate(path string) (os.File, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		createDirectoryAndFile(path)
		//open the file
		file, err := os.Open(path)
		if err != nil {
			logger.Error("could not open file %s", path)
			return *file, err
		}
		return *file, nil
	}
	//open the file
	file, err := os.Open(path)
	if err != nil {
		logger.Error("could not open file %s", path)
	}
	return *file, nil

}

func createDirectoryAndFile(pathe string) {
	directory, file := path.Split(pathe)
	if directory != "" {
		err := os.MkdirAll(directory, 0755)
		if err != nil {
			logger.Panic(err)
		}
		_, err = os.Create(directory + "/" + file)
		if err != nil {
			logger.Panic(err)
		}
	} else {
		_, err := os.Create(file)
		if err != nil {
			logger.Panic(err)
		}
	}

}
