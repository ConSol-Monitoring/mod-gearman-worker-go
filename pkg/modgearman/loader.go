package modgearman

import (
	"fmt"
	"os"
	"path"
)

const (
	// EncryptionKeySize defines the exact size of the encryption key
	EncryptionKeySize = 32
)

// returns the secret_key as byte array from the location in the worker.cfg
func getKey(config *config) []byte {
	if config.encryption {
		if config.key != "" {
			return fixKeySize([]byte(config.key))
		}
		if config.keyfile != "" {
			return fixKeySize(readKeyFile(config.keyfile))
		}
		log.Panic("no key set but encyption enabled!")

		return nil
	}

	return nil
}

// loads the keyfile and extracts the key, if a newline is at the end it gets cut off
func readKeyFile(filename string) []byte {
	dat, err := os.ReadFile(filename)
	if err != nil {
		log.Fatalf("could not open keyfile")
	}
	if len(dat) > 1 && dat[len(dat)-1] == 10 {
		return dat[:len(dat)-1]
	}

	return dat
}

func fixKeySize(key []byte) []byte {
	if len(key) > EncryptionKeySize {
		return key[0:EncryptionKeySize]
	}
	for {
		if len(key) == EncryptionKeySize {
			return key
		}
		key = append(key, 0)
	}
}

func openFileOrCreate(filename string) (os.File, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		createDirectoryAndFile(filename)
		file, err := os.Open(filename)
		if err != nil {
			log.Errorf("could not open file %s: %w", filename, err)

			return *file, fmt.Errorf("open file %s failed: %w", filename, err)
		}

		return *file, nil
	}
	file, err := os.Open(filename)
	if err != nil {
		log.Errorf("could not open file %s: %w", filename, err)
	}

	return *file, nil
}

func createDirectoryAndFile(pathe string) {
	directory, file := path.Split(pathe)
	if directory != "" {
		err := os.MkdirAll(directory, 0o755)
		if err != nil {
			log.Fatalf("mkdir: %s", err.Error())
		}
		_, err = os.Create(directory + "/" + file)
		if err != nil {
			log.Fatalf("open: %s", err.Error())
		}
	} else {
		_, err := os.Create(file)
		if err != nil {
			log.Fatalf("open: %s", err.Error())
		}
	}
}
